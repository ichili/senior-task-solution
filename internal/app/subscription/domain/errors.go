package domain

import "errors"

var (
	ErrInvalidSubscriptionID = errors.New("subscription id is required")
	ErrInvalidCustomerID     = errors.New("customer id is required")
	ErrInvalidPlanID         = errors.New("plan id is required")
	ErrInvalidPrice          = errors.New("price cannot be negative")
	ErrAlreadyCancelled      = errors.New("subscription is already cancelled")
)
