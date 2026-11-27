package openmeteo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type providerError struct {
	status int
	reason string
}

// Calls the endpoint up to MaxAttempts times. Temporary failures wait
// longer after each attempt; permanent errors stop immediately.
func (c *Client) requestWithRetries(ctx context.Context, params url.Values) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, c.options.RequestTimeout)
	defer cancel()
	endpoint := c.endpoint
	endpoint.RawQuery = params.Encode()
	delay := min(c.options.RetryDelay, c.options.RequestTimeout)
	address := endpoint.String()
	for attempt := 1; ; attempt++ {
		body, retryAfter, err := c.sendForecastRequest(ctx, address)
		if err == nil {
			return body, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if !isRetryableWeatherError(err) || attempt == c.options.MaxAttempts {
			return nil, fmt.Errorf("forecast request attempt %d: %w", attempt, err)
		}
		if err := waitForRetryOrCancellation(ctx, retryDelayWithJitter(delay, retryAfter)); err != nil {
			return nil, err
		}
		// Double the next delay, capped at the total request timeout.
		delay = min(delay, c.options.RequestTimeout/2) * 2
	}
}

// Picks a random wait between half and all of the current delay.
// With a 1s starting delay, retries wait 0.5–1s, then 1–2s.
// A longer Retry-After from the provider always takes priority.
func retryDelayWithJitter(delay, retryAfter time.Duration) time.Duration {
	jittered := time.Duration(float64(delay) * (0.5 + rand.Float64()*0.5))
	return max(jittered, retryAfter)
}

// Sends one GET with a timeout. It returns the response body or
// the provider error and how long the provider asks us to wait before retrying.
func (c *Client) sendForecastRequest(ctx context.Context, endpoint string) ([]byte, time.Duration, error) {
	ctx, cancel := context.WithTimeout(ctx, c.options.AttemptTimeout)
	defer cancel()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, 0, err
	}

	response, err := c.http.Do(request)
	if err != nil {
		return nil, 0, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, parseRetryAfter(response.Header.Get("Retry-After")), readProviderError(response)
	}

	const maxBytes = 8 << 20
	body, err := io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
	if len(body) > maxBytes {
		return nil, 0, fmt.Errorf("weather response too large")
	}

	return body, 0, err
}

// Reads the provider's wait time as seconds or an HTTP date.
// A missing or invalid value leaves the normal retry delay in use.
func parseRetryAfter(value string) time.Duration {
	if seconds, err := strconv.ParseInt(value, 10, 32); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	if at, err := http.ParseTime(value); err == nil {
		return time.Until(at)
	}
	return 0
}

// Pauses before the next attempt, stopping early if the request is cancelled.
func waitForRetryOrCancellation(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// Error includes the provider's status and reason for server logs.
func (e *providerError) Error() string {
	if e.reason != "" {
		return fmt.Sprintf("forecast provider HTTP %d: %s", e.status, e.reason)
	}
	return fmt.Sprintf("forecast provider HTTP %d", e.status)
}

// Identifies HTTP 408/429/5xx and temporary network failures worth retrying.
func isRetryableWeatherError(err error) bool {
	var provider *providerError
	if errors.As(err, &provider) {
		return provider.status == 408 || provider.status == 429 || provider.status >= 500 && provider.status <= 599
	}
	var network net.Error
	return errors.Is(err, context.DeadlineExceeded) || errors.Is(err, io.ErrUnexpectedEOF) || errors.As(err, &network) && (network.Timeout() || network.Temporary())
}

// Reads a bounded error reason from an unsuccessful API response.
func readProviderError(response *http.Response) error {
	var failure struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 2048)).Decode(&failure); err != nil {
		return &providerError{status: response.StatusCode}
	}
	if len(failure.Reason) > 512 {
		failure.Reason = failure.Reason[:512]
	}
	return &providerError{status: response.StatusCode, reason: failure.Reason}
}
