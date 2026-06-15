package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/distributedstate"
)

func main() {
	command := flag.String("cmd", "help", "Command: store-test, lock-test, election-test, counter-test, set-test, queue-test, cas-test, cache-test, all")
	storeType := flag.String("store", "memory", "Store type: memory or etcd")
	etcdEndpoint := flag.String("etcd", "localhost:2379", "etcd endpoint")
	verbose := flag.Bool("v", false, "Verbose output")
	flag.Parse()

	if *verbose {
		log.SetFlags(log.LstdFlags | log.Lshortfile)
	}

	var store distributedstate.StateStore
	var err error

	switch *storeType {
	case "etcd":
		store, err = distributedstate.NewEtcdStateStore([]string{*etcdEndpoint})
		if err != nil {
			log.Fatalf("Failed to connect to etcd: %v", err)
		}
		fmt.Println("✓ Connected to etcd")
	case "memory":
		store = distributedstate.NewInMemoryStateStore()
		fmt.Println("✓ Using in-memory store")
	default:
		log.Fatalf("Unknown store type: %s", *storeType)
	}

	defer store.Close()

	ctx := context.Background()

	switch *command {
	case "store-test":
		testBasicStore(ctx, store)
	case "lock-test":
		testDistributedLock(ctx, store)
	case "election-test":
		testLeaderElection(ctx, store)
	case "counter-test":
		testDistributedCounter(ctx, store)
	case "set-test":
		testDistributedSet(ctx, store)
	case "queue-test":
		testDistributedQueue(ctx, store)
	case "cas-test":
		testCompareAndSwap(ctx, store)
	case "cache-test":
		testCachedStore(ctx, store)
	case "all":
		testBasicStore(ctx, store)
		testCompareAndSwap(ctx, store)
		testDistributedCounter(ctx, store)
		testDistributedSet(ctx, store)
		testDistributedQueue(ctx, store)
		testDistributedLock(ctx, store)
		testCachedStore(ctx, store)
	case "help":
		printHelp()
	default:
		log.Fatalf("Unknown command: %s", *command)
	}

	fmt.Println("\n✓ All tests completed")
}

func testBasicStore(ctx context.Context, store distributedstate.StateStore) {
	fmt.Println("\n=== Testing Basic Store Operations ===")

	tests := []struct {
		key   string
		value string
	}{
		{"test/key1", "value1"},
		{"test/key2", "value2"},
		{"config/app/name", "AxiomNizam"},
		{"config/app/version", "1.0.0"},
	}

	for _, test := range tests {
		if err := store.Put(ctx, test.key, test.value); err != nil {
			fmt.Printf("✗ Put failed: %v\n", err)
			return
		}
		fmt.Printf("✓ Put: %s = %s\n", test.key, test.value)
	}

	for _, test := range tests {
		value, err := store.Get(ctx, test.key)
		if err != nil || value != test.value {
			fmt.Printf("✗ Get failed: %v\n", err)
			return
		}
		fmt.Printf("✓ Get: %s = %s\n", test.key, value)
	}

	items, err := store.List(ctx, "test/")
	if err != nil {
		fmt.Printf("✗ List failed: %v\n", err)
		return
	}
	fmt.Printf("✓ List: found %d items\n", len(items))

	if err := store.Delete(ctx, "test/key1"); err != nil {
		fmt.Printf("✗ Delete failed: %v\n", err)
		return
	}
	fmt.Println("✓ Delete: test/key1")

	value, err := store.Get(ctx, "test/key1")
	if value != "" {
		fmt.Printf("✗ Verify delete failed: key still exists\n")
		return
	}
	fmt.Println("✓ Verify delete: key removed")
}

func testCompareAndSwap(ctx context.Context, store distributedstate.StateStore) {
	fmt.Println("\n=== Testing Compare-and-Swap ===")

	key := "cas/test"

	if err := store.Put(ctx, key, "initial"); err != nil {
		fmt.Printf("✗ Initial put failed: %v\n", err)
		return
	}
	fmt.Println("✓ Initial value set")

	success, err := store.CompareAndSwap(ctx, key, "initial", "updated")
	if err != nil || !success {
		fmt.Printf("✗ CAS failed: success=%v, err=%v\n", success, err)
		return
	}
	fmt.Println("✓ CAS succeeded: initial → updated")

	success, err = store.CompareAndSwap(ctx, key, "initial", "should_fail")
	if err != nil || success {
		fmt.Printf("✗ CAS should have failed but didn't\n")
		return
	}
	fmt.Println("✓ CAS correctly rejected old value")

	value, err := store.Get(ctx, key)
	if err != nil || value != "updated" {
		fmt.Printf("✗ Value verification failed: %s\n", value)
		return
	}
	fmt.Println("✓ Value correctly updated")
}

