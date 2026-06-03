package telegram

import (
	"fmt"
	"strings"
	"time"

	"clever-connect/internal/logger"

	tele "gopkg.in/telebot.v4"
)

// registerMiddleware attaches global middleware to the bot.
func (e *Engine) registerMiddleware() {
	// Logging middleware — every incoming update is logged
	e.Bot.Use(func(next tele.HandlerFunc) tele.HandlerFunc {
		return func(c tele.Context) error {
			start := time.Now()
			e.messagesProcessed.Add(1)

			sender := c.Sender()
			logger.Info("Telegram", "Incoming update",
				"user_id", sender.ID,
				"username", sender.Username,
				"text", truncate(c.Text(), 80),
				"chat_id", c.Chat().ID,
			)

			err := next(c)

			logger.Info("Telegram", "Update processed",
				"duration_ms", time.Since(start).Milliseconds(),
				"user_id", sender.ID,
			)

			return err
		}
	})

	// Guard middleware — check if user is admin.
	// Allow only /myid command so users can find their ID.
	e.Bot.Use(func(next tele.HandlerFunc) tele.HandlerFunc {
		return func(c tele.Context) error {
			// Always allow /myid
			if strings.HasPrefix(c.Text(), "/myid") {
				return next(c)
			}

			if !e.IsAdmin(c.Sender().ID) {
				logger.Warn("Telegram", "Unauthorized access blocked",
					"user_id", c.Sender().ID,
					"username", c.Sender().Username,
					"text", c.Text(),
				)
				return c.Send("⛔ Access denied. You are not an authorized administrator.")
			}
			return next(c)
		}
	})

	// Admin-only guard middleware for sensitive commands
	// This is applied per-handler, not globally
}

// AdminOnly returns a middleware that restricts a handler to admin users only.
func (e *Engine) AdminOnly(next tele.HandlerFunc) tele.HandlerFunc {
	return func(c tele.Context) error {
		if !e.IsAdmin(c.Sender().ID) {
			logger.Warn("Telegram", "Unauthorized access attempt",
				"user_id", c.Sender().ID,
				"username", c.Sender().Username,
				"command", c.Text(),
			)
			return c.Send("⛔ Access denied. You are not an authorized administrator.")
		}
		return next(c)
	}
}

// RateLimiter provides per-user rate limiting middleware.
type RateLimiter struct {
	users    map[int64]time.Time
	interval time.Duration
}

// NewRateLimiter creates a rate limiter with the specified minimum interval between messages.
func NewRateLimiter(interval time.Duration) *RateLimiter {
	return &RateLimiter{
		users:    make(map[int64]time.Time),
		interval: interval,
	}
}

// Middleware returns a telebot middleware function for rate limiting.
func (rl *RateLimiter) Middleware(next tele.HandlerFunc) tele.HandlerFunc {
	return func(c tele.Context) error {
		uid := c.Sender().ID
		if lastSeen, ok := rl.users[uid]; ok {
			if time.Since(lastSeen) < rl.interval {
				return c.Send("⏳ Please wait before sending another command.")
			}
		}
		rl.users[uid] = time.Now()
		return next(c)
	}
}

// ──────────────────────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────────────────────

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func formatUptime(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60

	parts := []string{}
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", days))
	}
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	parts = append(parts, fmt.Sprintf("%dm", mins))
	return strings.Join(parts, " ")
}
