package cancel_subscription

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"subscriptionservice/internal/app/subscription/contracts"
	"subscriptionservice/internal/app/subscription/domain"
)

var startedAt = time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)

func TestExecuteSuccess(t *testing.T) {
	repository := &fakeRepository{subscription: rehydrated(domain.StatusActive, nil)}
	interactor := New(repository, &fakeOutbox{}, fakeClock{startedAt.Add(10 * 24 * time.Hour)})

	result, err := interactor.Execute(context.Background(), Command{SubscriptionID: "sub-1"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Subscription.Status() != domain.StatusCancelled {
		t.Fatalf("status = %q, want CANCELLED", result.Subscription.Status())
	}

	want := []string{"update sub-1", "outbox sub-1:cancelled:2"}
	if got := statements(result.Mutations); !slices.Equal(got, want) {
		t.Fatalf("mutations = %q, want %q", got, want)
	}
}

func TestExecuteAlreadyCancelled(t *testing.T) {
	cancelledAt := startedAt.Add(24 * time.Hour)
	outbox := &fakeOutbox{}
	repository := &fakeRepository{subscription: rehydrated(domain.StatusCancelled, &cancelledAt)}
	interactor := New(repository, outbox, fakeClock{cancelledAt.Add(10 * 24 * time.Hour)})

	_, err := interactor.Execute(context.Background(), Command{SubscriptionID: "sub-1"})
	if !errors.Is(err, domain.ErrAlreadyCancelled) {
		t.Fatalf("Execute() error = %v, want ErrAlreadyCancelled", err)
	}
	if repository.updated != nil || len(outbox.events) != 0 {
		t.Fatal("already cancelled subscription got mutations")
	}
}

func TestExecuteRefundCalculation(t *testing.T) {
	tests := []struct {
		name    string
		elapsed time.Duration
		want    int64
	}{
		{name: "immediate", elapsed: 0, want: 3000},
		{name: "one third used", elapsed: 10 * 24 * time.Hour, want: 2000},
		{name: "rounds down to a whole cent", elapsed: time.Second, want: 2999},
		{name: "whole period used", elapsed: domain.BillingPeriod, want: 0},
		{name: "after the period", elapsed: 31 * 24 * time.Hour, want: 0},
		{name: "canceled at before start", elapsed: -time.Second, want: 3000},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			outbox := &fakeOutbox{}
			repository := &fakeRepository{subscription: rehydrated(domain.StatusActive, nil)}
			interactor := New(repository, outbox, fakeClock{startedAt.Add(test.elapsed)})

			if _, err := interactor.Execute(context.Background(), Command{SubscriptionID: "sub-1"}); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			event := outbox.events[0].(domain.SubscriptionCancelledEvent)
			if event.RefundCents != test.want {
				t.Fatalf("refund = %d cents, want %d", event.RefundCents, test.want)
			}
		})
	}
}

func TestExecuteOutboxEventCreated(t *testing.T) {
	cancelledAt := startedAt.Add(10 * 24 * time.Hour)
	outbox := &fakeOutbox{}
	repository := &fakeRepository{subscription: rehydrated(domain.StatusActive, nil)}
	interactor := New(repository, outbox, fakeClock{cancelledAt})

	if _, err := interactor.Execute(context.Background(), Command{SubscriptionID: "sub-1"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(outbox.events) != 1 {
		t.Fatalf("outbox events = %d, want 1", len(outbox.events))
	}
	// 20 of 30 days remain.
	want := domain.SubscriptionCancelledEvent{
		EventID:        "sub-1:cancelled:2",
		SubscriptionID: "sub-1",
		CustomerID:     "customer-1",
		RefundCents:    2000,
		CancelledAt:    cancelledAt,
	}
	event, ok := outbox.events[0].(domain.SubscriptionCancelledEvent)
	if !ok || event != want {
		t.Fatalf("outbox event = %#v, want %#v", outbox.events[0], want)
	}
}

func rehydrated(status domain.Status, cancelledAt *time.Time) *domain.Subscription {
	return domain.Rehydrate(
		"sub-1",
		"customer-1",
		"test-plan",
		3000,
		status,
		startedAt,
		cancelledAt,
		1,
	)
}

type fakeClock struct{ now time.Time }

func (c fakeClock) Now() time.Time { return c.now }

type fakeRepository struct {
	subscription *domain.Subscription
	updated      *domain.Subscription
}

func (r *fakeRepository) GetByID(context.Context, domain.SubscriptionID) (*domain.Subscription, error) {
	return r.subscription, nil
}
func (r *fakeRepository) Add(*domain.Subscription) contracts.Mutation {
	panic("unexpected Add")
}
func (r *fakeRepository) Update(subscription *domain.Subscription) contracts.Mutation {
	r.updated = subscription
	return contracts.Mutation{Statement: "update " + string(subscription.ID())}
}

type fakeOutbox struct{ events []domain.Event }

func (o *fakeOutbox) Add(events []domain.Event) []contracts.Mutation {
	o.events = append(o.events, events...)
	mutations := make([]contracts.Mutation, 0, len(events))
	for _, event := range events {
		mutations = append(mutations, contracts.Mutation{Statement: "outbox " + event.ID()})
	}
	return mutations
}

func statements(mutations []contracts.Mutation) []string {
	labels := make([]string, 0, len(mutations))
	for _, mutation := range mutations {
		labels = append(labels, mutation.Statement)
	}
	return labels
}
