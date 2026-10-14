## Q1: Where should the refund HTTP call happen?
### Answer: D. A separate use case triggered by `SubscriptionCancelledEvent`.

Cancel usecase should focus on business logic of cancelling the subscription. Actual refund processing is more of a side effect for the Cancel usecase.
Domain `Cancel()` method should remain a pure aggregate operation: validate the transition, calculate the refund, change status and record an event.
Service layer manages DB operations to ensure that all mutations are saved atomically.
After SubscriptionCancelledEvent is saved to outbox it is then delivered via relay\broker to a refund usecase, which handles the refund according to its own rules, like retry policy, idempotency etc.

## Q2: What happens when billing is down?

The subscription should remain CANCELLED, since refund processing is a separate operation. The refund status should stay PENDING while the system is trying to process it.
I would retry only temporary failures, like timeouts, connection issues, HTTP 429 or 5xx errors.
We can use exponential backoff, for example, 3 retries after 1, 5 and 30 minutes. Each retry should use the same idempotency key to avoid duplicate refunds.
If all retries fail, I would mark the refund as FAILED and move the message to a dead-letter queue for further investigation or manual retry. We should also notify the support or operations team.
The subscription should not be reverted to ACTIVE just because the refund failed.

## Q3: Time and money

time.Since() is not a good choice here because it depends on the current system time. This makes the calculation harder to test and can produce different results if the refund calculation is processed at different point in time. It's better to inject a clock into the use case.
Using floats for money is also very risky because of rounding errors. It’s better to store amounts as int64 cents and use integer arithmetic.

```go
type Clock interface {
    Now() time.Time
}

// example defund calculation
func calculateRefund(priceCents int64, startedAt, cancelledAt time.Time) int64 {
    const billingPeriod = 30 * 24 * time.Hour

    totalSeconds := int64(billingPeriod.Seconds())
    usedSeconds := int64(cancelledAt.Sub(startedAt).Seconds())

    if usedSeconds < 0 {
        usedSeconds = 0
    }
    if usedSeconds >= totalSeconds {
        return 0
    }

    return priceCents * (totalSeconds - usedSeconds) / totalSeconds
}

// In the use case
now := i.clock.Now()
if err := subscription.Cancel(now); err != nil {
    return nil, err
}
```
This way, the calculation is predictable, easier to test, and avoids floating-point rounding issues. Integer division rounds down to the nearest cent.

## Q4: `CancelSubscription` tests

The use case doesn't write to the database itself. It only returns a list of mutations, and the service layer saves them in one transaction. So the tests just check which mutations the use case returns.

What to mock:
- Subscription repository: returns a test subscription and remembers what was passed to `Update`.
- Outbox: remembers which events it received.
- Clock: returns a fixed time, so the refund is always the same.

What to assert:
- Success path: no error, status is CANCELLED, and two mutations are returned: the subscription update and the outbox event.
- Already cancelled: the error is `ErrAlreadyCancelled`, and nothing was sent to the repository or the outbox.
- Refund calculation: different cancel times give the expected refund.
- Outbox event: exactly one `SubscriptionCancelledEvent` with the correct fields.

The tests are in `internal/app/subscription/usecases/cancel_subscription/interactor_test.go` and use only the standard library. Short version:

```go
func TestExecuteSuccess(t *testing.T) {
    repository := &fakeRepository{subscription: rehydrated(domain.StatusActive, nil)} // price $30.00
    interactor := New(repository, &fakeOutbox{}, fakeClock{startedAt.Add(10 * 24 * time.Hour)})

    result, err := interactor.Execute(ctx, Command{SubscriptionID: "sub-1"})

    // err == nil; status == CANCELLED
	// there are only to mutations: update subscription with id sub-1, outbox cancelled event
}

func TestExecuteAlreadyCancelled(t *testing.T) {
    // Mock a CANCELLED subscription then run Execute
    // errors.Is(err, domain.ErrAlreadyCancelled)
    // repository.updated == nil, outbox.events is empty
}

func TestExecuteRefundCalculation(t *testing.T) {
    // Set different cancel times on the fake clock and check the refund in the event:
    // 0 seconds used -> refund $30.00
	// 1 second-> $29.99 (rounded down)
	// 10 days -> $20.00
	// 30 days -> $0
    // 31 days (more than billing period) -> $0
	// -1 second (cancel time before start) → $30.00
}

func TestExecuteOutboxEventCreated(t *testing.T) {
    // Cancel after 10 days, then:
    // outbox.events holds exactly one domain.SubscriptionCancelledEvent equal to
    // {EventID: "sub-1:cancelled:2", SubscriptionID: "sub-1", CustomerID: "customer-1",
    //  RefundCents: 2000 (20 of 30 days remain), CancelledAt: the fake clock's time}
}
```

## Q5: The business consequence of ignored validation errors

A technical failure is converted into a business answer. With a failed request, malformed body, or billing 500, the zero value of `Valid` is false, so legitimate customers are rejected as though billing had authoritatively declared them invalid. That creates lost sales, false fraud/support signals, misleading metrics, and retry behavior that cannot distinguish an outage from a real rejection. Dependency failure must be a distinct error (for example, `billing is unavailable`) so callers can retry or return a temporary-service response without changing business truth.

The implementation keeps the two outcomes apart. Any failure to get an answer (billing unreachable, a non-2xx status, or a body without a `valid` field) becomes `ErrBillingUnavailable` wrapping the cause, which a transport would map to 503 so the client may retry. Only billing's answer `valid: false` becomes `ErrInvalidCustomer`, a 422.
