package profile

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"snooker/profile/internal/httpx"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) GetMe(c *gin.Context) {
	userID, ok := userIDFromContext(c)
	if !ok {
		unauthorized(c)
		return
	}

	p, err := h.service.GetProfile(c.Request.Context(), userID)
	if err != nil {
		handleProfileError(c, err)
		return
	}
	c.JSON(http.StatusOK, p)
}

func (h *Handler) GetPublic(c *gin.Context) {
	userID, err := uuid.Parse(c.Param("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, httpx.ErrorResponse{
			Error: httpx.ErrorDetail{Code: httpx.ErrCodeValidationFailed, Message: "Invalid user_id"},
		})
		return
	}

	p, err := h.service.GetProfile(c.Request.Context(), userID)
	if err != nil {
		handleProfileError(c, err)
		return
	}
	c.JSON(http.StatusOK, p)
}

func (h *Handler) CreatePhotoUploadURL(c *gin.Context) {
	userID, ok := userIDFromContext(c)
	if !ok {
		unauthorized(c)
		return
	}

	var req PhotoUploadURLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, httpx.ErrorResponse{
			Error: httpx.ErrorDetail{Code: httpx.ErrCodeValidationFailed, Message: "Invalid request body"},
		})
		return
	}

	resp, err := h.service.CreatePhotoUploadURL(c.Request.Context(), userID, &req)
	if err != nil {
		handleProfileError(c, err)
		return
	}
	c.JSON(http.StatusCreated, resp)
}

func (h *Handler) Complete(c *gin.Context) {
	userID, ok := userIDFromContext(c)
	if !ok {
		unauthorized(c)
		return
	}

	status, _ := c.Get(httpx.ContextKeyStatus)
	if status == StatusBlocked {
		c.JSON(http.StatusForbidden, httpx.ErrorResponse{
			Error: httpx.ErrorDetail{Code: httpx.ErrCodeForbidden, Message: "Conta bloqueada"},
		})
		return
	}

	var req CompleteProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, httpx.ErrorResponse{
			Error: httpx.ErrorDetail{Code: httpx.ErrCodeValidationFailed, Message: "Invalid request body"},
		})
		return
	}

	resp, err := h.service.CompleteProfile(c.Request.Context(), userID, &req)
	if err != nil {
		handleProfileError(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handler) Update(c *gin.Context) {
	userID, ok := userIDFromContext(c)
	if !ok {
		unauthorized(c)
		return
	}

	status, _ := c.Get(httpx.ContextKeyStatus)
	if status != StatusActive {
		c.JSON(http.StatusForbidden, httpx.ErrorResponse{
			Error: httpx.ErrorDetail{Code: httpx.ErrCodeOnboardingNeeded, Message: "Complete o perfil antes de editar"},
		})
		return
	}

	var req UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, httpx.ErrorResponse{
			Error: httpx.ErrorDetail{Code: httpx.ErrCodeValidationFailed, Message: "Invalid request body"},
		})
		return
	}

	p, err := h.service.UpdateProfile(c.Request.Context(), userID, &req)
	if err != nil {
		handleProfileError(c, err)
		return
	}
	c.JSON(http.StatusOK, p)
}

func (h *Handler) AwardMatchXP(c *gin.Context) {
	var req MatchXPRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.ParticipantUserIDs) == 0 {
		c.JSON(http.StatusBadRequest, httpx.ErrorResponse{
			Error: httpx.ErrorDetail{Code: httpx.ErrCodeValidationFailed, Message: "Invalid request body"},
		})
		return
	}

	resp, err := h.service.AwardMatchXP(c.Request.Context(), &req)
	if err != nil {
		handleProfileError(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func handleProfileError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		c.JSON(http.StatusNotFound, httpx.ErrorResponse{
			Error: httpx.ErrorDetail{Code: httpx.ErrCodeNotFound, Message: "Profile not found"},
		})
	case errors.Is(err, ErrDuplicateNickname):
		c.JSON(http.StatusConflict, httpx.ErrorResponse{
			Error: httpx.ErrorDetail{Code: httpx.ErrCodeConflict, Message: "Nickname ja esta em uso"},
		})
	case errors.Is(err, ErrInvalidUpload):
		c.JSON(http.StatusBadRequest, httpx.ErrorResponse{
			Error: httpx.ErrorDetail{Code: httpx.ErrCodeInvalidUpload, Message: err.Error()},
		})
	default:
		c.JSON(http.StatusInternalServerError, httpx.ErrorResponse{
			Error: httpx.ErrorDetail{Code: httpx.ErrCodeInternal, Message: "An unexpected error occurred"},
		})
	}
}

func userIDFromContext(c *gin.Context) (uuid.UUID, bool) {
	value, exists := c.Get(httpx.ContextKeyUserID)
	if !exists {
		return uuid.Nil, false
	}
	userIDStr, ok := value.(string)
	if !ok {
		return uuid.Nil, false
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return uuid.Nil, false
	}
	return userID, true
}

func unauthorized(c *gin.Context) {
	c.JSON(http.StatusUnauthorized, httpx.ErrorResponse{
		Error: httpx.ErrorDetail{Code: httpx.ErrCodeUnauthorized, Message: "User not authenticated"},
	})
}
