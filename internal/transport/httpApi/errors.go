package httpApi

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
)

const requestIDHeader = "X-Request-ID"
const recommendationUnavailableCode = "RECOMMENDATION_UNAVAILABLE"

type httpError struct {
	Status  int
	Code    string
	Message string
}

var (
	queryNotAllowed            = httpError{400, "INVALID_QUERY", "This endpoint does not accept query parameters."}
	malformedQuery             = httpError{http.StatusBadRequest, "INVALID_QUERY", "Malformed query parameters."}
	unknownQuery               = httpError{400, "INVALID_QUERY", "Unknown query parameter."}
	invalidAreaID              = httpError{400, "INVALID_AREA_ID", "Area ID must be a positive integer."}
	areaNotFound               = httpError{404, "AREA_NOT_FOUND", "Area is not in the configured scope."}
	currentInsightsUnavailable = httpError{503, recommendationUnavailableCode, "Current insights are unavailable."}
	insightsUnavailable        = httpError{503, "INSIGHTS_UNAVAILABLE", "Area evidence is unavailable."}
	snapshotChanged            = httpError{409, "SNAPSHOT_CHANGED", "Recommendation evidence changed; refresh recommendations."}
	invalidWindowStart         = httpError{400, "INVALID_QUERY", "window_start must be an RFC3339 timestamp."}
	windowChanged              = httpError{409, "SNAPSHOT_CHANGED", "Planning window changed; refresh recommendations."}
	internalFailure            = httpError{500, "INTERNAL_ERROR", "Unable to complete this request."}
)

func writeHTTPError(w http.ResponseWriter, failure httpError) {
	writeJSON(w, failure.Status, map[string]any{
		"error":                  map[string]string{"code": failure.Code, "message": failure.Message, "request_id": w.Header().Get(requestIDHeader)},
		"primary_recommendation": nil,
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	body, err := json.Marshal(value)
	if err != nil {
		slog.Error("encode HTTP response", "request_id", w.Header().Get(requestIDHeader), "error", err)
		writeHTTPError(w, internalFailure)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if _, err := w.Write(append(body, '\n')); err != nil {
		slog.Debug("write HTTP response", "request_id", w.Header().Get(requestIDHeader), "error", err)
	}
}

func withRequestRecovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(requestIDHeader, uuid.NewString())
		defer func() {
			if cause := recover(); cause != nil {
				if cause == http.ErrAbortHandler {
					panic(cause)
				}
				slog.Error("request handler failed", "request_id", w.Header().Get(requestIDHeader), "cause", cause)
				writeHTTPError(w, internalFailure)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
