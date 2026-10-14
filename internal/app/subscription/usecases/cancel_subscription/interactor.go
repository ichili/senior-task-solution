package cancel_subscription

import (
	"context"
	"fmt"
	"time"

	"subscriptionservice/internal/app/subscription/contracts"
	"subscriptionservice/internal/app/subscription/domain"
)

type Command struct {
	SubscriptionID domain.SubscriptionID
}

type Result struct {
	Subscription *domain.Subscription
	Mutations    []contracts.Mutation
}

type Clock interface {
	Now() time.Time
}

type Interactor struct {
	repository contracts.SubscriptionRepository
	outbox     contracts.Outbox
	clock      Clock
}

func New(repository contracts.SubscriptionRepository, outbox contracts.Outbox, clock Clock) *Interactor {
	return &Interactor{repository: repository, outbox: outbox, clock: clock}
}

func (i *Interactor) Execute(ctx context.Context, command Command) (*Result, error) {
	if err := command.SubscriptionID.Validate(); err != nil {
		return nil, fmt.Errorf("cancel subscription: %w", err)
	}
	subscription, err := i.repository.GetByID(ctx, command.SubscriptionID)
	if err != nil {
		return nil, fmt.Errorf("get subscription: %w", err)
	}
	if err := subscription.Cancel(i.clock.Now()); err != nil {
		return nil, fmt.Errorf("cancel subscription: %w", err)
	}
	return &Result{
		Subscription: subscription,
		Mutations: append([]contracts.Mutation{
			i.repository.Update(subscription)},
			i.outbox.Add(subscription.PendingEvents())...,
		),
	}, nil
}
