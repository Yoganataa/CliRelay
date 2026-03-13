package util

// ContextKey is a strongly-typed context key to avoid collisions with other
// packages and satisfy static analysis checks (SA1029).
type ContextKey string

const (
	// ContextKeyAlt carries an executor "alt" mode hint (e.g. "responses/compact").
	ContextKeyAlt ContextKey = "alt"

	// ContextKeyGin carries a *gin.Context for request-scoped logging and helpers.
	// It is intentionally stored as an opaque value here to avoid import cycles.
	ContextKeyGin ContextKey = "gin"

	// ContextKeyRoundTripper carries an optional http.RoundTripper override used
	// by proxy-aware HTTP clients.
	ContextKeyRoundTripper ContextKey = "cliproxy.roundtripper"
)
