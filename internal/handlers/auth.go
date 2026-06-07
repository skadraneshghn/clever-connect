package handlers

import (
	"net/http"
	"strings"
	"time"

	"clever-connect/internal/config"
	"clever-connect/internal/db"
	"clever-connect/internal/logger"
	"clever-connect/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type AuthHandler struct {
	cfg *config.Config
}

func NewAuthHandler(cfg *config.Config) *AuthHandler {
	return &AuthHandler{cfg: cfg}
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Warn("Auth", "Login request with invalid payload",
			"ip", c.ClientIP(),
			"error", err.Error(),
		)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
		return
	}

	logger.Info("Auth", "Login attempt",
		"username", req.Username,
		"ip", c.ClientIP(),
	)

	var user models.User
	if err := db.DB.Where("username = ?", req.Username).First(&user).Error; err != nil {
		logger.Warn("Auth", "Login failed — user not found",
			"username", req.Username,
			"ip", c.ClientIP(),
		)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid username or password"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		logger.Warn("Auth", "Login failed — incorrect password",
			"username", req.Username,
			"ip", c.ClientIP(),
		)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid username or password"})
		return
	}

	// Generate JWT Token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"username": user.Username,
		"role":     user.Role,
		"exp":      time.Now().Add(24 * time.Hour).Unix(),
	})

	tokenString, err := token.SignedString(h.cfg.JWTSecret)
	if err != nil {
		logger.Error("Auth", "Failed to sign JWT token",
			"username", req.Username,
			"error", err.Error(),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to sign authentication token"})
		return
	}

	logger.Info("Auth", "Login successful",
		"username", user.Username,
		"role", user.Role,
		"ip", c.ClientIP(),
	)

	c.JSON(http.StatusOK, gin.H{
		"token":    tokenString,
		"username": user.Username,
	})
}

// AuthMiddleware protects routing groups
func AuthMiddleware(jwtSecret []byte) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		var tokenString string

		if authHeader != "" {
			parts := strings.Split(authHeader, " ")
			if len(parts) == 2 && parts[0] == "Bearer" {
				tokenString = parts[1]
			}
		}

		// Check query param for websockets
		if tokenString == "" {
			tokenString = c.Query("token")
		}

		if tokenString == "" {
			logger.Warn("Auth", "Request blocked — missing authorization token",
				"path", c.Request.URL.Path,
				"ip", c.ClientIP(),
			)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Missing authorization token"})
			c.Abort()
			return
		}

		// Check if the token matches Ehco server or client auth token from DB
		if db.DB != nil {
			var serverCfg models.EhcoServerConfig
			if db.DB.First(&serverCfg).Error == nil && serverCfg.AuthToken != "" && tokenString == serverCfg.AuthToken {
				c.Next()
				return
			}
			var clientCfg models.EhcoClientConfig
			if db.DB.First(&clientCfg).Error == nil && clientCfg.AuthToken != "" && tokenString == clientCfg.AuthToken {
				c.Next()
				return
			}
		}

		token, err := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
			return jwtSecret, nil
		})

		if err != nil || !token.Valid {
			logger.Warn("Auth", "Request blocked — invalid or expired token",
				"path", c.Request.URL.Path,
				"ip", c.ClientIP(),
				"error", err,
			)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
			c.Abort()
			return
		}

		c.Next()
	}
}
