// Package logger — Gin middleware for structured HTTP request/response logging.
//
// Captures: method, path, status code, latency, client IP, user-agent, request
// body size, response body size, and any error messages — all dispatched
// asynchronously through the core logger channel.
package logger

import (
	"time"

	"github.com/gin-gonic/gin"
)

// GinMiddleware returns a Gin middleware handler that logs every HTTP request
// with rich context through the async structured logging pipeline.
//
// It is designed to replace gin.Logger() and the inline logging middleware
// that was previously defined in main.go.
func GinMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		// Process request
		c.Next()

		latency := time.Since(start)
		statusCode := c.Writer.Status()
		clientIP := c.ClientIP()
		method := c.Request.Method
		userAgent := c.Request.UserAgent()
		bodySize := c.Writer.Size()
		errorMsg := c.Errors.ByType(gin.ErrorTypePrivate).String()

		if raw != "" {
			path = path + "?" + raw
		}

		// Determine log level by status code
		level := INFO
		component := "HTTP"

		switch {
		case statusCode >= 500:
			level = ERROR
		case statusCode >= 400:
			level = WARN
		case statusCode >= 300:
			level = INFO
		}

		fields := []interface{}{
			"status", statusCode,
			"latency", latency.String(),
			"ip", clientIP,
			"method", method,
			"bytes", bodySize,
		}

		// Only include user-agent for API requests (not static assets)
		if len(userAgent) > 0 && len(userAgent) < 200 {
			fields = append(fields, "ua", userAgent)
		}

		if errorMsg != "" {
			fields = append(fields, "error", errorMsg)
		}

		msg := method + " " + path
		emitAt(level, component, msg, 4, fields...)
	}
}

// GinRecoveryMiddleware returns a Gin recovery middleware that logs panics
// through our structured logger instead of writing to stdout.
func GinRecoveryMiddleware() gin.HandlerFunc {
	return gin.CustomRecoveryWithWriter(GinWriter(), func(c *gin.Context, err interface{}) {
		Error("Panic", "Recovered from panic in HTTP handler",
			"error", err,
			"path", c.Request.URL.Path,
			"method", c.Request.Method,
			"ip", c.ClientIP(),
		)
		c.AbortWithStatus(500)
	})
}
