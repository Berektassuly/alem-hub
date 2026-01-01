// Package alem implements Alem Platform API client.
// This package handles all communication with the Alem School platform,
// including fetching student data, XP, tasks, and online status.
package alem

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

// ══════════════════════════════════════════════════════════════════════════════
// CONFIGURATION
// ══════════════════════════════════════════════════════════════════════════════

// ClientConfig contains configuration for the Alem API client.
type ClientConfig struct {
	// BaseURL is the Alem API base URL
	BaseURL string

	// APIKey is the API key for authentication (if applicable)
	APIKey string

	// Timeout is the HTTP request timeout
	Timeout time.Duration

	// RateLimiterConfig for API rate limiting
	RateLimiterConfig RateLimiterConfig

	// CircuitBreakerConfig for fault tolerance
	CircuitBreakerConfig CircuitBreakerConfig

	// RetryConfig for retry behavior
	RetryConfig RetryConfig

	// Logger for structured logging
	Logger *slog.Logger

	// Debug enables debug logging
	Debug bool
}

// DefaultClientConfig returns sensible defaults.
func DefaultClientConfig(baseURL string) ClientConfig {
	return ClientConfig{
		BaseURL:              baseURL,
		Timeout:              30 * time.Second,
		RateLimiterConfig:    DefaultRateLimiterConfig(),
		CircuitBreakerConfig: DefaultCircuitBreakerConfig(),
		RetryConfig:          DefaultRetryConfig(),
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// CLIENT
// ══════════════════════════════════════════════════════════════════════════════

// Client is the Alem Platform API client.
type Client struct {
	config         ClientConfig
	httpClient     *http.Client
	logger         *slog.Logger
	rateLimiter    *RateLimiter
	circuitBreaker *CircuitBreaker
	mapper         *Mapper

	// Token management
	token   *TokenDTO
	tokenMu sync.RWMutex
}

// NewClient creates a new Alem API client.
func NewClient(config ClientConfig) *Client {
	if config.Logger == nil {
		config.Logger = slog.Default()
	}

	return &Client{
		config: config,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		logger:         config.Logger,
		rateLimiter:    NewRateLimiter(config.RateLimiterConfig),
		circuitBreaker: NewCircuitBreaker(config.CircuitBreakerConfig),
		mapper:         NewMapper(),
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// AUTHENTICATION OPERATIONS
// ══════════════════════════════════════════════════════════════════════════════

// AuthRequest contains credentials for authentication.
type AuthRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// AuthResult contains the result of authentication.
type AuthResult struct {
	Token   *TokenDTO
	Student *StudentDTO
}

// Authenticate authenticates a user with email and password using Basic Authentication.
// The Alem platform uses Basic Auth header with base64(email:password) and returns an access_token.
func (c *Client) Authenticate(ctx context.Context, email, password string) (*AuthResult, error) {
	// Build the request manually since we need Basic Auth header
	fullURL := c.config.BaseURL + "/api/v1/auth/signin"
	
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	
	// Set Basic Authentication header
	credentials := email + ":" + password
	encoded := base64.StdEncoding.EncodeToString([]byte(credentials))
	req.Header.Set("Authorization", "Basic "+encoded)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	
	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()
	
	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	
	// Check status code
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("auth failed with status %d: %s", resp.StatusCode, string(respBody))
	}
	
	// Parse response - Alem returns {"access_token": "...", "verified": true}
	var authResponse struct {
		AccessToken string `json:"access_token"`
		Verified    bool   `json:"verified"`
	}
	if err := json.Unmarshal(respBody, &authResponse); err != nil {
		return nil, fmt.Errorf("parse auth response: %w", err)
	}
	
	// Create token from access_token
	token := TokenDTO{
		AccessToken: authResponse.AccessToken,
		TokenType:   "Bearer",
	}
	
	// Store token for subsequent requests
	c.tokenMu.Lock()
	c.token = &token
	c.tokenMu.Unlock()
	
	return &AuthResult{
		Token:   &token,
		Student: nil, // Alem signin doesn't return student data directly
	}, nil
}

// ══════════════════════════════════════════════════════════════════════════════
// STUDENT OPERATIONS
// ══════════════════════════════════════════════════════════════════════════════

// GetStudent fetches a single student by ID.
func (c *Client) GetStudent(ctx context.Context, studentID string) (*StudentDTO, error) {
	path := fmt.Sprintf("/students/%s", url.PathEscape(studentID))

	var response APIResponse[StudentDTO]
	if err := c.doRequest(ctx, http.MethodGet, path, nil, &response); err != nil {
		return nil, fmt.Errorf("get student %s: %w", studentID, err)
	}

	if !response.Success {
		return nil, fmt.Errorf("api error: %s", response.Error)
	}

	return &response.Data, nil
}

// GetStudentByLogin fetches a student by their login/username.
func (c *Client) GetStudentByLogin(ctx context.Context, login string) (*StudentDTO, error) {
	path := fmt.Sprintf("/students/by-login/%s", url.PathEscape(login))

	var response APIResponse[StudentDTO]
	if err := c.doRequest(ctx, http.MethodGet, path, nil, &response); err != nil {
		return nil, fmt.Errorf("get student by login %s: %w", login, err)
	}

	if !response.Success {
		return nil, fmt.Errorf("api error: %s", response.Error)
	}

	return &response.Data, nil
}

// ListStudents fetches a list of students with optional filters.
func (c *Client) ListStudents(ctx context.Context, req StudentsRequestDTO) ([]StudentDTO, *Meta, error) {
	params := url.Values{}
	if req.Cohort != "" {
		params.Set("cohort", req.Cohort)
	}
	if req.IsActive != nil {
		params.Set("is_active", strconv.FormatBool(*req.IsActive))
	}
	if req.IsOnline != nil {
		params.Set("is_online", strconv.FormatBool(*req.IsOnline))
	}
	if req.Search != "" {
		params.Set("search", req.Search)
	}
	if req.ModifiedSince != nil {
		params.Set("modified_since", req.ModifiedSince.Format(time.RFC3339))
	}
	if req.Page > 0 {
		params.Set("page", strconv.Itoa(req.Page))
	}
	if req.PerPage > 0 {
		params.Set("per_page", strconv.Itoa(req.PerPage))
	}

	path := "/students"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	var response APIResponse[[]StudentDTO]
	if err := c.doRequest(ctx, http.MethodGet, path, nil, &response); err != nil {
		return nil, nil, fmt.Errorf("list students: %w", err)
	}

	if !response.Success {
		return nil, nil, fmt.Errorf("api error: %s", response.Error)
	}

	return response.Data, response.Meta, nil
}

// GetOnlineStudents fetches currently online students.
func (c *Client) GetOnlineStudents(ctx context.Context) (*OnlineStudentsDTO, error) {
	var response APIResponse[OnlineStudentsDTO]
	if err := c.doRequest(ctx, http.MethodGet, "/students/online", nil, &response); err != nil {
		return nil, fmt.Errorf("get online students: %w", err)
	}

	if !response.Success {
		return nil, fmt.Errorf("api error: %s", response.Error)
	}

	return &response.Data, nil
}

// ══════════════════════════════════════════════════════════════════════════════
// LEADERBOARD OPERATIONS
// ══════════════════════════════════════════════════════════════════════════════

// GetLeaderboard fetches the leaderboard with optional filters.
func (c *Client) GetLeaderboard(ctx context.Context, req LeaderboardRequestDTO) (*LeaderboardDTO, error) {
	params := url.Values{}
	if req.Cohort != "" {
		params.Set("cohort", req.Cohort)
	}
	if req.Limit > 0 {
		params.Set("limit", strconv.Itoa(req.Limit))
	}
	if req.Offset > 0 {
		params.Set("offset", strconv.Itoa(req.Offset))
	}
	if req.SortBy != "" {
		params.Set("sort_by", req.SortBy)
	}
	if req.Order != "" {
		params.Set("order", req.Order)
	}

	path := "/leaderboard"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	var response APIResponse[LeaderboardDTO]
	if err := c.doRequest(ctx, http.MethodGet, path, nil, &response); err != nil {
		return nil, fmt.Errorf("get leaderboard: %w", err)
	}

	if !response.Success {
		return nil, fmt.Errorf("api error: %s", response.Error)
	}

	return &response.Data, nil
}

// GetStudentRank fetches a specific student's rank in the leaderboard.
func (c *Client) GetStudentRank(ctx context.Context, studentID string, cohort string) (*LeaderboardEntryDTO, error) {
	params := url.Values{}
	if cohort != "" {
		params.Set("cohort", cohort)
	}

	path := fmt.Sprintf("/leaderboard/rank/%s", url.PathEscape(studentID))
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	var response APIResponse[LeaderboardEntryDTO]
	if err := c.doRequest(ctx, http.MethodGet, path, nil, &response); err != nil {
		return nil, fmt.Errorf("get student rank: %w", err)
	}

	if !response.Success {
		return nil, fmt.Errorf("api error: %s", response.Error)
	}

	return &response.Data, nil
}

// ══════════════════════════════════════════════════════════════════════════════
// TASK COMPLETION OPERATIONS
// ══════════════════════════════════════════════════════════════════════════════

// GetTaskCompletions fetches task completions with optional filters.
func (c *Client) GetTaskCompletions(ctx context.Context, req TaskCompletionsRequestDTO) ([]TaskCompletionDTO, *Meta, error) {
	params := url.Values{}
	if req.StudentID != "" {
		params.Set("student_id", req.StudentID)
	}
	if req.TaskID != "" {
		params.Set("task_id", req.TaskID)
	}
	if req.Status != "" {
		params.Set("status", req.Status)
	}
	if req.Since != nil {
		params.Set("since", req.Since.Format(time.RFC3339))
	}
	if req.Page > 0 {
		params.Set("page", strconv.Itoa(req.Page))
	}
	if req.PerPage > 0 {
		params.Set("per_page", strconv.Itoa(req.PerPage))
	}

	path := "/task-completions"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	var response APIResponse[[]TaskCompletionDTO]
	if err := c.doRequest(ctx, http.MethodGet, path, nil, &response); err != nil {
		return nil, nil, fmt.Errorf("get task completions: %w", err)
	}

	if !response.Success {
		return nil, nil, fmt.Errorf("api error: %s", response.Error)
	}

	return response.Data, response.Meta, nil
}

// GetStudentTaskCompletions fetches all task completions for a specific student.
func (c *Client) GetStudentTaskCompletions(ctx context.Context, studentID string) ([]TaskCompletionDTO, error) {
	completions, _, err := c.GetTaskCompletions(ctx, TaskCompletionsRequestDTO{
		StudentID: studentID,
		PerPage:   1000, // Get all completions
	})
	return completions, err
}

// ══════════════════════════════════════════════════════════════════════════════
// SYNC OPERATIONS
// ══════════════════════════════════════════════════════════════════════════════

// GetSyncDelta fetches changes since the last sync.
func (c *Client) GetSyncDelta(ctx context.Context, syncToken string) (*SyncDeltaDTO, error) {
	params := url.Values{}
	if syncToken != "" {
		params.Set("sync_token", syncToken)
	}

	path := "/sync/delta"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	var response APIResponse[SyncDeltaDTO]
	if err := c.doRequest(ctx, http.MethodGet, path, nil, &response); err != nil {
		return nil, fmt.Errorf("get sync delta: %w", err)
	}

	if !response.Success {
		return nil, fmt.Errorf("api error: %s", response.Error)
	}

	return &response.Data, nil
}

// FullSync performs a full synchronization of all data.
func (c *Client) FullSync(ctx context.Context) (*SyncResult, error) {
	delta, err := c.GetSyncDelta(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("full sync: %w", err)
	}

	return c.mapper.SyncDeltaFromDTO(delta), nil
}

// ══════════════════════════════════════════════════════════════════════════════
// ACTIVITY OPERATIONS
// ══════════════════════════════════════════════════════════════════════════════

// GetStudentActivities fetches recent activities for a student.
func (c *Client) GetStudentActivities(ctx context.Context, studentID string, since time.Time) ([]ActivityDTO, error) {
	params := url.Values{}
	if !since.IsZero() {
		params.Set("since", since.Format(time.RFC3339))
	}

	path := fmt.Sprintf("/students/%s/activities", url.PathEscape(studentID))
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	var response APIResponse[[]ActivityDTO]
	if err := c.doRequest(ctx, http.MethodGet, path, nil, &response); err != nil {
		return nil, fmt.Errorf("get student activities: %w", err)
	}

	if !response.Success {
		return nil, fmt.Errorf("api error: %s", response.Error)
	}

	return response.Data, nil
}

// ══════════════════════════════════════════════════════════════════════════════
// HTTP REQUEST HELPERS
// ══════════════════════════════════════════════════════════════════════════════

// doRequest performs an HTTP request with rate limiting, circuit breaking, and retries.
func (c *Client) doRequest(ctx context.Context, method, path string, body interface{}, result interface{}) error {
	// Check circuit breaker
	if err := c.circuitBreaker.Allow(); err != nil {
		return fmt.Errorf("circuit breaker: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt <= c.config.RetryConfig.MaxRetries; attempt++ {
		if attempt > 0 {
			backoff := c.config.RetryConfig.CalculateBackoff(attempt)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}

		// Wait for rate limiter
		if err := c.rateLimiter.Allow(ctx); err != nil {
			return fmt.Errorf("rate limiter: %w", err)
		}

		err := c.doSingleRequest(ctx, method, path, body, result)
		if err == nil {
			c.circuitBreaker.RecordSuccess()
			return nil
		}

		lastErr = err

		// Check if error is retryable
		if !c.isRetryable(err) {
			c.circuitBreaker.RecordFailure()
			return err
		}

		// Handle rate limit response
		var rateLimitErr *RateLimitError
		if errors.As(err, &rateLimitErr) {
			c.rateLimiter.RecordRateLimitHit(rateLimitErr.RetryAfter)
		}
	}

	c.circuitBreaker.RecordFailure()
	return fmt.Errorf("request failed after %d retries: %w", c.config.RetryConfig.MaxRetries, lastErr)
}

// doSingleRequest performs a single HTTP request.
func (c *Client) doSingleRequest(ctx context.Context, method, path string, body interface{}, result interface{}) error {
	fullURL := c.config.BaseURL + path

	var bodyReader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal body: %w", err)
		}
		bodyReader = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, bodyReader)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	if c.config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	}

	// Add token if available
	c.tokenMu.RLock()
	if c.token != nil && !c.token.IsExpired() {
		req.Header.Set("Authorization", c.token.TokenType+" "+c.token.AccessToken)
	}
	c.tokenMu.RUnlock()

	if c.config.Debug {
		c.logger.Debug("alem api request", "method", method, "path", path)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	// Handle rate limiting
	if resp.StatusCode == http.StatusTooManyRequests {
		retryAfter := 60 * time.Second
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if seconds, err := strconv.Atoi(ra); err == nil {
				retryAfter = time.Duration(seconds) * time.Second
			}
		}
		return &RateLimitError{
			RetryAfter: retryAfter,
			Message:    "rate limit exceeded",
		}
	}

	// Handle error responses
	if resp.StatusCode >= 400 {
		var apiErr APIErrorDTO
		if err := json.Unmarshal(respBody, &apiErr); err == nil && apiErr.Message != "" {
			return &apiErr
		}
		return fmt.Errorf("api error: status %d", resp.StatusCode)
	}

	// Unmarshal response
	if result != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("unmarshal response: %w", err)
		}
	}

	return nil
}

