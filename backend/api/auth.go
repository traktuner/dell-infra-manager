package api

import (
	"net/http"
	"strings"

	"github.com/dell-infra-manager/backend/config"
	"github.com/gin-gonic/gin"
)

// User represents the currently-authenticated identity, lifted from the
// reverse proxy's auth headers.
type User struct {
	Email  string   `json:"email"`
	Name   string   `json:"name,omitempty"`
	Groups []string `json:"groups,omitempty"`
}

// ctxUserKey is the gin context key under which the authenticated User is stored.
const ctxUserKey = "user"

// AuthMiddleware reads identity headers set by an upstream reverse proxy
// (Traefik forward-auth, oauth2-proxy, Cloudflare Access). We trust them
// blindly — the deployment must ensure these headers are scrubbed from
// untrusted sources at the proxy boundary.
//
// Behaviour:
//   - cfg.Enabled == false → no-op, allows all requests (dev mode).
//   - cfg.Enabled == true  → rejects requests without the user header (401).
//
// The audit log uses ctxUser(c) to record who performed each action.
func AuthMiddleware(cfg config.AuthConfig) gin.HandlerFunc {
	userHeader := cfg.UserHeader
	if userHeader == "" {
		userHeader = "X-Auth-Request-Email"
	}
	return func(c *gin.Context) {
		email := strings.TrimSpace(c.GetHeader(userHeader))
		if !cfg.Enabled {
			// Dev mode: accept anonymous, attach a placeholder so audit logs
			// still have something to write.
			c.Set(ctxUserKey, &User{Email: "anonymous"})
			c.Next()
			return
		}
		if email == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "missing identity header — request must come through the auth proxy",
			})
			return
		}
		user := &User{Email: email}
		if cfg.NameHeader != "" {
			user.Name = strings.TrimSpace(c.GetHeader(cfg.NameHeader))
		}
		if cfg.GroupHeader != "" {
			if g := strings.TrimSpace(c.GetHeader(cfg.GroupHeader)); g != "" {
				user.Groups = strings.Split(g, ",")
				for i := range user.Groups {
					user.Groups[i] = strings.TrimSpace(user.Groups[i])
				}
			}
		}
		c.Set(ctxUserKey, user)
		c.Next()
	}
}

// ctxUser returns the authenticated user from the gin context.
// Always returns non-nil (anonymous fallback in dev mode).
func ctxUser(c *gin.Context) *User {
	if v, ok := c.Get(ctxUserKey); ok {
		if u, ok := v.(*User); ok {
			return u
		}
	}
	return &User{Email: "anonymous"}
}

// MeHandler returns the currently-authenticated user. Frontend uses this to
// render the "logged in as …" indicator and to detect auth-disabled mode.
func MeHandler(cfg config.AuthConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"user":         ctxUser(c),
			"auth_enabled": cfg.Enabled,
		})
	}
}
