package refund_subscription

import (
	"context"
	"fmt"

	"subscriptionservice/internal/app/subscription/contracts"
	"subscriptionservice/internal/app/subscription/domain"
)

type Interactor struct {
	billing contracts.BillingClient
}

func New(billing contracts.BillingClient) *Interactor {
	return &Interactor{billing: billing}
}

func (i *Interactor) Execute(ctx context.Context, event domain.SubscriptionCancelledEvent) error {
	if event.RefundCents == 0 {
		return nil
	}
	err := i.billing.Refund(ctx, contracts.RefundRequest{
		IdempotencyKey: event.EventID,
		SubscriptionID: event.SubscriptionID,
		CustomerID:     event.CustomerID,
		AmountCents:    event.RefundCents,
	})
	if err != nil {
		return fmt.Errorf("refund subscription %s: %w", event.SubscriptionID, err)
	}
	return nil
}
