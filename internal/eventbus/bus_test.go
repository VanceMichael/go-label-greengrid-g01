package eventbus

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/VanceMichael/greengrid/internal/domain"
)

func TestPublishDeliversToSubscribers(t *testing.T) {
	b := New()
	ch, closeSub, err := b.Subscribe("telemetry", 2)
	if err != nil {
		t.Fatal(err)
	}
	defer closeSub()
	if err := b.Publish(context.Background(), Event{Kind: "telemetry.reading", TenantID: "t", AggregateID: "n"}); err != nil {
		t.Fatal(err)
	}
	select {
	case e := <-ch:
		if e.Kind != "telemetry.reading" {
			t.Fatal(e)
		}
	case <-time.After(time.Second):
		t.Fatal("event not delivered")
	}
}
func TestSubscriberCancellationUnblocksBackpressure(t *testing.T) {
	b := New()
	_, closeSub, err := b.Subscribe("full", 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Publish(context.Background(), Event{Kind: "one"}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- b.Publish(ctx, Event{Kind: "two"}) }()
	time.Sleep(10 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("publish stayed blocked")
	}
	closeSub()
}
func TestUnsubscribeClosesChannelAndBusCloseIsIdempotent(t *testing.T) {
	b := New()
	ch, closeSub, err := b.Subscribe("one", 1)
	if err != nil {
		t.Fatal(err)
	}
	closeSub()
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("channel still open")
		}
	case <-time.After(time.Second):
		t.Fatal("channel not closed")
	}
	if b.SubscriberCount() != 0 {
		t.Fatal("subscriber remains")
	}
	b.Close()
	b.Close()
	if _, _, err := b.Subscribe("after", 1); !errors.Is(err, domain.ErrState) {
		t.Fatalf("closed bus err=%v", err)
	}
}
func TestDuplicateSubscriptionAndValidation(t *testing.T) {
	b := New()
	if _, _, err := b.Subscribe("", 1); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal(err)
	}
	if _, _, err := b.Subscribe("id", 0); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal(err)
	}
	_, closeSub, _ := b.Subscribe("id", 1)
	defer closeSub()
	if _, _, err := b.Subscribe("id", 1); !errors.Is(err, domain.ErrAlreadyExists) {
		t.Fatal(err)
	}
}
