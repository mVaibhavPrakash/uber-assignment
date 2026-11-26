// Package appErrors keeps application error definitions and handling in one place.
// It does not depend on business services or repositories.
package appErrors

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const XRequestId = "X-Request-ID"

var (
	ErrNoSnapshot         = errors.New("no forecast snapshot")
	ErrCommitAmbiguous    = errors.New("forecast commit outcome unknown")
	ErrForecastExpired    = errors.New("forecast expired")
	ErrForecastCoverage   = errors.New("forecast does not cover the current recommendation window")
	ErrWeatherRefreshBusy = errors.New("forecast refresh already running")
)

// HTTPError defines the status, safe code and message returned to API callers.
type HTTPError struct {
	Status  int
	Code    string
	Message string
}

var (
	QueryNotAllowed            = HTTPError{400, "INVALID_QUERY", "This endpoint does not accept query parameters."}
	UnknownQuery               = HTTPError{400, "INVALID_QUERY", "Unknown query parameter."}
	InvalidAreaID              = HTTPError{400, "INVALID_AREA_ID", "Area ID must be a positive integer."}
	AreaNotFound               = HTTPError{404, "AREA_NOT_FOUND", "Area is not in the configured scope."}
	CurrentInsightsUnavailable = HTTPError{503, "RECOMMENDATION_UNAVAILABLE", "Current insights are unavailable."}
	InsightsUnavailable        = HTTPError{503, "INSIGHTS_UNAVAILABLE", "Area evidence is unavailable."}
	SnapshotChanged            = HTTPError{409, "SNAPSHOT_CHANGED", "Recommendation evidence changed; refresh recommendations."}
	InvalidWindowStart         = HTTPError{400, "INVALID_QUERY", "window_start must be an RFC3339 timestamp."}
	WindowChanged              = HTTPError{409, "SNAPSHOT_CHANGED", "Planning window changed; refresh recommendations."}
	InternalFailure            = HTTPError{500, "INTERNAL_ERROR", "Unable to complete this request."}
	RecommendationUnavailable  = HTTPError{503, "RECOMMENDATION_UNAVAILABLE", "A current, complete forecast and recommendation are not available."}
	ForecastUnavailable        = HTTPError{503, "FORECAST_UNAVAILABLE", "Complete forecast coverage is unavailable."}
)

// WriteHTTPError sends a safe JSON error with its request ID and no recommended action.
func WriteHTTPError(w http.ResponseWriter, failure HTTPError) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(failure.Status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error":                  map[string]string{"code": failure.Code, "message": failure.Message, "request_id": w.Header().Get(XRequestId)},
		"primary_recommendation": nil,
	})
}

// WithRequestRecovery adds request IDs and turns unexpected panics into safe errors.
func WithRequestRecovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(XRequestId, uuid.NewString())
		defer func() {
			if cause := recover(); cause != nil {
				slog.Error("request handler failed", "request_id", w.Header().Get(XRequestId), "cause", cause)
				WriteHTTPError(w, InternalFailure)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type providerError struct {
	status int
	reason string
}

// Error includes the provider's status and reason for server logs.
func (e *providerError) Error() string {
	if e.reason != "" {
		return fmt.Sprintf("forecast provider HTTP %d: %s", e.status, e.reason)
	}
	return fmt.Sprintf("forecast provider HTTP %d", e.status)
}

// Identifies HTTP 408/429/5xx and temporary network failures worth retrying.
func IsRetryableWeatherError(err error) bool {
	var provider *providerError
	if errors.As(err, &provider) {
		return provider.status == 408 || provider.status == 429 || provider.status >= 500 && provider.status <= 599
	}
	var network net.Error
	return errors.Is(err, context.DeadlineExceeded) || errors.Is(err, io.ErrUnexpectedEOF) || errors.As(err, &network) && (network.Timeout() || network.Temporary())
}

// Reads a bounded error reason from an unsuccessful API response.
func ReadProviderError(response *http.Response) error {
	var failure struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(io.LimitReader(response.Body, 2048)).Decode(&failure)
	if len(failure.Reason) > 512 {
		failure.Reason = failure.Reason[:512]
	}
	return &providerError{status: response.StatusCode, reason: failure.Reason}
}

// Cleans up an unfinished transaction and keeps any earlier error.
// A committed transaction needs no cleanup.
func Rollback(tx pgx.Tx, operation string, result *error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		*result = errors.Join(*result, fmt.Errorf("rollback %s: %w", operation, err))
	}
}

// Marks an uncertain save so the caller reloads instead of writing twice.
func CommitForecast(ctx context.Context, tx pgx.Tx) error {
	if err := tx.Commit(ctx); err != nil {
		wrapped := fmt.Errorf("commit forecast snapshot: %w", err)
		if errors.Is(err, pgx.ErrTxCommitRollback) {
			return wrapped
		}
		return errors.Join(ErrCommitAmbiguous, wrapped)
	}
	return nil
}

// Asks the caller to reload a saved version after an uncertain commit.
// If reloading fails, discard the cached result. Never repeat the database write.
func ReloadAfterUncertainForecastCommit(ctx context.Context, timeout time.Duration, version string, original error, reload func(context.Context) (string, error), invalidate func()) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	defer cancel()
	savedVersion, err := reload(ctx)
	if err != nil {
		invalidate()
		return errors.Join(original, fmt.Errorf("reconcile forecast commit: %w", err))
	}
	if savedVersion == version {
		return nil
	}
	return original
}
