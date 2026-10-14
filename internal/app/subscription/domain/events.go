package domain

import (
	"fmt"
	"time"
)

const (
	SubscriptionCreatedEventName   = "subscription.created.v1"
	SubscriptionCancelledEventName = "subscription.cancelled.v1"
)

type Event interface {
	ID() string
	Name() string
	AggregateID() SubscriptionID
	OccurredAt() time.Time
}

type SubscriptionCreatedEvent struct {
	EventID        string         `json:"event_id"`
	SubscriptionID SubscriptionID `json:"subscription_id"`
	CustomerID     CustomerID     `json:"customer_id"`
	PlanID         PlanID         `json:"plan_id"`
	PriceCents     int64          `json:"price_cents"`
	StartedAt      time.Time      `json:"started_at"`
}

var _ Event = SubscriptionCreatedEvent{}

func newSubscriptionCreatedEvent(s *Subscription) SubscriptionCreatedEvent {
	return SubscriptionCreatedEvent{
		EventID:        fmt.Sprintf("%s:created:%d", s.id, s.version),
		SubscriptionID: s.id,
		CustomerID:     s.customerID,
		PlanID:         s.planID,
		PriceCents:     s.priceCents,
		StartedAt:      s.startDate,
	}
}

func (e SubscriptionCreatedEvent) ID() string                  { return e.EventID }
func (e SubscriptionCreatedEvent) Name() string                { return SubscriptionCreatedEventName }
func (e SubscriptionCreatedEvent) AggregateID() SubscriptionID { return e.SubscriptionID }
func (e SubscriptionCreatedEvent) OccurredAt() time.Time       { return e.StartedAt }

type SubscriptionCancelledEvent struct {
	EventID        string         `json:"event_id"`
	SubscriptionID SubscriptionID `json:"subscription_id"`
	CustomerID     CustomerID     `json:"customer_id"`
	RefundCents    int64          `json:"refund_cents"`
	CancelledAt    time.Time      `json:"cancelled_at"`
}

var _ Event = SubscriptionCancelledEvent{}

func newSubscriptionCancelledEvent(s *Subscription, refundCents int64) SubscriptionCancelledEvent {
	return SubscriptionCancelledEvent{
		EventID:        fmt.Sprintf("%s:cancelled:%d", s.id, s.version),
		SubscriptionID: s.id,
		CustomerID:     s.customerID,
		RefundCents:    refundCents,
		CancelledAt:    *s.cancelledAt,
	}
}

func (e SubscriptionCancelledEvent) ID() string                  { return e.EventID }
func (e SubscriptionCancelledEvent) Name() string                { return SubscriptionCancelledEventName }
func (e SubscriptionCancelledEvent) AggregateID() SubscriptionID { return e.SubscriptionID }
func (e SubscriptionCancelledEvent) OccurredAt() time.Time       { return e.CancelledAt }
