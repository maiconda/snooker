package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"snooker/internal/models"
)

type clientLimit struct {
	lastSeen time.Time
	count    int
}

// IPBasedRateLimiter limita requisições com base no endereço IP.
// Spec: 06-auth-architecture.md - /signup e /login limitados a 5 requisições por minuto por IP.
func IPBasedRateLimiter(limit int, window time.Duration) gin.HandlerFunc {
	var mu sync.Mutex
	clients := make(map[string]*clientLimit)

	// Goroutine de limpeza periódica para liberar memória
	go func() {
		for {
			time.Sleep(window)
			mu.Lock()
			for ip, cl := range clients {
				if time.Since(cl.lastSeen) > window {
					delete(clients, ip)
				}
			}
			mu.Unlock()
		}
	}()

	return func(c *gin.Context) {
		ip := c.ClientIP()

		mu.Lock()
		cl, exists := clients[ip]
		if !exists {
			clients[ip] = &clientLimit{
				lastSeen: time.Now(),
				count:    1,
			}
			mu.Unlock()
			c.Next()
			return
		}

		if time.Since(cl.lastSeen) > window {
			cl.lastSeen = time.Now()
			cl.count = 1
			mu.Unlock()
			c.Next()
			return
		}

		if cl.count >= limit {
			mu.Unlock()
			c.AbortWithStatusJSON(http.StatusTooManyRequests, models.ErrorResponse{
				Error: models.ErrorDetail{
					Code:    "TOO_MANY_REQUESTS",
					Message: "Too many requests. Please try again later.",
				},
			})
			return
		}

		cl.count++
		mu.Unlock()
		c.Next()
	}
}

// UserBasedRateLimiter limita requisições com base no ID do usuário autenticado no contexto.
// Spec: 06-auth-architecture.md - /upload-url limitado a 2 requisições por minuto por usuário.
func UserBasedRateLimiter(limit int, window time.Duration) gin.HandlerFunc {
	var mu sync.Mutex
	users := make(map[string]*clientLimit)

	go func() {
		for {
			time.Sleep(window)
			mu.Lock()
			for userID, cl := range users {
				if time.Since(cl.lastSeen) > window {
					delete(users, userID)
				}
			}
			mu.Unlock()
		}
	}()

	return func(c *gin.Context) {
		userID, exists := c.Get(ContextKeyUserID)
		if !exists {
			// Se o usuário não estiver autenticado, permite passar (o middleware de autenticação cuidará disso)
			c.Next()
			return
		}

		userIDStr := userID.(string)

		mu.Lock()
		cl, exists := users[userIDStr]
		if !exists {
			users[userIDStr] = &clientLimit{
				lastSeen: time.Now(),
				count:    1,
			}
			mu.Unlock()
			c.Next()
			return
		}

		if time.Since(cl.lastSeen) > window {
			cl.lastSeen = time.Now()
			cl.count = 1
			mu.Unlock()
			c.Next()
			return
		}

		if cl.count >= limit {
			mu.Unlock()
			c.AbortWithStatusJSON(http.StatusTooManyRequests, models.ErrorResponse{
				Error: models.ErrorDetail{
					Code:    "TOO_MANY_REQUESTS",
					Message: "Rate limit exceeded. Please try again later.",
				},
			})
			return
		}

		cl.count++
		mu.Unlock()
		c.Next()
	}
}
