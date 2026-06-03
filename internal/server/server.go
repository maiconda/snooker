package server

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"snooker/internal/auth"
	"snooker/internal/config"
	"snooker/internal/handler"
	"snooker/internal/middleware"
	"snooker/internal/profile"
	"snooker/internal/repository"
	"snooker/internal/storage"
)

// Server envelopa a aplicação Gin HTTP.
type Server struct {
	router *gin.Engine
	cfg    *config.Config
	db     *pgxpool.Pool
}

// NewServer inicializa as dependências, repositórios, serviços e rotas do servidor.
func NewServer(cfg *config.Config, db *pgxpool.Pool, store *storage.StorageService) (*Server, error) {
	// Cria repositórios
	userRepo := repository.NewUsuarioRepository(db)
	profileRepo := repository.NewPerfilRepository(db)
	tokenRepo := repository.NewRefreshTokenRepository(db)

	// Cria serviços
	tokenService := auth.NewTokenService(cfg.JWTSecret, tokenRepo)
	authService := auth.NewAuthService(userRepo, tokenService, cfg.GoogleClientID)
	profileService := profile.NewProfileService(profileRepo, userRepo, store)

	// Cria handlers
	authHandler := handler.NewAuthHandler(authService, tokenService)
	profileHandler := handler.NewProfileHandler(profileService, tokenService, authService)

	// Setup do Router Gin
	router := gin.Default()

	// CORS Middleware
	router.Use(CORSMiddleware(cfg.AllowedOrigins))

	// Rota de Healthcheck
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "UP", "timestamp": time.Now().Format(time.RFC3339)})
	})

	// Grupo da API v1
	v1 := router.Group("/api/v1")
	{
		// 1. Endpoints públicos de autenticação com rate limiting (5 req/min por IP)
		authLimit := middleware.IPBasedRateLimiter(5, 1*time.Minute)
		authGroup := v1.Group("/auth")
		{
			authGroup.POST("/signup", authLimit, authHandler.Signup)
			authGroup.POST("/login", authLimit, authHandler.Login)
			authGroup.POST("/google", authLimit, authHandler.GoogleAuth)
			authGroup.POST("/refresh", authHandler.Refresh)
		}

		// 2. Endpoints autenticados
		authMiddleware := middleware.AuthMiddleware(tokenService)
		
		// Logout (autenticado)
		v1.POST("/auth/logout", authMiddleware, authHandler.Logout)

		// Perfil & Onboarding (autenticado com restrições de status)
		profileGroup := v1.Group("/profile")
		profileGroup.Use(authMiddleware)
		{
			// GET /profile/upload-url: apenas para onboarding_pending ou active
			// Rate limit: 2 req/min por usuário
			uploadLimit := middleware.UserBasedRateLimiter(2, 1*time.Minute)
			profileGroup.GET("/upload-url", middleware.RequireActiveOrOnboarding(), uploadLimit, profileHandler.GetUploadURL)

			// POST /profile/complete: estritamente restrito a onboarding_pending
			profileGroup.POST("/complete", middleware.RequireOnboarding(), profileHandler.CompleteProfile)
		}

		// Rotas de teste/game protegidas para validar o RequireActive
		v1.GET("/lobbies", authMiddleware, middleware.RequireActive(), func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"message": "Bem-vindo à sala de jogos!"})
		})
	}

	return &Server{
		router: router,
		cfg:    cfg,
		db:     db,
	}, nil
}

// Start inicia o servidor HTTP na porta configurada.
func (s *Server) Start() error {
	addr := fmt.Sprintf(":%s", s.cfg.Port)
	fmt.Printf("Servidor rodando em http://localhost%s\n", addr)
	return s.router.Run(addr)
}

// CORSMiddleware implementa a política de CORS exigida.
// Spec: OPTIONS requests must return Access-Control-Allow-Credentials: true.
func CORSMiddleware(allowedOrigins string) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin == "" {
			origin = allowedOrigins
		}

		c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// GetRouter retorna o router Gin interno (útil para testes de integração/E2E).
func (s *Server) GetRouter() *gin.Engine {
	return s.router
}
