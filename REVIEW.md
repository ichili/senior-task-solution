## Issue 1: The usecase depends directly on `database/sql`
- Category: Layer Violation
- Severity: CRITICAL
- Problem: `SubscriptionService` holds a `*sql.DB` and writes SQL instead of using a repository interface.
- Why it matters: Business logic depends on database details. Storage changes also require changes to the usecase.

## Issue 2: The usecase controls when changes are saved
- Category: Layer Violation
- Severity: CRITICAL
- Problem: The usecase saves changes immediately instead of leaving mutation application to the service layer.
- Why it matters: The service layer can't commit the subscription change and an outbox event together.

## Issue 3: The usecase implements billing client 
- Category: Layer Violation
- Severity: CRITICAL
- Problem: The usecase uses `*http.Client`, knows billing URLs, and builds and parses billing's request and response formats.
- Why it matters: This belongs to an adapter. Changes to the billing API shouldn't require changes to business logic.

## Issue 4: The refund is sent before cancellation is saved
- Category: Layer Violation
- Severity: CRITICAL
- Problem: `CancelSubscription` sends the refund before updating the database, with no saved refund event.
- Why it matters: If the refund succeeds but the update fails, a retry can refund the customer again. Send the refund after commit from the service layer, or process a saved event.

## Issue 5: The aggregate doesn't protect its state
- Category: Domain Purity
- Severity: CRITICAL
- Problem: All `Subscription` fields are exported, with no domain checks for required customer and plan IDs.
- Why it matters: Any code can create an invalid subscription or change its state without following the rules.

## Issue 6: Status is an unconstrained string
- Category: Domain Purity
- Severity: WARNING
- Problem: `Status` accepts any string, including typos and unknown values.
- Why it matters: Cancellation only rejects `CANCELLED`, so an invalid status can still trigger a refund.

## Issue 7: Cancellation rules live in the service
- Category: Domain Purity
- Severity: WARNING
- Problem: The cancellation check and refund calculation are in the service instead of a domain method.
- Why it matters: Another cancellation flow could duplicate these rules and behave differently.

## Issue 8: Customer validation ignores failures
- Category: Error Handling
- Severity: CRITICAL
- Problem: The `Get` error, HTTP status, and JSON decode error are ignored. A nil `resp` causes a panic at `defer resp.Body.Close()`.
- Why it matters: A billing failure can be reported as "invalid customer", while an error response containing `Valid: true` can pass validation.

## Issue 9: Database scan errors are ignored
- Category: Error Handling
- Severity: CRITICAL
- Problem: `row.Scan` errors are ignored, so cancellation continues with an empty or partially filled subscription.
- Why it matters: The code can calculate a refund from invalid data or send a refund for a subscription that wasn't found.

## Issue 10: Refund errors and responses are ignored
- Category: Error Handling
- Severity: CRITICAL
- Problem: JSON marshaling and HTTP errors are ignored. The response status is not checked, and the response body is not closed.
- Why it matters: Cancellation can report success when billing rejected the refund. Unclosed response bodies can also leak resources and prevent connection reuse.

## Issue 11: Errors are hard to handle reliably
- Category: Error Handling
- Severity: WARNING
- Problem: Business errors use fresh `errors.New` values, and database errors are returned without context.
- Why it matters: Callers have to compare messages to identify business errors, and logs don't clearly show which operation failed.

## Issue 12: Money uses floats
- Category: Money Handling
- Severity: CRITICAL
- Problem: Prices and refunds use `float64`. Currency is also absent from the model and refund request.
- Why it matters: Float rounding can produce the wrong number of cents. Use integer minor units and define the currency in the data or a shared contract.

## Issue 13: The refund has no bounds or rounding rule
- Category: Money Handling
- Severity: CRITICAL
- Problem: The formula gives a negative refund after 30 days and more than the price for a future start date. It has no rounding rule, and the period is fixed at 30 days.
- Why it matters: Billing can receive an invalid or excessive refund. Bound the amount and define how the billing period, partial days, and rounding should work.

## Issue 14: The refund request doesn't identify the payment
- Category: Money Handling
- Severity: CRITICAL
- Problem: The request contains only `amount`, with no payment, subscription, or customer ID.
- Why it matters: The request gives billing no way to identify which payment to refund.

## Issue 15: The refund amount can change on retry
- Category: Money Handling
- Severity: CRITICAL
- Problem: Each cancellation attempt uses `time.Since(sub.StartDate)`, so a retry calculates the refund at a later time.
- Why it matters: Retrying the same cancellation can send a different refund amount. Save the original cancellation time or refund amount and reuse it on retries.

## Issue 16: Business logic reads the system clock directly
- Category: Testability
- Severity: WARNING
- Problem: The code calls `time.Now()` and `time.Since()` directly.
- Why it matters: Tests can't supply a fixed time to check exact refund amounts and date boundaries.

## Issue 17: Business rules are coupled to database and HTTP calls
- Category: Testability
- Severity: WARNING
- Problem: Testing the refund calculation or cancellation check requires going through SQL and HTTP code.
- Why it matters: Tests need SQL and HTTP stubs instead of simple domain inputs and fake dependencies. Moving the rules into the domain makes them easier to test.

## Issue 18: External configuration is hard-coded
- Category: Other
- Severity: WARNING
- Problem: Billing URLs are hard-coded in the business code.
- Why it matters: Switching billing endpoints requires a code change instead of a configuration change.

## Issue 19: HTTP requests ignore the caller's context
- Category: Other
- Severity: WARNING
- Problem: `Get` and `Post` don't pass `ctx` to the request.
- Why it matters: Billing calls keep running after the caller cancels or its deadline expires. A client timeout, if configured elsewhere, doesn't fix this.

## Issue 20: Refund calls have no idempotency key
- Category: Other
- Severity: CRITICAL
- Problem: The refund request has no key that billing can use to recognize a retry.
- Why it matters: After a timeout, retrying can refund twice, while not retrying can leave the refund unpaid.

## Issue 21: SQL depends on column order
- Category: Other
- Severity: CRITICAL
- Problem: `INSERT INTO subscriptions VALUES (...)` has no column list, and `SELECT *` is read by position with `row.Scan`.
- Why it matters: Schema changes can break reads and writes or put values in the wrong fields. List the columns explicitly.

## Issue 22: Create has no retry protection
- Category: Other
- Severity: CRITICAL
- Problem: Each call generates a new ID, with no request key or lookup to recognize a retry.
- Why it matters: A retry after a timeout can create another subscription unless a database constraint prevents it. Even then, the code doesn't return the original result.
