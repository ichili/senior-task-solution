package domain

import (
	"errors"
	"slices"
	"strings"
	"time"
	"uuid"
)

// BillingPeriod is fixed to not widen the scope of solution.
// Generally, it should be a property of plan.
const BillingPeriod = 30 * 24 * time.Hour

type Subscription struct {
	id          SubscriptionID
	customerID  CustomerID
	planID      PlanID
	priceCents  int64
	status      Status
	startDate   time.Time
	cancelledAt *time.Time
	version     int64
	// Change tracking for optimistic locking
	loadedVersion int64
	changed       Field
	events        []Event
}

func NewSubscription(customerID CustomerID, planID PlanID, priceCents int64, startedAt time.Time) (*Subscription, error) {
	err := errors.Join(
		customerID.Validate(),
		planID.Validate(),
		validatePrice(priceCents),
	)
	if err != nil {
		return nil, err
	}

	startedAt = startedAt.UTC()
	s := &Subscription{
		id:         SubscriptionID(uuid.NewV4().String()),
		customerID: customerID,
		planID:     planID,
		priceCents: priceCents,
		status:     StatusActive,
		startDate:  startedAt,
		version:    1,
	}

	s.events = append(s.events, newSubscriptionCreatedEvent(s))
	return s, nil
}

func (s *Subscription) ID() SubscriptionID     { return s.id }
func (s *Subscription) CustomerID() CustomerID { return s.customerID }
func (s *Subscription) PlanID() PlanID         { return s.planID }
func (s *Subscription) PriceCents() int64      { return s.priceCents }
func (s *Subscription) Status() Status         { return s.status }
func (s *Subscription) StartDate() time.Time   { return s.startDate }
func (s *Subscription) Version() int64         { return s.version }
func (s *Subscription) LoadedVersion() int64   { return s.loadedVersion }

// Changed reports if the field was changed since the aggregate was created or loaded.
func (s *Subscription) Changed(fields Field) bool { return s.changed&fields != 0 }

func (s *Subscription) CancelledAt() *time.Time {
	if s.cancelledAt == nil {
		return nil
	}
	return new(*s.cancelledAt)
}

// PendingEvents returns a copy of the events recorded since the aggregate was
// created or loaded.
func (s *Subscription) PendingEvents() []Event { return slices.Clone(s.events) }

// Cancel changes state and records the event that will initiate the refund.
func (s *Subscription) Cancel(cancelledAt time.Time) error {
	if s.status == StatusCancelled {
		return ErrAlreadyCancelled
	}

	cancelledAt = cancelledAt.UTC()
	refundCents := calculateRefund(s.priceCents, s.startDate, cancelledAt)
	s.status = StatusCancelled
	s.cancelledAt = &cancelledAt
	s.changed |= FieldStatus | FieldCancelledAt
	s.version++
	s.events = append(s.events, newSubscriptionCancelledEvent(s, refundCents))
	return nil
}

type Status string

const (
	StatusActive    Status = "ACTIVE"
	StatusCancelled Status = "CANCELLED"
)

// Distinct ID types let the compiler catch one ID passed in place of another.
type (
	SubscriptionID string
	CustomerID     string
	PlanID         string
)

func (id SubscriptionID) Validate() error {
	if strings.TrimSpace(string(id)) == "" {
		return ErrInvalidSubscriptionID
	}
	return nil
}

func (id CustomerID) Validate() error {
	if strings.TrimSpace(string(id)) == "" {
		return ErrInvalidCustomerID
	}
	return nil
}

func (id PlanID) Validate() error {
	if strings.TrimSpace(string(id)) == "" {
		return ErrInvalidPlanID
	}
	return nil
}

// Field names an aggregate field that can change after creation. Fields are bit
// flags, but can also be implemented by using list.
type Field uint8

const (
	FieldStatus Field = 1 << iota
	FieldCancelledAt
)

// Rehydrate reconstructs stored state without emitting events.
func Rehydrate(
	id SubscriptionID,
	customerID CustomerID,
	planID PlanID,
	priceCents int64,
	status Status,
	startedAt time.Time,
	cancelledAt *time.Time,
	version int64,
) *Subscription {

	s := &Subscription{
		id:            id,
		customerID:    customerID,
		planID:        planID,
		priceCents:    priceCents,
		status:        status,
		startDate:     startedAt.UTC(),
		version:       version,
		loadedVersion: version,
	}
	if cancelledAt != nil {
		s.cancelledAt = new(cancelledAt.UTC())
	}
	return s
}

// calculateRefund returns the unused portion of a billing period, in cents.
func calculateRefund(priceCents int64, startedAt, cancelledAt time.Time) int64 {
	totalSeconds := int64(BillingPeriod.Seconds())
	usedSeconds := int64(cancelledAt.Sub(startedAt).Seconds())
	usedSeconds = max(0, usedSeconds) // guard against cancelledAt being earlier than startedAt
	if usedSeconds >= totalSeconds {
		return 0
	}
	return priceCents * (totalSeconds - usedSeconds) / totalSeconds
}

func validatePrice(cents int64) error {
	if cents < 0 {
		return ErrInvalidPrice
	}
	return nil
}
