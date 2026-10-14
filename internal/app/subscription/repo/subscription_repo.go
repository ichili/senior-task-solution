package repo

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"subscriptionservice/internal/app/subscription/contracts"
	"subscriptionservice/internal/app/subscription/domain"
)

type SubscriptionRepository struct {
	db *sql.DB
}

var _ contracts.SubscriptionRepository = (*SubscriptionRepository)(nil)

func NewSubscriptionRepository(db *sql.DB) *SubscriptionRepository {
	return &SubscriptionRepository{db: db}
}

func (r *SubscriptionRepository) GetByID(ctx context.Context, id domain.SubscriptionID) (*domain.Subscription, error) {
	const query = `
		SELECT
		    customer_id,
		    plan_id,
		    price_cents,
		    status,
		    start_date,
		    cancelled_at,
		    version
		FROM subscriptions
		WHERE id = ?
	`

	var (
		customerID  domain.CustomerID
		planID      domain.PlanID
		priceCents  int64
		status      domain.Status
		startedAt   time.Time
		cancelledAt sql.NullTime
		version     int64
	)
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&customerID,
		&planID,
		&priceCents,
		&status,
		&startedAt,
		&cancelledAt,
		&version,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, contracts.ErrSubscriptionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query subscription: %w", err)
	}

	var cancelled *time.Time
	if cancelledAt.Valid {
		cancelled = &cancelledAt.Time
	}

	return domain.Rehydrate(id, customerID, planID, priceCents, status, startedAt, cancelled, version), nil
}

func (r *SubscriptionRepository) Add(s *domain.Subscription) contracts.Mutation {
	const stmt = `
		INSERT INTO subscriptions
		(id, customer_id, plan_id, price_cents, status, start_date, cancelled_at, version)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	return contracts.Mutation{
		Statement: stmt,
		Args: []any{
			s.ID(), s.CustomerID(), s.PlanID(), s.PriceCents(),
			s.Status(), s.StartDate(), s.CancelledAt(), s.Version(),
		},
	}
}

func (r *SubscriptionRepository) Update(subscription *domain.Subscription) contracts.Mutation {
	const template = `
		UPDATE subscriptions 
		SET %s 
		WHERE id = ? 
		  AND version = ?
	`

	var (
		assignments []string
		args        []any
	)

	set := func(column string, value any) {
		assignments = append(assignments, column+" = ?")
		args = append(args, value)
	}

	if subscription.Changed(domain.FieldStatus) {
		set("status", subscription.Status())
	}
	if subscription.Changed(domain.FieldCancelledAt) {
		set("cancelled_at", subscription.CancelledAt())
	}
	set("version", subscription.Version())

	args = append(args, subscription.ID(), subscription.LoadedVersion())
	query := fmt.Sprintf(template, strings.Join(assignments, ", "))

	return contracts.Mutation{
		Statement: query,
		Args:      args,
		Conflict:  contracts.ErrConcurrentMutation,
	}
}

// PlanRepository reads prices from the plans table.
type PlanRepository struct {
	db *sql.DB
}

var _ contracts.PlanRepository = (*PlanRepository)(nil)

func NewPlanRepository(db *sql.DB) *PlanRepository {
	return &PlanRepository{db: db}
}

func (r *PlanRepository) PriceCents(ctx context.Context, planID domain.PlanID) (int64, error) {
	var priceCents int64
	err := r.db.
		QueryRowContext(ctx, `SELECT price_cents FROM plans WHERE id = ?`, planID).
		Scan(&priceCents)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, contracts.ErrUnknownPlan
	}
	if err != nil {
		return 0, fmt.Errorf("query plan price: %w", err)
	}
	return priceCents, nil
}

type Outbox struct{}

var _ contracts.Outbox = Outbox{}

func (Outbox) Add(events []domain.Event) []contracts.Mutation {
	const query = `
		INSERT INTO outbox_events
		(event_id, aggregate_id, event_type, payload, occurred_at)
		VALUES (?, ?, ?, ?, ?)
	`

	mutations := make([]contracts.Mutation, 0, len(events))
	for _, event := range events {
		mutations = append(mutations, contracts.Mutation{
			Statement: query,
			Args: []any{event.ID(), event.AggregateID(), event.Name(),
				jsonPayload{event}, event.OccurredAt()},
		})
	}
	return mutations
}

// jsonPayload is a helper for storing event as json in DB column.
type jsonPayload struct{ event domain.Event }

var _ driver.Valuer = jsonPayload{}

func (p jsonPayload) Value() (driver.Value, error) {
	payload, err := json.Marshal(p.event)
	if err != nil {
		return nil, err
	}
	return string(payload), nil
}

// Committer applies mutations in one database transaction.
type Committer struct {
	db *sql.DB
}

var _ contracts.Committer = (*Committer)(nil)

func NewCommitter(db *sql.DB) *Committer {
	return &Committer{db: db}
}

func (c *Committer) Apply(ctx context.Context, mutations ...contracts.Mutation) error {
	if len(mutations) == 0 {
		return nil
	}

	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		// Rollback error is ignored here because observability is out of scope for this task
		// and if transaction is already commited, rollback does nothing.
		_ = tx.Rollback()
	}()

	for i, mutation := range mutations {
		result, err := tx.ExecContext(ctx, mutation.Statement, mutation.Args...)
		if err != nil {
			return fmt.Errorf("apply mutation %d: %w", i+1, err)
		}

		if mutation.Conflict == nil {
			continue
		}

		affected, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("apply mutation %d: %w", i+1, err)
		}
		if affected != 1 {
			return mutation.Conflict
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}
