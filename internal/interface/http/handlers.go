// Package http implements REST API and Webhook endpoints for Alem Community Hub.
package http

import (
	"github.com/alem-hub/alem-community-hub/internal/application/query"
	"github.com/alem-hub/alem-community-hub/pkg/logger"
	"encoding/json"
	"io"
	"net/http"
)

// ══════════════════════════════════════════════════════════════════════════════
// HEALTH & STATUS HANDLERS
// ══════════════════════════════════════════════════════════════════════════════

// handleRoot serves the root endpoint with basic API information.
func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	info := map[string]interface{}{
		"name":        "Alem Community Hub API",
		"version":     "v1",
		"description": "REST API for Alem Community Hub - From Competition to Collaboration",
		"endpoints": map[string]string{
			"health":      "/health",
			"leaderboard": "/api/v1/leaderboard",
			"online":      "/api/v1/students/online",
			"helpers":     "/api/v1/helpers",
			"stats":       "/api/v1/stats",
		},
		"documentation": "https://github.com/alem-hub/alem-community-hub",
	}

	writeJSON(w, http.StatusOK, info)
}

// handleHealth handles the health check endpoint.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if s.deps.HealthChecker != nil {
		status := s.deps.HealthChecker.Check(r.Context())
		if !status.Healthy {
			writeJSON(w, http.StatusServiceUnavailable, status)
			return
		}
		writeJSON(w, http.StatusOK, status)
		return
	}

	// Default health response
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "healthy",
		"uptime":  s.Uptime().String(),
		"version": "v1",
	})
}

// handleReady handles the readiness probe endpoint (for Kubernetes).
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if s.deps.HealthChecker != nil {
		status := s.deps.HealthChecker.Check(r.Context())
		if !status.Ready {
			w.WriteHeader(http.StatusServiceUnavailable)
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"status": "not_ready",
				"reason": status.Message,
			})
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

// handleLive handles the liveness probe endpoint (for Kubernetes).
func (s *Server) handleLive(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "alive"})
}

// handleMetrics handles the Prometheus metrics endpoint.
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement Prometheus metrics exposition
	// For now, return basic server metrics as JSON
	metrics := map[string]interface{}{
		"uptime_seconds": s.Uptime().Seconds(),
		"running":        s.IsRunning(),
	}

	writeJSON(w, http.StatusOK, metrics)
}

// ══════════════════════════════════════════════════════════════════════════════
// LEADERBOARD HANDLERS
// ══════════════════════════════════════════════════════════════════════════════

// handleGetLeaderboard handles GET /api/v1/leaderboard
func (s *Server) handleGetLeaderboard(w http.ResponseWriter, r *http.Request) {
	s.handleLeaderboardInternal(w, r, "")
}

// handleGetLeaderboardByCohort handles GET /api/v1/leaderboard/{cohort}
func (s *Server) handleGetLeaderboardByCohort(w http.ResponseWriter, r *http.Request) {
	cohort := r.PathValue("cohort")
	s.handleLeaderboardInternal(w, r, cohort)
}

// handleLeaderboardInternal is the internal implementation for leaderboard handlers.
func (s *Server) handleLeaderboardInternal(w http.ResponseWriter, r *http.Request, cohort string) {
	if s.deps.GetLeaderboardHandler == nil {
		writeJSONError(w, http.StatusNotImplemented, "not_implemented", "Leaderboard handler not configured")
		return
	}

	// Parse query parameters
	q := query.GetLeaderboardQuery{
		Cohort:               cohort,
		Limit:                getQueryParamInt(r, "limit", 20),
		Offset:               getQueryParamInt(r, "offset", 0),
		OnlyOnline:           getQueryParamBool(r, "online"),
		OnlyAvailableForHelp: getQueryParamBool(r, "available_for_help"),
		IncludeRankChange:    getQueryParamBool(r, "include_rank_change"),
	}

	// Execute query
	result, err := s.deps.GetLeaderboardHandler.Handle(r.Context(), q)
	if err != nil {
		s.logger.Error("failed to get leaderboard", logger.Err(err))
		writeJSONError(w, http.StatusInternalServerError, "internal_error", "Failed to get leaderboard")
		return
	}

	// Build response with pagination metadata
	meta := &ResponseMeta{
		TotalCount: result.TotalCount,
		Page:       result.Page,
		PageSize:   result.PageSize,
		HasMore:    result.HasMore,
	}

	writeJSONWithMeta(w, r, http.StatusOK, result, meta)
}

