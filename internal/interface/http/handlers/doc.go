// Package handlers contains HTTP handler interfaces, implementations, and middleware.
//
// This package provides:
//   - Health check interfaces and implementations
//   - Webhook handling for Telegram integration
//   - Reusable middleware components
//   - Authentication middleware
//
// # Health Checks
//
// The HealthChecker interface allows registering multiple named health checks
// that are executed in parallel:
//
//	checker := handlers.NewCompositeHealthChecker("v1.0.0")
//	checker.AddCheck("database", handlers.NewDatabaseCheck(db))
//	checker.AddCheck("cache", handlers.NewCacheCheck(cache))
//	checker.AddCheck("alem_api", handlers.NewExternalAPICheck(alemClient))
//
//	status := checker.Check(ctx)
//	if !status.Healthy {
//	    log.Printf("Health check failed: %s", status.Message)
//	}
//
// # Webhook Handling
//
// The WebhookHandler interface provides a way to handle Telegram webhooks:
//
//	handler := handlers.NewTelegramWebhookHandler()
//
//	// Register command handlers
//	handler.RegisterCommand("/start", startHandler)
//	handler.RegisterCommand("/help", helpHandler)
//	handler.RegisterCommand("/top", topHandler)
//
//	// Register callback handler for inline buttons
//	handler.RegisterCallback(callbackHandler)
//
//	// Handle webhook payload
//	err := handler.HandleTelegramUpdate(ctx, payload)
//
// # Middleware
//
// The package provides several reusable middleware components:
//
//	// API Key authentication
//	auth := handlers.NewAPIKeyAuth("X-API-Key", []string{"secret-key"})
//	protected := auth.Middleware(myHandler)
//
//	// Request timeout
//	withTimeout := handlers.TimeoutMiddleware(30 * time.Second)(myHandler)
//
//	// Security headers
//	secure := handlers.SecurityHeadersMiddleware(myHandler)
//
//	// Chain multiple middleware
//	handler := handlers.ChainHandler(
//	    myHandler,
//	    handlers.SecurityHeadersMiddleware,
//	    handlers.NoCacheMiddleware,
//	    auth.Middleware,
//	)
//
// # Best Practices
//
// When implementing health checks:
//   - Use timeouts to prevent slow checks from blocking the response
//   - Include critical dependencies like database and cache
//   - Keep checks fast (< 1 second ideally)
//   - Return detailed information for debugging
//
// When handling webhooks:
//   - Always return 200 to Telegram to prevent retries
//   - Process updates asynchronously if needed
//   - Validate webhook tokens for security
//   - Log all received updates for debugging
//
// When using middleware:
//   - Apply security middleware early in the chain
//   - Apply authentication before authorization
//   - Use request size limits to prevent DoS attacks
//   - Add proper timeout handling for all endpoints
package handlers