func testDistributedCounter(ctx context.Context, store distributedstate.StateStore) {
	fmt.Println("\n=== Testing Distributed Counter ===")

	counter := distributedstate.NewDistributedCounter(store, "counter/visits")

	for i := 1; i <= 5; i++ {
		val, err := counter.Increment(ctx)
		if err != nil {
			fmt.Printf("✗ Increment failed: %v\n", err)
			return
		}
		fmt.Printf("✓ Counter increment: %d\n", val)
	}

	current, err := counter.Get(ctx)
	if err != nil || current != 5 {
		fmt.Printf("✗ Counter get failed: expected 5, got %d\n", current)
		return
	}
	fmt.Printf("✓ Counter current value: %d\n", current)

	for i := 1; i <= 3; i++ {
		val, err := counter.Decrement(ctx)
		if err != nil {
			fmt.Printf("✗ Decrement failed: %v\n", err)
			return
		}
		fmt.Printf("✓ Counter decrement: %d\n", val)
	}
}

func testDistributedSet(ctx context.Context, store distributedstate.StateStore) {
	fmt.Println("\n=== Testing Distributed Set ===")

	set := distributedstate.NewDistributedSet(store, "set/users")

	members := []string{"alice", "bob", "charlie", "diana"}

	for _, member := range members {
		if err := set.Add(ctx, member); err != nil {
			fmt.Printf("✗ Add failed: %v\n", err)
			return
		}
		fmt.Printf("✓ Added: %s\n", member)
	}

	contains, err := set.Contains(ctx, "alice")
	if err != nil || !contains {
		fmt.Printf("✗ Contains check failed\n")
		return
	}
	fmt.Println("✓ Contains check: alice exists")

	size, err := set.Size(ctx)
	if err != nil || size != len(members) {
		fmt.Printf("✗ Size check failed: expected %d, got %d\n", len(members), size)
		return
	}
	fmt.Printf("✓ Set size: %d\n", size)

	if err := set.Remove(ctx, "bob"); err != nil {
		fmt.Printf("✗ Remove failed: %v\n", err)
		return
	}
	fmt.Println("✓ Removed: bob")

	size, err = set.Size(ctx)
	if err != nil || size != len(members)-1 {
		fmt.Printf("✗ Size after remove failed: expected %d, got %d\n", len(members)-1, size)
		return
	}
	fmt.Printf("✓ Set size after remove: %d\n", size)
}

func testDistributedQueue(ctx context.Context, store distributedstate.StateStore) {
	fmt.Println("\n=== Testing Distributed Queue ===")

	queue := distributedstate.NewDistributedQueue(store, "queue/tasks")

	tasks := []string{"task1", "task2", "task3", "task4", "task5"}

	for _, task := range tasks {
		if err := queue.Enqueue(ctx, task); err != nil {
			fmt.Printf("✗ Enqueue failed: %v\n", err)
			return
		}
		fmt.Printf("✓ Enqueued: %s\n", task)
	}

	size, err := queue.Size(ctx)
	if err != nil || size != len(tasks) {
		fmt.Printf("✗ Size check failed: expected %d, got %d\n", len(tasks), size)
		return
	}
	fmt.Printf("✓ Queue size: %d\n", size)

	for i := 0; i < 3; i++ {
		task, err := queue.Dequeue(ctx)
		if err != nil {
			fmt.Printf("✗ Dequeue failed: %v\n", err)
			return
		}
		fmt.Printf("✓ Dequeued: %s\n", task)
	}

	size, err = queue.Size(ctx)
	if err != nil {
		fmt.Printf("✗ Size check after dequeue failed\n")
		return
	}
	fmt.Printf("✓ Queue size after dequeue: %d\n", size)
}

func testDistributedLock(ctx context.Context, store distributedstate.StateStore) {
	fmt.Println("\n=== Testing Distributed Lock ===")

	lock1 := distributedstate.NewDistributedLock(store, "resource/db", "node1", 5*time.Second)

	acquired, err := lock1.Acquire(ctx)
	if err != nil || !acquired {
		fmt.Printf("✗ Acquire lock failed: %v\n", err)
		return
	}
	fmt.Println("✓ Lock acquired by node1")

	lock2 := distributedstate.NewDistributedLock(store, "resource/db", "node2", 5*time.Second)

	acquired, err = lock2.Acquire(ctx)
	if err == nil && acquired {
		fmt.Printf("✗ Lock should not be acquired by node2\n")
		return
	}
	fmt.Println("✓ Lock correctly denied to node2")

	held, holder, err := lock1.IsHeld(ctx)
	if err != nil || !held || holder != "node1" {
		fmt.Printf("✗ IsHeld check failed\n")
		return
	}
	fmt.Printf("✓ Lock held by: %s\n", holder)

	if err := lock1.Release(ctx); err != nil {
		fmt.Printf("✗ Release failed: %v\n", err)
		return
	}
	fmt.Println("✓ Lock released by node1")

	held, _, err = lock1.IsHeld(ctx)
	if err != nil || held {
		fmt.Printf("✗ Lock should be released\n")
		return
	}
	fmt.Println("✓ Lock is now available")
}

