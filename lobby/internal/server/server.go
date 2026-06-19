package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	lobbyauth "snooker/lobby/internal/auth"
	"snooker/lobby/internal/config"
	"snooker/lobby/internal/lobby"
)

type Server struct {
	router *gin.Engine
	cfg    *config.Config
	db     *pgxpool.Pool
}

func NewServer(cfg *config.Config, db *pgxpool.Pool) (*Server, error) {
	repo := lobby.NewRepository(db)
	handler := lobby.NewHandler(repo)

	router := gin.Default()
	router.Use(CORSMiddleware(cfg.AllowedOrigins))

	s := &Server{
		router: router,
		cfg:    cfg,
		db:     db,
	}

	router.GET("/health", s.Live)
	router.GET("/health/live", s.Live)
	router.GET("/health/ready", s.Ready)

	tokenValidator := lobbyauth.NewTokenValidator(cfg.JWTSecret)
	
	v1 := router.Group("/api/v1")
	{
		// Endpoints protegidos por JWT
		rooms := v1.Group("/rooms", lobbyauth.Middleware(tokenValidator))
		{
			rooms.POST("", handler.CreateRoom)
			rooms.GET("/public", handler.ListPublicRooms)
			rooms.GET("/:code_or_id", handler.GetRoom)
			rooms.POST("/:code_or_id/join", handler.JoinRoom)
			rooms.GET("/:code_or_id/ws", handler.HandleWS)
		}
	}

	return s, nil
}

func (s *Server) Start() error {
	addr := fmt.Sprintf(":%s", s.cfg.Port)
	fmt.Printf("lobby service rodando em http://localhost%s\n", addr)
	return s.router.Run(addr)
}

func (s *Server) Live(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"service":   "lobby",
		"status":    "UP",
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

func (s *Server) Ready(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	if err := s.db.Ping(ctx); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"service": "lobby",
			"status":  "DOWN",
			"error":   "database unavailable",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"service": "lobby",
		"status":  "READY",
	})
}

func CORSMiddleware(allowedOrigins string) gin.HandlerFunc {
	allowed := parseAllowedOrigins(allowedOrigins)

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" && isAllowedOrigin(origin, allowed) {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			c.Writer.Header().Set("Vary", "Origin")
		}

		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PATCH, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

func parseAllowedOrigins(allowedOrigins string) map[string]struct{} {
	allowed := make(map[string]struct{})
	for _, origin := range strings.Split(allowedOrigins, ",") {
		origin = strings.TrimSpace(origin)
		if origin != "" {
			allowed[origin] = struct{}{}
		}
	}
	return allowed
}

func isAllowedOrigin(origin string, allowed map[string]struct{}) bool {
	_, ok := allowed[origin]
	return ok
}

func (s *Server) GetRouter() *gin.Engine {
	return s.router
}
