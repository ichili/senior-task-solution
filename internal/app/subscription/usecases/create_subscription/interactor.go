package create_subscription

import (
	"context"
	"errors"
	"fmt"
	"time"

	"subscriptionservice/internal/app/subscription/contracts"
	"subscriptionservice/internal/app/subscription/domain"
)

var (
	ErrInvalidCustomer    = errors.New("customer is invalid")
	ErrBillingUnavailable = errors.New("billing is unavailable")
)

type Command struct {
	CustomerID domain.CustomerID
	PlanID     domain.PlanID
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
	plans      contracts.PlanRepository
	outbox     contracts.Outbox
	billing    contracts.BillingClient
	clock      Clock
}

func New(
	repository contracts.SubscriptionRepository,
	plans contracts.PlanRepository,
	outbox contracts.Outbox,
	billing contracts.BillingClient,
	clock Clock,
) *Interactor {
	return &Interactor{
		repository: repository,
		plans:      plans,
		outbox:     outbox,
		billing:    billing,
		clock:      clock,
	}
}

func (i *Interactor) Execute(ctx context.Context, command Command) (*Result, error) {
	if err := errors.Join(command.CustomerID.Validate(), command.PlanID.Validate()); err != nil {
		return nil, fmt.Errorf("create subscription: %w", err)
	}

	// The price comes from the plan catalog, never from the caller.
	// Task doesn't specify if caller is trusted or not.
	priceCents, err := i.plans.PriceCents(ctx, command.PlanID)
	if err != nil {
		return nil, fmt.Errorf("get plan price: %w", err)
	}

	subscription, err := domain.NewSubscription(
		command.CustomerID,
		command.PlanID,
		priceCents,
		i.clock.Now(),
	)
	if err != nil {
		return nil, fmt.Errorf("create subscription: %w", err)
	}

	// An outage is not an answer about the customer: it must not turn into
	// ErrInvalidCustomer.
	valid, err := i.billing.ValidateCustomer(ctx, command.CustomerID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBillingUnavailable, err)
	}
	if !valid {
		return nil, ErrInvalidCustomer
	}

	return &Result{
		Subscription: subscription,
		Mutations:    append([]contracts.Mutation{i.repository.Add(subscription)}, i.outbox.Add(subscription.PendingEvents())...),
	}, nil
}
