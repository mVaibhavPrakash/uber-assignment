// Package appErrors keeps application error definitions and handling in one place.
// It does not depend on business services or repositories.
package appErrors

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
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
