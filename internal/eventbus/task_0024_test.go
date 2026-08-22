package eventbus_test

import (
	"context"
	"sync"
	"testing"

	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/eventbus"
)

func TestGreenGridTask0024(t *testing.T) {
	bus := eventbus.New()
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, unsubscribe, err := bus.Subscribe("telemetry-alerts", 1)
			if unsubscribe != nil {
				defer unsubscribe()
			}
			results <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		} else if err != domain.ErrAlreadyExists {
			t.Fatalf("subscribe error=%v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("duplicate subscribers accepted=%d", successes)
	}
	if err := bus.Publish(context.Background(), eventbus.Event{Kind: "telemetry.alert"}); err != nil {
		t.Fatal(err)
	}
}
