package runtime

import (
	"context"
	"sync"
	"testing"
)

// TestInMemoryBrokerPublishVsCancelNoPanic races publishers against
// subscribe/cancel churn. A subscriber's cancel() closes its channel, and the
// previous Publish (snapshot-then-send-after-unlock) could send on that closed
// channel, panicking (review-v1 P2-7). Holding the read lock across the sends
// makes that impossible. Without the fix this test panics with "send on closed
// channel"; with it, it runs clean.
func TestInMemoryBrokerPublishVsCancelNoPanic(t *testing.T) {
	b := NewInMemoryBroker()
	var wg sync.WaitGroup
	stop := make(chan struct{})

	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_ = b.Publish(context.Background(), Event{Type: EventDraftCreated})
				}
			}
		}()
	}

	for i := 0; i < 500; i++ {
		_, cancel := b.Subscribe(EventDraftCreated, 1)
		cancel()
	}
	close(stop)
	wg.Wait()
}
