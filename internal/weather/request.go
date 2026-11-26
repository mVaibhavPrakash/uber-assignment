package weather

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"mobilitycontrolroom/internal/appErrors"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Calls the endpoint up to MaxAttempts times. Temporary failures wait
// longer after each attempt; permanent errors stop immediately.
func (c *Client) requestWithRetries(ctx context.Context, params url.Values) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, c.config.ExternalRequestTimeout)
	defer cancel()
	endpoint, err := url.Parse(c.config.ForecastURL)
	if err != nil {
		return nil, err
	}
	endpoint.RawQuery = params.Encode()
	delay := min(c.config.RetryDelay, c.config.ExternalRequestTimeout)
	for attempt := 1; attempt <= c.config.MaxAttempts; attempt++ {
		body, retryAfter, err := c.sendForecastRequest(ctx, endpoint.String())
		if err == nil {
			return body, nil
		}
		slog.Warn("weather request failed", "attempt", attempt, "error", err)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if !appErrors.IsRetryableWeatherError(err) || attempt == c.config.MaxAttempts {
			return nil, fmt.Errorf("fetch weather: %w", err)
		}
		if err := waitForRetryOrCancellation(ctx, retryDelayWithJitter(delay, retryAfter)); err != nil {
			return nil, err
		}
		// Double the next delay, capped at the total request timeout.
		delay = min(delay, c.config.ExternalRequestTimeout/2) * 2
	}
	return nil, fmt.Errorf("weather attempts must be positive")
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
	ctx, cancel := context.WithTimeout(ctx, c.config.HTTPAttemptTimeout)
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
		return nil, parseRetryAfter(response.Header.Get("Retry-After")), appErrors.ReadProviderError(response)
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
