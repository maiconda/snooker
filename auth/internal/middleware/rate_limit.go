package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"snooker/auth/internal/httpx"
)

type clientLimit struct {
	lastSeen time.Time
	count    int
}

func IPBasedRateLimiter(limit int, window time.Duration) gin.HandlerFunc {
	var mu sync.Mutex
	clients := make(map[string]*clientLimit)

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
			c.AbortWithStatusJSON(http.StatusTooManyRequests, httpx.ErrorResponse{
				Error: httpx.ErrorDetail{
					Code:    httpx.ErrCodeTooManyRequests,
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
		userID, exists := c.Get(httpx.ContextKeyUserID)
		if !exists {
			c.Next()
			return
		}

		userIDStr, ok := userID.(string)
		if !ok {
			c.Next()
			return
		}

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
			c.AbortWithStatusJSON(http.StatusTooManyRequests, httpx.ErrorResponse{
				Error: httpx.ErrorDetail{
					Code:    httpx.ErrCodeTooManyRequests,
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
