package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	profileauth "snooker/profile/internal/auth"
	"snooker/profile/internal/config"
	"snooker/profile/internal/profile"
)

type Server struct {
	router *gin.Engine
	cfg    *config.Config
	db     *pgxpool.Pool
}

func NewServer(cfg *config.Config, db *pgxpool.Pool) (*Server, error) {
	repo := profile.NewRepository(db)
	storage := profile.NewS3Storage(profile.S3StorageConfig{
		Endpoint:      cfg.StorageEndpoint,
		PublicBaseURL: cfg.StoragePublicBaseURL,
		AccessKey:     cfg.StorageAccessKey,
		SecretKey:     cfg.StorageSecretKey,
		BucketName:    cfg.StorageBucketName,
		Region:        cfg.StorageRegion,
		UseSSL:        cfg.StorageUseSSL,
	})
	authClient := profile.NewAuthClient(cfg.AuthInternalURL, cfg.InternalAPIKey)
	service := profile.NewService(repo, storage, authClient, cfg.MaxPhotoBytes)
	handler := profile.NewHandler(service)

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

	tokenValidator := profileauth.NewTokenValidator(cfg.JWTSecret)
	v1 := router.Group("/api/v1")
	{
		internal := v1.Group("/internal", internalAPIKeyMiddleware(cfg.InternalAPIKey))
		{
			internal.POST("/profiles/xp/match", handler.AwardMatchXP)
		}

		profiles := v1.Group("/profiles", profileauth.Middleware(tokenValidator))
		{
			profiles.GET("/me", handler.GetMe)
			profiles.POST("/me/complete", handler.Complete)
			profiles.PATCH("/me", handler.Update)
			profiles.POST("/me/photo-upload-url", handler.CreatePhotoUploadURL)
			profiles.GET("/:user_id", handler.GetPublic)
		}
	}

	return s, nil
}

func (s *Server) Start() error {
	addr := fmt.Sprintf(":%s", s.cfg.Port)
	fmt.Printf("profile service rodando em http://localhost%s\n", addr)
	return s.router.Run(addr)
}

func (s *Server) Live(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"service":   "profile",
		"status":    "UP",
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

func (s *Server) Ready(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	if err := s.db.Ping(ctx); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"service": "profile",
			"status":  "DOWN",
			"error":   "database unavailable",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"service": "profile",
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
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PATCH")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

func internalAPIKeyMiddleware(expectedKey string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if expectedKey == "" || c.GetHeader("X-Internal-API-Key") != expectedKey {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{
					"code":    "UNAUTHORIZED",
					"message": "Invalid internal API key",
				},
			})
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
