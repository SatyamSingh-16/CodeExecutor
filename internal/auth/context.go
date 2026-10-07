package auth

import "context"

type authContextKey struct{}

var userCtxKey = authContextKey{}

// WithUserID injects the authenticated user ID into the context.
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userCtxKey, userID)
}

// UserIDFromContext extracts the authenticated user ID from the context if present.
func UserIDFromContext(ctx context.Context) (string, bool) {
	val := ctx.Value(userCtxKey)
	if val == nil {
		return "", false
	}
	userID, ok := val.(string)
	return userID, ok && userID != ""
}