func testCachedStore(ctx context.Context, store distributedstate.StateStore) {
	fmt.Println("\n=== Testing Cached Store ===")

	underlying := distributedstate.NewInMemoryStateStore()
	cached := distributedstate.NewCachedStateStore(underlying, 2*time.Second)

	if err := cached.Put(ctx, "cache/test", "value1"); err != nil {
		fmt.Printf("✗ Put failed: %v\n", err)
		return
	}
	fmt.Println("✓ Initial value cached")

	v1, _ := cached.Get(ctx, "cache/test")
	fmt.Printf("✓ First get (from cache): %s\n", v1)

	v2, _ := cached.Get(ctx, "cache/test")
	fmt.Printf("✓ Second get (from cache): %s\n", v2)

	cached.Invalidate("cache/test")
	fmt.Println("✓ Cache invalidated")

	if err := cached.Put(ctx, "cache/test", "value2"); err != nil {
		fmt.Printf("✗ Put after invalidate failed: %v\n", err)
		return
	}

	v3, _ := cached.Get(ctx, "cache/test")
	fmt.Printf("✓ After update: %s\n", v3)

	cached.Close()
}

func testLeaderElection(ctx context.Context, store distributedstate.StateStore) {
	fmt.Println("\n=== Testing Leader Election ===")

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	node1 := distributedstate.NewDistributedLeaderElection(store, "cluster", "node1", 1*time.Second)
	node2 := distributedstate.NewDistributedLeaderElection(store, "cluster", "node2", 1*time.Second)

	leaderChanges := make([]string, 0)

	node1.OnLeadershipChange(func(isLeader bool) {
		if isLeader {
			leaderChanges = append(leaderChanges, "node1-elected")
			fmt.Println("✓ node1 elected as leader")
		} else {
			leaderChanges = append(leaderChanges, "node1-demoted")
			fmt.Println("✓ node1 lost leadership")
		}
	})

	node2.OnLeadershipChange(func(isLeader bool) {
		if isLeader {
			leaderChanges = append(leaderChanges, "node2-elected")
			fmt.Println("✓ node2 elected as leader")
		} else {
			leaderChanges = append(leaderChanges, "node2-demoted")
			fmt.Println("✓ node2 lost leadership")
		}
	})

	go func() {
		if err := node1.Start(ctx); err != nil && err != context.DeadlineExceeded {
			fmt.Printf("✗ node1 election failed: %v\n", err)
		}
	}()

	go func() {
		time.Sleep(500 * time.Millisecond)
		if err := node2.Start(ctx); err != nil && err != context.DeadlineExceeded {
			fmt.Printf("✗ node2 election failed: %v\n", err)
		}
	}()

	<-ctx.Done()

	if len(leaderChanges) > 0 {
		fmt.Printf("✓ Leadership changes: %v\n", leaderChanges)
	}
}

func printHelp() {
	help := `
AxiomNizam Distributed State Store Tester

Usage:
  go run cmd/axiomnizamctl/test_cli.go [options]

Options:
  -cmd string
        Command to run (default "help")
        Commands:
          store-test      - Test basic store operations
          lock-test       - Test distributed locks
          election-test   - Test leader election
          counter-test    - Test distributed counter
          set-test        - Test distributed set
          queue-test      - Test distributed queue
          cas-test        - Test compare-and-swap
          cache-test      - Test cached store
          all             - Run all tests
          help            - Show this help

  -store string
        Store backend (default "memory")
        Options: memory, etcd

  -etcd string
        etcd endpoint (default "localhost:2379")
        Used when -store=etcd

  -v    Enable verbose output

Examples:
  # Test with in-memory store
  go run cmd/axiomnizamctl/test_cli.go -cmd all -store memory

  # Test with etcd
  go run cmd/axiomnizamctl/test_cli.go -cmd all -store etcd -etcd localhost:2379

  # Test specific functionality
  go run cmd/axiomnizamctl/test_cli.go -cmd lock-test -store memory

  # Verbose output
  go run cmd/axiomnizamctl/test_cli.go -cmd all -v
`
	fmt.Println(help)
}
