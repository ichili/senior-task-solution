package contracts

import (
	"context"
	"errors"

	"subscriptionservice/internal/app/subscription/domain"
)

var (
	ErrSubscriptionNotFound = errors.New("subscription not found")
	ErrConcurrentMutation   = errors.New("subscription was changed concurrently")
	ErrUnknownPlan          = errors.New("plan is unknown")
)

type Mutation struct {
	Statement string
	Args      []any
	// Conflict is the error that should be returned if mutation's statement
	// affected more then 1 row.
	Conflict error
}

type Committer interface {
	Apply(ctx context.Context, mutations ...Mutation) error
}

type SubscriptionRepository interface {
	GetByID(ctx context.Context, id domain.SubscriptionID) (*domain.Subscription, error)
	Add(subscription *domain.Subscription) Mutation
	Update(subscription *domain.Subscription) Mutation
}

// PlanRepository is the plan catalog, the only source of prices.
type PlanRepository interface {
	PriceCents(ctx context.Context, planID domain.PlanID) (int64, error)
}

type Outbox interface {
	Add(events []domain.Event) []Mutation
}