// ══════════════════════════════════════════════════════════════════════════════
// STUDENT HANDLERS
// ══════════════════════════════════════════════════════════════════════════════

// handleGetStudent handles GET /api/v1/students/{id}
func (s *Server) handleGetStudent(w http.ResponseWriter, r *http.Request) {
	studentID := r.PathValue("id")
	if studentID == "" {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", "Student ID is required")
		return
	}

	// Get student rank (which includes student info)
	if s.deps.GetStudentRankHandler == nil {
		writeJSONError(w, http.StatusNotImplemented, "not_implemented", "Student handler not configured")
		return
	}

	q := query.GetStudentRankQuery{
		StudentID:      studentID,
		IncludeHistory: getQueryParamBool(r, "include_history"),
		HistoryDays:    getQueryParamInt(r, "history_days", 7),
	}

	result, err := s.deps.GetStudentRankHandler.Handle(r.Context(), q)
	if err != nil {
		s.logger.Error("failed to get student", logger.Err(err), logger.String("student_id", studentID))
		writeJSONError(w, http.StatusNotFound, "not_found", "Student not found")
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// handleGetStudentRank handles GET /api/v1/students/{id}/rank
func (s *Server) handleGetStudentRank(w http.ResponseWriter, r *http.Request) {
	studentID := r.PathValue("id")
	if studentID == "" {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", "Student ID is required")
		return
	}

	if s.deps.GetStudentRankHandler == nil {
		writeJSONError(w, http.StatusNotImplemented, "not_implemented", "Rank handler not configured")
		return
	}

	q := query.GetStudentRankQuery{
		StudentID:      studentID,
		Cohort:         getQueryParam(r, "cohort", ""),
		IncludeHistory: getQueryParamBool(r, "include_history"),
		HistoryDays:    getQueryParamInt(r, "history_days", 7),
	}

	result, err := s.deps.GetStudentRankHandler.Handle(r.Context(), q)
	if err != nil {
		s.logger.Error("failed to get student rank", logger.Err(err), logger.String("student_id", studentID))
		writeJSONError(w, http.StatusNotFound, "not_found", "Student rank not found")
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// handleGetStudentNeighbors handles GET /api/v1/students/{id}/neighbors
func (s *Server) handleGetStudentNeighbors(w http.ResponseWriter, r *http.Request) {
	studentID := r.PathValue("id")
	if studentID == "" {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", "Student ID is required")
		return
	}

	if s.deps.GetNeighborsHandler == nil {
		writeJSONError(w, http.StatusNotImplemented, "not_implemented", "Neighbors handler not configured")
		return
	}

	q := query.GetNeighborsQuery{
		StudentID:           studentID,
		Cohort:              getQueryParam(r, "cohort", ""),
		RangeSize:           getQueryParamInt(r, "radius", 5),
		IncludeOnlineStatus: getQueryParamBool(r, "include_online"),
	}

	result, err := s.deps.GetNeighborsHandler.Handle(r.Context(), q)
	if err != nil {
		s.logger.Error("failed to get neighbors", logger.Err(err), logger.String("student_id", studentID))
		writeJSONError(w, http.StatusNotFound, "not_found", "Neighbors not found")
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// handleGetStudentProgress handles GET /api/v1/students/{id}/progress
func (s *Server) handleGetStudentProgress(w http.ResponseWriter, r *http.Request) {
	studentID := r.PathValue("id")
	if studentID == "" {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", "Student ID is required")
		return
	}

	if s.deps.GetDailyProgressHandler == nil {
		writeJSONError(w, http.StatusNotImplemented, "not_implemented", "Progress handler not configured")
		return
	}

	q := query.GetDailyProgressQuery{
		StudentID:         studentID,
		HistoryDays:       getQueryParamInt(r, "days", 7),
		IncludeHistory:    true,
		IncludeComparison: getQueryParamBool(r, "include_comparison"),
	}

	result, err := s.deps.GetDailyProgressHandler.Handle(r.Context(), q)
	if err != nil {
		s.logger.Error("failed to get progress", logger.Err(err), logger.String("student_id", studentID))
		writeJSONError(w, http.StatusNotFound, "not_found", "Progress not found")
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// ══════════════════════════════════════════════════════════════════════════════
// ONLINE STUDENTS HANDLER
// ══════════════════════════════════════════════════════════════════════════════

// handleGetOnline handles GET /api/v1/students/online
func (s *Server) handleGetOnline(w http.ResponseWriter, r *http.Request) {
	if s.deps.GetOnlineNowHandler == nil {
		writeJSONError(w, http.StatusNotImplemented, "not_implemented", "Online handler not configured")
		return
	}

	q := query.GetOnlineNowQuery{
		Cohort:               getQueryParam(r, "cohort", ""),
		IncludeAway:          getQueryParamBool(r, "include_away"),
		IncludeRecent:        getQueryParamBool(r, "include_recent"),
		OnlyAvailableForHelp: getQueryParamBool(r, "available_for_help"),
		Limit:                getQueryParamInt(r, "limit", 50),
		Offset:               getQueryParamInt(r, "offset", 0),
		SortBy:               getQueryParam(r, "sort_by", "last_seen"),
		SortDesc:             getQueryParamBool(r, "sort_desc"),
		IncludeActivity:      getQueryParamBool(r, "include_activity"),
		IncludeRank:          getQueryParamBool(r, "include_rank"),
	}

	result, err := s.deps.GetOnlineNowHandler.Handle(r.Context(), q)
	if err != nil {
		s.logger.Error("failed to get online students", logger.Err(err))
		writeJSONError(w, http.StatusInternalServerError, "internal_error", "Failed to get online students")
		return
	}

	meta := &ResponseMeta{
		TotalCount: result.TotalCount,
		HasMore:    result.HasMore,
	}

	writeJSONWithMeta(w, r, http.StatusOK, result, meta)
}

// ══════════════════════════════════════════════════════════════════════════════
// HELPERS HANDLER
// ══════════════════════════════════════════════════════════════════════════════

// handleFindHelpers handles GET /api/v1/helpers
func (s *Server) handleFindHelpers(w http.ResponseWriter, r *http.Request) {
	if s.deps.FindHelpersHandler == nil {
		writeJSONError(w, http.StatusNotImplemented, "not_implemented", "Helpers handler not configured")
		return
	}

	taskID := getQueryParam(r, "task_id", "")
	if taskID == "" {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", "task_id query parameter is required")
		return
	}

	q := query.FindHelpersQuery{
		TaskID:        taskID,
		Cohort:        getQueryParam(r, "cohort", ""),
		PreferOnline:  getQueryParamBool(r, "only_online"),
		MinHelpRating: 0, // Could parse from query param if needed
		Limit:         getQueryParamInt(r, "limit", 10),
		RequesterID:   getQueryParam(r, "exclude", ""),
	}

	result, err := s.deps.FindHelpersHandler.Handle(r.Context(), q)
	if err != nil {
		s.logger.Error("failed to find helpers", logger.Err(err), logger.String("task_id", taskID))
		writeJSONError(w, http.StatusInternalServerError, "internal_error", "Failed to find helpers")
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// ══════════════════════════════════════════════════════════════════════════════
// STATS HANDLER
// ══════════════════════════════════════════════════════════════════════════════

// handleGetStats handles GET /api/v1/stats
func (s *Server) handleGetStats(w http.ResponseWriter, r *http.Request) {
	// Aggregate stats from various sources
	stats := map[string]interface{}{
		"server": map[string]interface{}{
			"uptime":  s.Uptime().String(),
			"running": s.IsRunning(),
		},
	}

	// Add online stats if handler is available
	if s.deps.GetOnlineNowHandler != nil {
		q := query.GetOnlineNowQuery{
			Limit:         1,
			IncludeAway:   true,
			IncludeRecent: true,
		}
		result, err := s.deps.GetOnlineNowHandler.Handle(r.Context(), q)
		if err == nil {
			stats["community"] = map[string]interface{}{
				"online_now":   result.TotalOnline,
				"away":         result.TotalAway,
				"recent":       result.TotalRecent,
				"total_active": result.TotalCount,
				"activity":     result.CommunityActivity,
			}
		}
	}

	// Add leaderboard stats if handler is available
	if s.deps.GetLeaderboardHandler != nil {
		q := query.GetLeaderboardQuery{
			Limit: 1,
		}
		result, err := s.deps.GetLeaderboardHandler.Handle(r.Context(), q)
		if err == nil {
			stats["leaderboard"] = map[string]interface{}{
				"total_students": result.TotalCount,
				"average_xp":     result.AverageXP,
				"median_xp":      result.MedianXP,
			}
		}
	}

	writeJSON(w, http.StatusOK, stats)
}

// ══════════════════════════════════════════════════════════════════════════════
// WEBHOOK HANDLERS
// ══════════════════════════════════════════════════════════════════════════════

// TelegramWebhookPayload represents a Telegram webhook update.
type TelegramWebhookPayload struct {
	UpdateID int64 `json:"update_id"`
	Message  *struct {
		MessageID int64 `json:"message_id"`
		From      *struct {
			ID        int64  `json:"id"`
			IsBot     bool   `json:"is_bot"`
			FirstName string `json:"first_name"`
			LastName  string `json:"last_name,omitempty"`
			Username  string `json:"username,omitempty"`
		} `json:"from,omitempty"`
		Chat *struct {
			ID   int64  `json:"id"`
			Type string `json:"type"`
		} `json:"chat,omitempty"`
		Date int64  `json:"date"`
		Text string `json:"text,omitempty"`
	} `json:"message,omitempty"`
	CallbackQuery *struct {
		ID   string `json:"id"`
		From *struct {
			ID       int64  `json:"id"`
			Username string `json:"username,omitempty"`
		} `json:"from,omitempty"`
		Data string `json:"data,omitempty"`
	} `json:"callback_query,omitempty"`
}

// handleTelegramWebhook handles POST /webhook/telegram
func (s *Server) handleTelegramWebhook(w http.ResponseWriter, r *http.Request) {
	s.processTelegramWebhook(w, r, "")
}

// handleTelegramWebhookWithToken handles POST /webhook/telegram/{token}
func (s *Server) handleTelegramWebhookWithToken(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	s.processTelegramWebhook(w, r, token)
}

// processTelegramWebhook is the internal implementation for webhook processing.
func (s *Server) processTelegramWebhook(w http.ResponseWriter, r *http.Request, token string) {
	// Validate token if configured
	if s.config.WebhookSecret != "" && token != s.config.WebhookSecret {
		s.logger.Warn("invalid webhook token", logger.String("ip", getClientIP(r)))
		writeJSONError(w, http.StatusUnauthorized, "unauthorized", "Invalid webhook token")
		return
	}

	// Read body
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1MB limit
	if err != nil {
		s.logger.Error("failed to read webhook body", logger.Err(err))
		writeJSONError(w, http.StatusBadRequest, "invalid_request", "Failed to read request body")
		return
	}
	defer r.Body.Close()

	// Parse payload
	var payload TelegramWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		s.logger.Error("failed to parse webhook payload", logger.Err(err))
		writeJSONError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON payload")
		return
	}

	// Log webhook receipt
	s.logger.Info("received telegram webhook",
		logger.Int64("update_id", payload.UpdateID),
		logger.Bool("has_message", payload.Message != nil),
		logger.Bool("has_callback", payload.CallbackQuery != nil),
	)

	// Delegate to webhook handler if configured
	if s.deps.WebhookHandler != nil {
		if err := s.deps.WebhookHandler.HandleTelegramUpdate(r.Context(), body); err != nil {
			s.logger.Error("failed to handle telegram update", logger.Err(err))
			// Still return 200 to Telegram to avoid retries
		}
	}

	// Always return 200 to acknowledge receipt
	writeJSON(w, http.StatusOK, map[string]string{"status": "received"})
}
