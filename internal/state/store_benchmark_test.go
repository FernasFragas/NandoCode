package state

import (
	"testing"
)

// BenchmarkStoreSetFiveSubscribers benchmarks Store Set performance with 5 subscribers.
// The goal is to achieve 10,000+ Set calls/sec with 5 subscribers and p99 latency at or below 1ms.
func BenchmarkStoreSetFiveSubscribers(b *testing.B) {
	// Create a store with an onChange callback that does work similar to OnChange
	store := NewStore(0, func(prev, next int) {
		// Simulate minimal onChange work: field comparison
		_ = prev == next
	})

	// Create 5 subscribers
	channels := make([]<-chan int, 5)
	unsubs := make([]func(), 5)
	for i := 0; i < 5; i++ {
		ch, unsub := store.Subscribe()
		channels[i] = ch
		unsubs[i] = unsub
	}
	defer func() {
		for _, u := range unsubs {
			u()
		}
	}()

	// Warm up: drain initial values
	for _, ch := range channels {
		<-ch
	}

	// Run the benchmark
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.Set(func(prev int) int {
			return prev + 1
		})
	}
	b.StopTimer()

	// Verify subscribers can still read (basic sanity check)
	store.Set(func(prev int) int {
		return prev + 1
	})
	for _, ch := range channels {
		select {
		case <-ch:
			// Success
		default:
			b.Error("subscriber failed to receive final update")
		}
	}

	// Report ops/sec if b.N >= 10000
	if b.N >= 10000 {
		opsPerSec := float64(b.N) / b.Elapsed().Seconds()
		b.Logf("%.0f ops/sec with 5 subscribers", opsPerSec)
	}
}
