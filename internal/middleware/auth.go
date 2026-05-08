package middleware

import (
	"net/http"
	"strings"

	"repository-contorller-service-go/internal/service"

	"github.com/gin-gonic/gin"
)

const UserIDContextKey = "userID"

func AuthMiddleware(authService *service.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString := extractToken(c)
		if tokenString == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing authorization token"})
			return
		}

		userID, err := authService.ParseToken(tokenString)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}

		c.Set(UserIDContextKey, userID)
		c.Next()
	}
}

func extractToken(c *gin.Context) string {
	candidates := []string{
		c.GetHeader("Authorization"),
		c.GetHeader("X-Access-Token"),
		c.GetHeader("Access-Token"),
		c.GetHeader("access_token"),
		c.Query("access_token"),
	}

	for _, candidate := range candidates {
		token := normalizeToken(candidate)
		if token != "" {
			return token
		}
	}

	return ""
}

func normalizeToken(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	for _, prefix := range []string{"Bearer ", "bearer "} {
		if token, ok := strings.CutPrefix(value, prefix); ok {
			return strings.TrimSpace(token)
		}
	}

	return value
}
