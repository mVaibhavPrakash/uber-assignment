// Package httpApi serves recommendation summaries and hourly evidence.
package httpApi

import (
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"time"

	"mobilitycontrolroom/internal/domain"
	"mobilitycontrolroom/internal/domain/recommendation"
)

// Service supplies the published recommendation and monitored zones.
type Service interface {
	Areas() []domain.Area
	// It shares read-only nested data; response preparation must not mutate it.
	CurrentRecommendation(time.Time) recommendation.Result
}

// Handler registers the HTTP API routes and request recovery.
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

	return withRequestRecovery(mux)
}

type handler struct {
	service Service
	clock   func() time.Time
}

func (h *handler) handleAreas(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawQuery != "" {
		writeHTTPError(w, queryNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, presentAreas(h.service.Areas()))
}

func (h *handler) handleRecommendations(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawQuery != "" {
		writeHTTPError(w, queryNotAllowed)
		return
	}
	response := summarize(h.service.CurrentRecommendation(h.clock()))
	if response.Status == "unavailable" {
		response.ErrorCode = recommendationUnavailableCode
		response.RequestID = w.Header().Get(requestIDHeader)
		writeJSON(w, http.StatusServiceUnavailable, response)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *handler) handleInsights(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		writeHTTPError(w, invalidAreaID)
		return
	}
	if !slices.ContainsFunc(h.service.Areas(), func(area domain.Area) bool { return area.ID == id }) {
		writeHTTPError(w, areaNotFound)
		return
	}
	result := h.service.CurrentRecommendation(h.clock())
	if result.Status == "unavailable" {
		writeHTTPError(w, currentInsightsUnavailable)
		return
	}
	if !validateInsightVersionAndWindow(w, r, result) {
		return
	}
	for _, area := range result.Areas {
		if area.Area.ID == id {
			writeJSON(w, http.StatusOK, map[string]any{
				"forecast_version":    result.ForecastVersion,
				"forecast_fetched_at": result.ForecastFetchedAt,
				"generated_at":        result.GeneratedAt,
				"valid_until":         result.ValidUntil,
				"window":              windowResponse(result.Window),
				"freshness":           result.Freshness,
				"evidence":            presentArea(area, true),
				"warnings":            warningsWithForecastFreshness(result),
			})
			return
		}
	}
	writeHTTPError(w, insightsUnavailable)
}

func (h *handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	result := h.service.CurrentRecommendation(h.clock())
	status, state := http.StatusOK, "ready"
	if result.Status == "unavailable" {
		status = http.StatusServiceUnavailable
		state = "unavailable"
	} else if result.Freshness == "stale" || result.Status == "insufficient_evidence" || result.Coverage.Assessed < result.Coverage.Total {
		state = "degraded"
	}
	writeJSON(w, status, map[string]any{"status": state, "recommendation_status": result.Status, "forecast_freshness": result.Freshness, "forecast_version": result.ForecastVersion})
}

// Keeps details tied to the forecast and window the user selected.
func validateInsightVersionAndWindow(w http.ResponseWriter, r *http.Request, result recommendation.Result) bool {
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		writeHTTPError(w, malformedQuery)
		return false
	}
	for key := range query {
		if key != "forecast_version" && key != "window_start" {
			writeHTTPError(w, unknownQuery)
			return false
		}
	}
	if version := query.Get("forecast_version"); version != "" && version != result.ForecastVersion {
		writeHTTPError(w, snapshotChanged)
		return false
	}
	if start := query.Get("window_start"); start != "" {
		at, e := time.Parse(time.RFC3339, start)
		if e != nil {
			writeHTTPError(w, invalidWindowStart)
			return false
		}
		if !at.Equal(result.Window.Start) {
			writeHTTPError(w, windowChanged)
			return false
		}
	}
	return true
}
