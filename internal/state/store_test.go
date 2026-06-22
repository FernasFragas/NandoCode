package state

import (
	"sync"
	"testing"
	"time"
)

func TestGetInitial(t *testing.T) {
	store := NewStore(42, nil)
	if store.Get() != 42 {
		t.Errorf("expected 42, got %d", store.Get())
	}
}

func TestSetUpdatesValue(t *testing.T) {
	store := NewStore(0, nil)
	store.Set(func(prev int) int {
		return prev + 1
	})
	if store.Get() != 1 {
		t.Errorf("expected 1, got %d", store.Get())
	}
}

func TestSetCallsUpdaterOnce(t *testing.T) {
	store := NewStore(0, nil)
	callCount := 0
	store.Set(func(prev int) int {
		callCount++
		return prev + 1
	})
	if callCount != 1 {
		t.Errorf("expected updater to be called once, got %d", callCount)
	}
}

func TestSetCallsOnChangeOnce(t *testing.T) {
	var changeCount int
	var prevVal, nextVal int
	store := NewStore(0, func(prev, next int) {
		changeCount++
		prevVal = prev
		nextVal = next
	})

	store.Set(func(prev int) int {
		return prev + 10
	})

	if changeCount != 1 {
		t.Errorf("expected onChange to be called once, got %d", changeCount)
	}
	if prevVal != 0 || nextVal != 10 {
		t.Errorf("expected onChange(0, 10), got onChange(%d, %d)", prevVal, nextVal)
	}
}

func TestSubscribeReceivesInitialValue(t *testing.T) {
	store := NewStore(42, nil)
	ch, unsub := store.Subscribe()
	defer unsub()

	val := <-ch
	if val != 42 {
		t.Errorf("expected 42, got %d", val)
	}
}

func TestSubscribeReceivesUpdates(t *testing.T) {
	store := NewStore(0, nil)
	ch, unsub := store.Subscribe()
	defer unsub()

	// Drain initial value
	<-ch

	store.Set(func(prev int) int {
		return prev + 5
	})

	val := <-ch
	if val != 5 {
		t.Errorf("expected 5, got %d", val)
	}
}

func TestUnsubscribeClosesChannel(t *testing.T) {
	store := NewStore(0, nil)
	ch, unsub := store.Subscribe()

	unsub()

	// Channel should be closed
	// Reading from a closed channel will return the zero value
	select {
	case <-ch:
		// Expected
	case <-time.After(100 * time.Millisecond):
		t.Error("expected channel to close")
	}
}

func TestUnsubscribeIdempotent(t *testing.T) {
	store := NewStore(0, nil)
	_, unsub := store.Subscribe()

	// Call multiple times - should not panic
	unsub()
	unsub()
	unsub()
}

func TestConcurrentWriters(t *testing.T) {
	store := NewStore(0, nil)
	var wg sync.WaitGroup

	// Spawn 20 concurrent writers
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				store.Set(func(prev int) int {
					return prev + 1
				})
			}
		}(i)
	}

	wg.Wait()

	final := store.Get()
	if final != 2000 {
		t.Errorf("expected final value 2000, got %d", final)
	}
}

func TestConcurrentSubscribers(t *testing.T) {
	store := NewStore(0, nil)
	subscribers := make([]<-chan int, 5)
	unsubs := make([]func(), 5)

	for i := 0; i < 5; i++ {
		ch, unsub := store.Subscribe()
		subscribers[i] = ch
		unsubs[i] = unsub
	}
	defer func() {
		for _, unsub := range unsubs {
			unsub()
		}
	}()

	// Drain initial values
	for _, ch := range subscribers {
		<-ch
	}

	// Update the store
	store.Set(func(prev int) int {
		return prev + 1
	})

	// All subscribers should receive the new value
	for i, ch := range subscribers {
		select {
		case val := <-ch:
			if val != 1 {
				t.Errorf("subscriber %d expected 1, got %d", i, val)
			}
		case <-time.After(100 * time.Millisecond):
			t.Errorf("subscriber %d did not receive update", i)
		}
	}
}

func TestSlowSubscriberDoesNotBlock(t *testing.T) {
	store := NewStore(0, nil)
	ch, unsub := store.Subscribe()
	defer unsub()

	// Drain initial value
	<-ch

	// Perform a Set - this queues value 1
	store.Set(func(prev int) int {
		return prev + 1
	})

	// Verify the queued value is there
	val1 := <-ch
	if val1 != 1 {
		t.Errorf("expected first update to be 1, got %d", val1)
	}

	// Now perform two more Sets quickly
	store.Set(func(prev int) int {
		return prev + 1
	})
	store.Set(func(prev int) int {
		return prev + 1
	})

	// The subscriber should get the latest value (3), not both intermediate values
	// because the first update (2) would be replaced by the second (3)
	val := <-ch
	if val != 2 && val != 3 {
		t.Errorf("expected final value 2 or 3, got %d", val)
	}
}

func TestMultipleSubscribersWithUpdates(t *testing.T) {
	store := NewStore(0, nil)

	subs := make([]<-chan int, 3)
	unsubs := make([]func(), 3)
	for i := 0; i < 3; i++ {
		ch, unsub := store.Subscribe()
		subs[i] = ch
		unsubs[i] = unsub
	}
	defer func() {
		for _, u := range unsubs {
			u()
		}
	}()

	// Drain initials
	for _, ch := range subs {
		<-ch
	}

	// Perform several updates
	for i := 1; i <= 5; i++ {
		store.Set(func(prev int) int {
			return prev + 1
		})

		// Each subscriber should receive the new value
		for j, ch := range subs {
			select {
			case val := <-ch:
				if val != i {
					t.Errorf("subscriber %d expected %d, got %d", j, i, val)
				}
			case <-time.After(100 * time.Millisecond):
				t.Errorf("subscriber %d did not receive update %d", j, i)
			}
		}
	}
}

func TestChangeCallbackWithType(t *testing.T) {
	type testStruct struct {
		Name string
		Age  int
	}

	var lastChange struct {
		prev testStruct
		next testStruct
	}

	store := NewStore(testStruct{Name: "Alice", Age: 30}, func(prev, next testStruct) {
		lastChange.prev = prev
		lastChange.next = next
	})

	store.Set(func(prev testStruct) testStruct {
		prev.Age = 31
		return prev
	})

	if lastChange.prev.Age != 30 {
		t.Errorf("expected prev age 30, got %d", lastChange.prev.Age)
	}
	if lastChange.next.Age != 31 {
		t.Errorf("expected next age 31, got %d", lastChange.next.Age)
	}
}

func TestConcurrentGetWhileSet(t *testing.T) {
	store := NewStore(0, nil)
	var wg sync.WaitGroup

	// Spawn readers and writers concurrently
	for i := 0; i < 100; i++ {
		if i%2 == 0 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_ = store.Get()
			}()
		} else {
			wg.Add(1)
			go func() {
				defer wg.Done()
				store.Set(func(prev int) int {
					return prev + 1
				})
			}()
		}
	}

	wg.Wait()
}
