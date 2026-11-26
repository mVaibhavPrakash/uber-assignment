package api

import (
	"encoding/json"
	"mobilitycontrolroom/internal/appErrors"

	"mobilitycontrolroom/internal/recommendation"
	"mobilitycontrolroom/internal/repository"
	"net/http"
	"slices"
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

	return appErrors.WithRequestRecovery(mux)
}

type handler struct {
	service Service
	clock   func() time.Time
}

func (h *handler) handleAreas(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawQuery != "" {
		appErrors.WriteHTTPError(w, appErrors.QueryNotAllowed)
		return
	}
	writeJSON(w, 200, h.service.Areas())
}

func (h *handler) handleRecommendations(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawQuery != "" {
		appErrors.WriteHTTPError(w, appErrors.QueryNotAllowed)
		return
	}
	result := omitHourlyEvidence(h.service.CurrentRecommendation(h.clock()))
	if result.Status == "unavailable" {
		result.ErrorCode = appErrors.RecommendationUnavailable.Code
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
		appErrors.WriteHTTPError(w, appErrors.InvalidAreaID)
		return
	}
	if !slices.ContainsFunc(h.service.Areas(), func(area repository.Area) bool { return area.ID == id }) {
		appErrors.WriteHTTPError(w, appErrors.AreaNotFound)
		return
	}
	result := h.service.CurrentRecommendation(h.clock())
	if result.Status == "unavailable" {
		appErrors.WriteHTTPError(w, appErrors.CurrentInsightsUnavailable)
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
	appErrors.WriteHTTPError(w, appErrors.InsightsUnavailable)
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
			appErrors.WriteHTTPError(w, appErrors.UnknownQuery)
			return false
		}
	}
	if version := query.Get("forecast_version"); version != "" && version != result.ForecastVersion {
		appErrors.WriteHTTPError(w, appErrors.SnapshotChanged)
		return false
	}
	if start := query.Get("window_start"); start != "" {
		at, e := time.Parse(time.RFC3339, start)
		if e != nil {
			appErrors.WriteHTTPError(w, appErrors.InvalidWindowStart)
			return false
		}
		if !at.Equal(result.Window.Start) {
			appErrors.WriteHTTPError(w, appErrors.WindowChanged)
			return false
		}
	}
	return true
}
