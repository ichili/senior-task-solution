package contracts

import (
	"context"

	"subscriptionservice/internal/app/subscription/domain"
)

type RefundRequest struct {
	IdempotencyKey string
	SubscriptionID domain.SubscriptionID
	CustomerID     domain.CustomerID
	AmountCents    int64
}

type BillingClient interface {
	ValidateCustomer(ctx context.Context, customerID domain.CustomerID) (bool, error)
	Refund(ctx context.Context, refund RefundRequest) error
}
