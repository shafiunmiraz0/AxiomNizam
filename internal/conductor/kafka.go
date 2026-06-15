package conductor

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/conductor/models"
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"

	"github.com/IBM/sarama"
)

// KafkaBackend manages Kafka producer/consumer connections.
type KafkaBackend struct {
	mu        sync.RWMutex
	brokers   []string
	producers map[string]sarama.SyncProducer
	consumers map[string]sarama.ConsumerGroup
	stopChs   map[string]chan struct{}
}

// NewKafkaBackend creates a new Kafka backend.
func NewKafkaBackend(brokers []string) *KafkaBackend {
	return &KafkaBackend{
		brokers:   brokers,
		producers: make(map[string]sarama.SyncProducer),
		consumers: make(map[string]sarama.ConsumerGroup),
		stopChs:   make(map[string]chan struct{}),
	}
}

// Close tears down all producers and consumer groups.
func (k *KafkaBackend) Close() {
	k.mu.Lock()
	defer k.mu.Unlock()

	for id, p := range k.producers {
		p.Close()
		delete(k.producers, id)
	}
	for id, cg := range k.consumers {
		cg.Close()
		delete(k.consumers, id)
	}
	for id, stop := range k.stopChs {
		close(stop)
		delete(k.stopChs, id)
	}
}

// getProducer returns (or creates) a SyncProducer for the producer ID.
func (k *KafkaBackend) getProducer(id string) (sarama.SyncProducer, error) {
	k.mu.Lock()
	defer k.mu.Unlock()

	if p, ok := k.producers[id]; ok {
		return p, nil
	}

	cfg := sarama.NewConfig()
	cfg.Producer.Return.Successes = true
	cfg.Producer.RequiredAcks = sarama.WaitForAll
	cfg.Producer.Retry.Max = 3

	p, err := sarama.NewSyncProducer(k.brokers, cfg)
	if err != nil {
		return nil, fmt.Errorf("kafka new producer: %w", err)
	}
	k.producers[id] = p
	return p, nil
}

// Publish sends a message to a Kafka topic.
func (k *KafkaBackend) Publish(p *Producer, msg *Message) error {
	sp, err := k.getProducer(p.ID)
	if err != nil {
		return err
	}

	body, err := json.Marshal(msg.Body)
	if err != nil {
		return fmt.Errorf("kafka marshal body: %w", err)
	}

	headers := make([]sarama.RecordHeader, 0, len(msg.Headers))
	for key, val := range msg.Headers {
		headers = append(headers, sarama.RecordHeader{
			Key:   []byte(key),
			Value: []byte(val),
		})
	}
	if msg.CorrelationID != "" {
		headers = append(headers, sarama.RecordHeader{
			Key:   []byte("correlationId"),
			Value: []byte(msg.CorrelationID),
		})
	}

	topic := p.Topic
	if topic == "" {
		topic = p.Exchange // fallback
	}
	if topic == "" {
		return fmt.Errorf("kafka: no topic configured for producer %s", p.ID)
	}

	pm := &sarama.ProducerMessage{
		Topic:   topic,
		Value:   sarama.ByteEncoder(body),
		Headers: headers,
	}

	_, _, err = sp.SendMessage(pm)
	return err
}

// consumerGroupHandler implements sarama.ConsumerGroupHandler.
type consumerGroupHandler struct {
	consumerID string
	handler    func(*Message) error
	cfg        models.ConsumerConfig
}

func (h *consumerGroupHandler) Setup(_ sarama.ConsumerGroupSession) error   { return nil }
func (h *consumerGroupHandler) Cleanup(_ sarama.ConsumerGroupSession) error { return nil }

func (h *consumerGroupHandler) ConsumeClaim(sess sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for m := range claim.Messages() {
		msg := &Message{
			ID:          fmt.Sprintf("kafka-%d-%d", m.Partition, m.Offset),
			ConsumerID:  h.consumerID,
			ContentType: "application/json",
			Timestamp:   m.Timestamp,
			Status:      "delivered",
			DeliveredAt: time.Now(),
			Headers:     make(map[string]string),
		}

		var body map[string]interface{}
		if err := json.Unmarshal(m.Value, &body); err != nil {
			body = map[string]interface{}{"raw": string(m.Value)}
		}
		msg.Body = body

		for _, hdr := range m.Headers {
			msg.Headers[string(hdr.Key)] = string(hdr.Value)
			if string(hdr.Key) == "correlationId" {
				msg.CorrelationID = string(hdr.Value)
			}
		}

		if herr := h.handler(msg); herr != nil {
			msg.ErrorMessage = herr.Error()
			msg.RetryCount++
			// Kafka consumer groups handle offset commit; we mark it anyway
			// so the conductor layer can DLQ the message if needed.
		} else {
			msg.Status = "acked"
			msg.AckedAt = time.Now()
		}

		sess.MarkMessage(m, "")
	}
	return nil
}

// StartConsumer creates a consumer group and begins consuming.
func (k *KafkaBackend) StartConsumer(c *Consumer, handler func(*Message) error) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	cfg := sarama.NewConfig()
	cfg.Consumer.Group.Rebalance.GroupStrategies = []sarama.BalanceStrategy{sarama.NewBalanceStrategyRoundRobin()}
	cfg.Consumer.Offsets.Initial = sarama.OffsetNewest

	group := c.ConsumerGroup
	if group == "" {
		group = "conductor-" + c.ID
	}

	cg, err := sarama.NewConsumerGroup(k.brokers, group, cfg)
	if err != nil {
		return fmt.Errorf("kafka consumer group: %w", err)
	}
	k.consumers[c.ID] = cg

	topic := c.Topic
	if topic == "" {
		topic = c.Queue // fallback
	}

	stop := make(chan struct{})
	k.stopChs[c.ID] = stop

	go func() {
		gh := &consumerGroupHandler{
			consumerID: c.ID,
			handler:    handler,
			cfg:        c.Config,
		}
		for {
			select {
			case <-stop:
				return
			default:
				if err := cg.Consume(nil, []string{topic}, gh); err != nil {
					logging.Z().Info(fmt.Sprintf("[conductor/kafka] consumer %s error: %v", c.ID, err))
					time.Sleep(2 * time.Second)
				}
			}
		}
	}()

	logging.Z().Info(fmt.Sprintf("[conductor/kafka] consumer %s started on topic %s (group %s)", c.ID, topic, group))
	return nil
}

// StopConsumer signals a consumer to stop.
func (k *KafkaBackend) StopConsumer(consumerID string) {
	k.mu.Lock()
	defer k.mu.Unlock()

	if stop, ok := k.stopChs[consumerID]; ok {
		close(stop)
		delete(k.stopChs, consumerID)
	}
	if cg, ok := k.consumers[consumerID]; ok {
		cg.Close()
		delete(k.consumers, consumerID)
	}
}

// StopProducer closes a Kafka producer.
func (k *KafkaBackend) StopProducer(producerID string) {
	k.mu.Lock()
	defer k.mu.Unlock()

	if p, ok := k.producers[producerID]; ok {
		p.Close()
		delete(k.producers, producerID)
	}
}
