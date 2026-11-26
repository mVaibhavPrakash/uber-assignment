package api

import (
	"encoding/json"
	"mobilitycontrolroom/internal/apperrors"

	"mobilitycontrolroom/internal/recommendation"
	"mobilitycontrolroom/internal/repository"
	"net/http"
	"strconv"
	"time"
)

type Service interface {
	Areas() []repository.Area
	// It shares read-only nested data; response preparation must not mutate it.
	CurrentRecommendation(time.Time) recommendation.Result
}

func Handler(service Service, clock func() time.Time) http.Handler {
	if clock == nil {
		clock = time.Now
	}
	h := &handler{service: service, clock: clock}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/areas", h.handleAreas)
	mux.HandleFunc("GET /api/recommendations", h.handleRecommendations)
	mux.HandleFunc("GET /api/areas/{id}/insights", h.handleInsights)
	mux.HandleFunc("GET /api/health", h.handleHealth)

	return apperrors.WithRequestRecovery(mux)
}

type handler struct {
	service Service
	clock   func() time.Time
}

func (h *handler) handleAreas(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawQuery != "" {
		apperrors.WriteHTTPError(w, apperrors.QueryNotAllowed)
		return
	}
	writeJSON(w, 200, h.service.Areas())
}

func (h *handler) handleRecommendations(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawQuery != "" {
		apperrors.WriteHTTPError(w, apperrors.QueryNotAllowed)
		return
	}
	result := omitHourlyEvidence(h.service.CurrentRecommendation(h.clock()))
	if result.Status == "unavailable" {
		result.ErrorCode = apperrors.RecommendationUnavailable.Code
		writeJSON(w, 503, struct {
			recommendation.Result
			RequestID string `json:"request_id"`
		}{result, w.Header().Get("X-Request-ID")})
		return
	}
	writeJSON(w, 200, result)
}

func (h *handler) handleInsights(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		apperrors.WriteHTTPError(w, apperrors.InvalidAreaID)
		return
	}
	known := false
	for _, area := range h.service.Areas() {
		if area.ID == id {
			known = true
			break
		}
	}
	if !known {
		apperrors.WriteHTTPError(w, apperrors.AreaNotFound)
		return
	}
	result := h.service.CurrentRecommendation(h.clock())
	if result.Status == "unavailable" {
		apperrors.WriteHTTPError(w, apperrors.CurrentInsightsUnavailable)
		return
	}
	if !validateInsightVersionAndWindow(w, r, result) {
		return
	}
	for _, area := range result.Areas {
		if area.Area.ID == id {
			writeJSON(w, 200, map[string]any{"forecast_version": result.ForecastVersion, "forecast_fetched_at": result.ForecastFetchedAt, "generated_at": result.GeneratedAt, "valid_until": result.ValidUntil, "window": result.Window, "freshness": result.Freshness, "evidence": area, "warnings": warningsWithForecastFreshness(result)})
			return
		}
	}
	apperrors.WriteHTTPError(w, apperrors.InsightsUnavailable)
}

func (h *handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	result := h.service.CurrentRecommendation(h.clock())
	status, state := 200, "ready"
	if result.Status == "unavailable" {
		status = 503
		state = "unavailable"
	} else if result.Freshness == "stale" || result.Status == "insufficient_evidence" || result.Coverage.Assessed < result.Coverage.Total {
		state = "degraded"
	}
	writeJSON(w, status, map[string]any{"status": state, "recommendation_status": result.Status, "forecast_freshness": result.Freshness, "forecast_version": result.ForecastVersion})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

// Keeps details tied to the forecast and window the user selected.
func validateInsightVersionAndWindow(w http.ResponseWriter, r *http.Request, result recommendation.Result) bool {
	query := r.URL.Query()
	for key := range query {
		if key != "forecast_version" && key != "window_start" {
			apperrors.WriteHTTPError(w, apperrors.UnknownQuery)
			return false
		}
	}
	if version := query.Get("forecast_version"); version != "" && version != result.ForecastVersion {
		apperrors.WriteHTTPError(w, apperrors.SnapshotChanged)
		return false
	}
	if start := query.Get("window_start"); start != "" {
		at, e := time.Parse(time.RFC3339, start)
		if e != nil {
			apperrors.WriteHTTPError(w, apperrors.InvalidWindowStart)
			return false
		}
		if !at.Equal(result.Window.Start) {
			apperrors.WriteHTTPError(w, apperrors.WindowChanged)
			return false
		}
	}
	return true
}
