package server

import (
	"context"
	"net/http"
	"strings"
)

type publicPathContextKey struct{}

// Prefixes are accepted only from a trusted proxy, for an explicitly
// configured host. Headers supplied through the old host cannot change it.
func (a *App) withPublicPath(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		prefix := a.config.PublicPathPrefixes[strings.ToLower(r.Host)]
		if prefix == "/zanelin" && r.Header.Get("X-Forwarded-Prefix") == prefix && a.isTrustedProxy(directRequestIP(r.RemoteAddr)) {
			r = r.WithContext(context.WithValue(r.Context(), publicPathContextKey{}, prefix))
		}
		next.ServeHTTP(w, r)
	})
}

func requestPublicPrefix(r *http.Request) string {
	prefix, _ := r.Context().Value(publicPathContextKey{}).(string)
	return prefix
}

func requestSessionCookiePath(r *http.Request) string {
	return requestPublicPrefix(r) + "/"
}
