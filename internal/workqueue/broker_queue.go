package workqueue

import (
	"fmt"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/evalbroker"
)

// BrokerQueue wraps an evalbroker.Broker and implements WorkQueue.
// It provides ack/nack semantics, visibility timeouts, and priority
// ordering on top of the standard WorkQueue interface.
//
// Use this when you need reliable dispatch (survives worker crashes)
// instead of the SimpleQueue which loses items on panic.
type BrokerQueue struct {
	broker *evalbroker.Broker
}

// NewBrokerQueue creates a WorkQueue backed by an evalbroker.
func NewBrokerQueue(cfg evalbroker.Config) *BrokerQueue {
	return &BrokerQueue{
		broker: evalbroker.New(cfg),
	}
}

// Add enqueues a key with default priority.
func (bq *BrokerQueue) Add(key string) error {
	bq.broker.Enqueue(evalbroker.Evaluation{
		ID:         key,
		Priority:   0,
		Type:       "reconcile",
		CreateTime: time.Now(),
	})
	return nil
}

// AddAfter enqueues after a delay.
func (bq *BrokerQueue) AddAfter(key string, duration time.Duration) error {
	go func() {
		<-time.After(duration)
		bq.Add(key)
	}()
	return nil
}

// AddRateLimited enqueues with default priority.
func (bq *BrokerQueue) AddRateLimited(key string) error {
	return bq.Add(key)
}

// Get dequeues the highest-priority evaluation and blocks until one is available.
func (bq *BrokerQueue) Get() (*Item, error) {
	eval, ok := bq.broker.Dequeue()
	if !ok {
		return nil, fmt.Errorf("broker closed")
	}
	return &Item{
		Key:     eval.ID,
		AddedAt: eval.CreateTime,
	}, nil
}

// Done acknowledges the item — it is permanently removed from the broker.
func (bq *BrokerQueue) Done(key string) error {
	bq.broker.Ack(key)
	return nil
}

// Forget nacks the item without re-enqueue delay, allowing it to be
// moved to the DLQ if the delivery limit is exceeded.
func (bq *BrokerQueue) Forget(key string) error {
	return bq.broker.Nack(key, 0)
}

// NumRequeues is not tracked by evalbroker — always returns 0.
func (bq *BrokerQueue) NumRequeues(key string) int {
	return 0
}

// Shutdown stops the broker.
func (bq *BrokerQueue) Shutdown() error {
	bq.broker.Close()
	return nil
}

// Len returns the number of pending evaluations.
func (bq *BrokerQueue) Len() int {
	// Broker doesn't expose a count; return 0 as approximate.
	return 0
}

// AddWithPriority enqueues with a specified priority (higher = dispatched first).
func (bq *BrokerQueue) AddWithPriority(key string, priority int) error {
	bq.broker.Enqueue(evalbroker.Evaluation{
		ID:         key,
		Priority:   priority,
		Type:       "reconcile",
		CreateTime: time.Now(),
	})
	return nil
}

// DrainDLQ returns all items in the dead-letter queue for inspection.
func (bq *BrokerQueue) DrainDLQ() []evalbroker.Evaluation {
	return bq.broker.DLQ()
}
