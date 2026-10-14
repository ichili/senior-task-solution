package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"subscriptionservice/internal/app/subscription/contracts"
	"subscriptionservice/internal/app/subscription/domain"
)

type BillingClient struct {
	baseURL *url.URL
	client  *http.Client
}

var _ contracts.BillingClient = (*BillingClient)(nil)

func NewBillingHTTPClient(rawBaseURL string, client *http.Client) (*BillingClient, error) {
	baseURL, err := url.Parse(rawBaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse billing base URL: %w", err)
	}

	if baseURL.Scheme != "http" && baseURL.Scheme != "https" || baseURL.Host == "" {
		return nil, errors.New("billing base URL must be an HTTP(S) URL")
	}

	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	return &BillingClient{baseURL: baseURL, client: client}, nil
}

func (c *BillingClient) ValidateCustomer(ctx context.Context, customerID domain.CustomerID) (bool, error) {
	id := url.PathEscape(string(customerID))
	endpoint := c.baseURL.JoinPath("validate", id)

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return false, fmt.Errorf("validate customer: %w", err)
	}
	request.Header.Set("Accept", "application/json")

	var result struct {
		Valid *bool `json:"valid"`
	}
	if err := c.send(request, &result); err != nil {
		return false, fmt.Errorf("validate customer: %w", err)
	}
	if result.Valid == nil {
		return false, errors.New("validate customer: response has no valid field")
	}

	return *result.Valid, nil
}

func (c *BillingClient) Refund(ctx context.Context, refund contracts.RefundRequest) error {
	endpoint := c.baseURL.JoinPath("refund")

	payload, err := json.Marshal(struct {
		SubscriptionID string `json:"subscription_id"`
		CustomerID     string `json:"customer_id"`
		AmountCents    int64  `json:"amount_cents"`
	}{string(refund.SubscriptionID), string(refund.CustomerID), refund.AmountCents})
	if err != nil {
		return fmt.Errorf("refund: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("refund: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Idempotency-Key", refund.IdempotencyKey)

	if err := c.send(request, nil); err != nil {
		return fmt.Errorf("refund: %w", err)
	}
	return nil
}

func (c *BillingClient) send(request *http.Request, target any) error {
	response, err := c.client.Do(request)
	if err != nil {
		return err
	}
	defer closeResponse(response.Body)

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("billing returned %s", response.Status)
	}

	if target == nil {
		return nil
	}

	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func closeResponse(body io.ReadCloser) {
	_, _ = io.Copy(io.Discard, io.LimitReader(body, 1<<20))
	_ = body.Close()
}
