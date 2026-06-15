package distributedstate

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
)

func ExampleEtcdStore() {
	store, err := NewEtcdStateStore([]string{"localhost:2379"})
	if err != nil {
		logging.Z().Fatal("etcd store failed", zap.Error(err))
	}
	defer store.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	store.Put(ctx, "config/app", "active")

	value, err := store.Get(ctx, "config/app")
	if err != nil {
		logging.Z().Fatal("etcd store failed", zap.Error(err))
	}

	fmt.Println("Config:", value)

	success, err := store.CompareAndSwap(ctx, "config/app", "active", "inactive")
	fmt.Println("CAS succeeded:", success)
}

func ExampleDistributedManager() {
	store := NewInMemoryStateStore()
	manager := NewDistributedManager(store, "myapp")

	ctx := context.Background()

	manager.PutState(ctx, "user/1/name", "John")
	manager.PutState(ctx, "user/1/email", "john@example.com")

	name, _ := manager.GetState(ctx, "user/1/name")
	fmt.Println("Name:", name)

	states, _ := manager.ListStates(ctx)
	fmt.Println("All states:", states)

	manager.UpdateStateIfUnchanged(ctx, "user/1/name", "John", "Jane")
	updated, _ := manager.GetState(ctx, "user/1/name")
	fmt.Println("Updated:", updated)
}

func ExampleDistributedLock() {
	store := NewInMemoryStateStore()
	ctx := context.Background()

	lock := NewDistributedLock(store, "critical-section", "node1", 10*time.Second)

	acquired, err := lock.Acquire(ctx)
	if err != nil || !acquired {
		logging.Z().Fatal("Failed to acquire lock")
	}
	fmt.Println("Lock acquired")

	defer lock.Release(ctx)

	fmt.Println("Critical section execution")
}

func ExampleLeaderElection() {
	store := NewInMemoryStateStore()
	ctx, cancel := context.WithCancel(context.Background())

	election := NewDistributedLeaderElection(store, "cluster", "node1", 2*time.Second)

	election.OnLeadershipChange(func(isLeader bool) {
		if isLeader {
			fmt.Println("Node1 became leader")
		} else {
			fmt.Println("Node1 lost leadership")
		}
	})

	go func() {
		if err := election.Start(ctx); err != nil {
			logging.Z().Fatal("etcd store failed", zap.Error(err))
		}
	}()

	time.Sleep(5 * time.Second)
	cancel()
}

func ExampleDistributedCounter() {
	store := NewInMemoryStateStore()
	ctx := context.Background()

	counter := NewDistributedCounter(store, "visits")

	for i := 0; i < 5; i++ {
		val, _ := counter.Increment(ctx)
		fmt.Println("Counter:", val)
	}
}

func ExampleDistributedSet() {
	store := NewInMemoryStateStore()
	ctx := context.Background()

	set := NewDistributedSet(store, "users/active")

	set.Add(ctx, "user1")
	set.Add(ctx, "user2")
	set.Add(ctx, "user3")

	contains, _ := set.Contains(ctx, "user1")
	fmt.Println("Contains user1:", contains)

	size, _ := set.Size(ctx)
	fmt.Println("Set size:", size)

	members, _ := set.Members(ctx)
	fmt.Println("Members:", members)
}

func ExampleDistributedQueue() {
	store := NewInMemoryStateStore()
	ctx := context.Background()

	queue := NewDistributedQueue(store, "tasks")

	queue.Enqueue(ctx, "task1")
	queue.Enqueue(ctx, "task2")
	queue.Enqueue(ctx, "task3")

	task, _ := queue.Dequeue(ctx)
	fmt.Println("Dequeued:", task)

	size, _ := queue.Size(ctx)
	fmt.Println("Queue size:", size)
}

func ExampleCachedStore() {
	underlying := NewInMemoryStateStore()
	cached := NewCachedStateStore(underlying, 1*time.Second)

	ctx := context.Background()

	cached.Put(ctx, "key1", "value1")
	v1, _ := cached.Get(ctx, "key1")
	fmt.Println("First get (cached):", v1)

	v2, _ := cached.Get(ctx, "key1")
	fmt.Println("Second get (cached):", v2)

	time.Sleep(1100 * time.Millisecond)

	cached.Put(ctx, "key1", "value2")
	v3, _ := cached.Get(ctx, "key1")
	fmt.Println("After invalidation:", v3)

	cached.Close()
}

func ExampleBatchUpdate() {
	store := NewInMemoryStateStore()
	manager := NewDistributedManager(store, "app")
	ctx := context.Background()

	ops := []BatchOperation{
		{Key: "config/db", Value: "postgresql", Type: OperationPut},
		{Key: "config/cache", Value: "redis", Type: OperationPut},
		{Key: "config/queue", Value: "nats", Type: OperationPut},
	}

	success, _ := manager.BatchUpdate(ctx, ops)
	fmt.Println("Batch update succeeded:", success)

	states, _ := manager.ListStates(ctx)
	for k, v := range states {
		fmt.Printf("%s=%s\n", k, v)
	}
}
