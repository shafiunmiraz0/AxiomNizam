package conductor

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// RabbitMQBackend manages RabbitMQ connections, channels, producers, and consumers.
type RabbitMQBackend struct {
	mu       sync.RWMutex
	conn     *amqp.Connection
	url      string
	channels map[string]*amqp.Channel // keyed by producer/consumer ID
	stopChs  map[string]chan struct{} // per-consumer stop signals
}

// NewRabbitMQBackend creates a new RabbitMQ backend.
func NewRabbitMQBackend(url string) *RabbitMQBackend {
	return &RabbitMQBackend{
		url:      url,
		channels: make(map[string]*amqp.Channel),
		stopChs:  make(map[string]chan struct{}),
	}
}

// Connect establishes the AMQP connection. Safe to call multiple times.
func (r *RabbitMQBackend) Connect() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.conn != nil && !r.conn.IsClosed() {
		return nil
	}
	conn, err := amqp.Dial(r.url)
	if err != nil {
		return fmt.Errorf("rabbitmq connect: %w", err)
	}
	r.conn = conn
	logging.Z().Info(fmt.Sprintf("[conductor/rabbitmq] connected to %s", r.url))
	return nil
}

// Close tears down all channels and the connection.
func (r *RabbitMQBackend) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()

	for id, ch := range r.channels {
		ch.Close()
		delete(r.channels, id)
	}
	for id, stop := range r.stopChs {
		close(stop)
		delete(r.stopChs, id)
	}
	if r.conn != nil {
		r.conn.Close()
		r.conn = nil
	}
}

// channel returns (or opens) a channel for the given ownerID.
func (r *RabbitMQBackend) channel(ownerID string) (*amqp.Channel, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if ch, ok := r.channels[ownerID]; ok {
		return ch, nil
	}
	if r.conn == nil || r.conn.IsClosed() {
		return nil, fmt.Errorf("rabbitmq not connected")
	}
	ch, err := r.conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("rabbitmq open channel: %w", err)
	}
	r.channels[ownerID] = ch
	return ch, nil
}

// EnsureExchange declares an exchange (idempotent).
func (r *RabbitMQBackend) EnsureExchange(name, kind string) error {
	ch, err := r.channel("_exchange_" + name)
	if err != nil {
		return err
	}
	if kind == "" {
		kind = "topic"
	}
	return ch.ExchangeDeclare(name, kind, true, false, false, false, nil)
}

// EnsureQueue declares a queue and optionally binds it to an exchange.
func (r *RabbitMQBackend) EnsureQueue(queue, exchange, routingKey string) error {
	ch, err := r.channel("_queue_" + queue)
	if err != nil {
		return err
	}
	_, err = ch.QueueDeclare(queue, true, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("rabbitmq declare queue %s: %w", queue, err)
	}
	if exchange != "" {
		if routingKey == "" {
			routingKey = "#"
		}
		if err := ch.QueueBind(queue, routingKey, exchange, false, nil); err != nil {
			return fmt.Errorf("rabbitmq bind queue %s→%s: %w", queue, exchange, err)
		}
	}
	return nil
}

// Publish sends a message through a producer.
func (r *RabbitMQBackend) Publish(p *Producer, msg *Message) error {
	ch, err := r.channel(p.ID)
	if err != nil {
		return err
	}

	body, err := json.Marshal(msg.Body)
	if err != nil {
		return fmt.Errorf("rabbitmq marshal body: %w", err)
	}

	pub := amqp.Publishing{
		ContentType:   p.ContentType,
		Body:          body,
		Timestamp:     time.Now(),
		MessageId:     msg.ID,
		CorrelationId: msg.CorrelationID,
		Headers:       amqp.Table{},
	}
	if p.Config.Persistent {
		pub.DeliveryMode = amqp.Persistent
	}
	for k, v := range msg.Headers {
		pub.Headers[k] = v
	}

	exchange := p.Exchange
	routingKey := msg.Headers["routingKey"]
	if routingKey == "" {
		routingKey = p.RoutingKey
	}

	return ch.Publish(exchange, routingKey, p.Config.Mandatory, p.Config.Immediate, pub)
}

// StartConsumer launches a goroutine that consumes from the queue and pushes
// messages into the returned channel. The handler function processes each
// message; returning an error triggers retry/DLQ logic.
func (r *RabbitMQBackend) StartConsumer(c *Consumer, handler func(*Message) error) error {
	ch, err := r.channel(c.ID)
	if err != nil {
		return err
	}

	if c.Config.PrefetchCount > 0 {
		if err := ch.Qos(c.Config.PrefetchCount, 0, false); err != nil {
			return fmt.Errorf("rabbitmq qos: %w", err)
		}
	}

	deliveries, err := ch.Consume(c.Queue, c.ID, c.Config.AutoAck, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("rabbitmq consume %s: %w", c.Queue, err)
	}

	stop := make(chan struct{})
	r.mu.Lock()
	r.stopChs[c.ID] = stop
	r.mu.Unlock()

	go func() {
		for {
			select {
			case <-stop:
				return
			case d, ok := <-deliveries:
				if !ok {
					return
				}
				msg := &Message{
					ID:            d.MessageId,
					ConsumerID:    c.ID,
					ContentType:   d.ContentType,
					Timestamp:     d.Timestamp,
					CorrelationID: d.CorrelationId,
					Status:        "delivered",
					DeliveredAt:   time.Now(),
				}
				// parse body
				var body map[string]interface{}
				if err := json.Unmarshal(d.Body, &body); err != nil {
					body = map[string]interface{}{"raw": string(d.Body)}
				}
				msg.Body = body

				// parse headers
				msg.Headers = make(map[string]string)
				for k, v := range d.Headers {
					msg.Headers[k] = fmt.Sprintf("%v", v)
				}

				if herr := handler(msg); herr != nil {
					msg.ErrorMessage = herr.Error()
					msg.RetryCount++
					if !c.Config.AutoAck {
						// nack + requeue only up to maxRetries
						requeue := c.Config.MaxRetries == 0 || msg.RetryCount <= c.Config.MaxRetries
						d.Nack(false, requeue)
					}
				} else {
					msg.Status = "acked"
					msg.AckedAt = time.Now()
					if !c.Config.AutoAck {
						d.Ack(false)
					}
				}
			}
		}
	}()

	logging.Z().Info(fmt.Sprintf("[conductor/rabbitmq] consumer %s started on queue %s", c.ID, c.Queue))
	return nil
}

// StopConsumer signals a consumer goroutine to stop.
func (r *RabbitMQBackend) StopConsumer(consumerID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if stop, ok := r.stopChs[consumerID]; ok {
		close(stop)
		delete(r.stopChs, consumerID)
	}
	if ch, ok := r.channels[consumerID]; ok {
		ch.Close()
		delete(r.channels, consumerID)
	}
}