// isRetryable checks if an error is retryable.
func (c *Client) isRetryable(err error) bool {
	if err == nil {
		return false
	}

	// Rate limit errors are retryable
	var rateLimitErr *RateLimitError
	if errors.As(err, &rateLimitErr) {
		return true
	}

	// API errors - check status code
	var apiErr *APIErrorDTO
	if errors.As(err, &apiErr) {
		// Server errors are retryable
		return apiErr.Code == "SERVER_ERROR" || apiErr.Code == "TEMPORARILY_UNAVAILABLE"
	}

	// Network errors are generally retryable
	errStr := err.Error()
	return containsAny(errStr, []string{"timeout", "connection refused", "temporary", "reset", "EOF"})
}

// containsAny checks if s contains any of the substrings.
func containsAny(s string, substrings []string) bool {
	for _, sub := range substrings {
		if len(s) >= len(sub) && findStr(s, sub) >= 0 {
			return true
		}
	}
	return false
}

// findStr finds substr in s.
func findStr(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// ══════════════════════════════════════════════════════════════════════════════
// HEALTH AND STATUS
// ══════════════════════════════════════════════════════════════════════════════

// IsHealthy checks if the Alem API is reachable.
func (c *Client) IsHealthy(ctx context.Context) bool {
	var response APIResponse[map[string]interface{}]
	err := c.doSingleRequest(ctx, http.MethodGet, "/health", nil, &response)
	return err == nil && response.Success
}

// Status returns the current status of the client.
type ClientStatus struct {
	RateLimiter    RateLimiterStatus
	CircuitBreaker CircuitBreakerStatus
	IsHealthy      bool
}

// Status returns the current status of the client.
func (c *Client) Status(ctx context.Context) ClientStatus {
	return ClientStatus{
		RateLimiter:    c.rateLimiter.Status(),
		CircuitBreaker: c.circuitBreaker.Status(),
		IsHealthy:      c.IsHealthy(ctx),
	}
}

// Reset resets the rate limiter and circuit breaker.
func (c *Client) Reset() {
	c.rateLimiter.Reset()
	c.circuitBreaker.Reset()
}

// GetAllStudents fetches all students from the Alem Platform, handling pagination.
func (c *Client) GetAllStudents(ctx context.Context) ([]StudentDTO, error) {
	var allStudents []StudentDTO
	page := 1
	perPage := 100

	for {
		students, meta, err := c.ListStudents(ctx, StudentsRequestDTO{
			Page:    page,
			PerPage: perPage,
		})
		if err != nil {
			return nil, fmt.Errorf("get all students page %d: %w", page, err)
		}

		allStudents = append(allStudents, students...)

		if len(students) < perPage || (meta != nil && page >= meta.TotalPages) {
			break
		}
		page++
	}

	return allStudents, nil
}

// GetBootcamp fetches the bootcamp data.
func (c *Client) GetBootcamp(ctx context.Context, bootcampID, cohortID string) (*BootcampDTO, error) {
	path := fmt.Sprintf("/api/v1/bootcamp/%s?cohort_id=%s", bootcampID, cohortID)
    
	// NOTE: The previous context indicated this endpoint might not use the standard APIResponse wrapper.
	// We will try without wrapper first based on the user's description, 
	// or if it fails we can adjust. The user said: "The bootcamp endpoint returns a direct BootcampDTO object".
	
	// Create a new request directly since doRequest expects APIResponse wrapper or handles errors differently.
	// But doRequest is robust. Let's try to reuse doSingleRequest with a custom struct if needed?
	// If the API returns raw JSON of BootcampDTO, we can just pass &BootcampDTO to doRequest/doSingleRequest?
	// doSingleRequest unmarshals into the result.
	// But doSingleRequest CHECKS for error fields in the JSON.
	
	// Let's assume for now it behaves like a normal endpoint but we map directly to BootcampDTO.
	var response BootcampDTO
	err := c.doRequest(ctx, http.MethodGet, path, nil, &response)
	if err != nil {
		return nil, fmt.Errorf("get bootcamp: %w", err)
	}
	return &response, nil
}

