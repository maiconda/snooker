package lobby

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"snooker/lobby/internal/httpx"
)

type Handler struct {
	repo Repository
}

func NewHandler(repo Repository) *Handler {
	return &Handler{repo: repo}
}

func (h *Handler) CreateRoom(c *gin.Context) {
	userID, exists := c.Get(httpx.ContextKeyUserID)
	if !exists {
		c.JSON(http.StatusUnauthorized, httpx.ErrorResponse{
			Error: httpx.ErrorDetail{Code: httpx.ErrCodeUnauthorized, Message: "Usuario nao autenticado"},
		})
		return
	}

	var req CreateRoomRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, httpx.ErrorResponse{
			Error: httpx.ErrorDetail{Code: httpx.ErrCodeValidationFailed, Message: err.Error()},
		})
		return
	}

	room, err := h.repo.CreateRoom(c.Request.Context(), userID.(string), req.IsPrivate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, httpx.ErrorResponse{
			Error: httpx.ErrorDetail{Code: httpx.ErrCodeInternal, Message: err.Error()},
		})
		return
	}

	c.JSON(http.StatusCreated, toRoomResponse(room))
}

func (h *Handler) ListPublicRooms(c *gin.Context) {
	rooms, err := h.repo.ListPublicRooms(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, httpx.ErrorResponse{
			Error: httpx.ErrorDetail{Code: httpx.ErrCodeInternal, Message: err.Error()},
		})
		return
	}

	resp := make([]RoomResponse, len(rooms))
	for i, r := range rooms {
		resp[i] = toRoomResponse(r)
	}

	c.JSON(http.StatusOK, resp)
}

func (h *Handler) GetRoom(c *gin.Context) {
	codeOrID := c.Param("code_or_id")
	if codeOrID == "" {
		c.JSON(http.StatusBadRequest, httpx.ErrorResponse{
			Error: httpx.ErrorDetail{Code: httpx.ErrCodeValidationFailed, Message: "Parametro code_or_id obrigatorio"},
		})
		return
	}

	var room *Room
	var err error

	// Se tem 6 caracteres, assumimos que e um codigo de sala
	if len(codeOrID) == 6 {
		room, err = h.repo.GetRoomByCode(c.Request.Context(), codeOrID)
	} else {
		room, err = h.repo.GetRoomByID(c.Request.Context(), codeOrID)
	}

	if err != nil {
		if errors.Is(err, ErrRoomNotFound) {
			c.JSON(http.StatusNotFound, httpx.ErrorResponse{
				Error: httpx.ErrorDetail{Code: httpx.ErrCodeNotFound, Message: "Sala nao encontrada"},
			})
			return
		}
		c.JSON(http.StatusInternalServerError, httpx.ErrorResponse{
			Error: httpx.ErrorDetail{Code: httpx.ErrCodeInternal, Message: err.Error()},
		})
		return
	}

	c.JSON(http.StatusOK, toRoomResponse(room))
}

func (h *Handler) JoinRoom(c *gin.Context) {
	userID, exists := c.Get(httpx.ContextKeyUserID)
	if !exists {
		c.JSON(http.StatusUnauthorized, httpx.ErrorResponse{
			Error: httpx.ErrorDetail{Code: httpx.ErrCodeUnauthorized, Message: "Usuario nao autenticado"},
		})
		return
	}

	codeOrID := c.Param("code_or_id")
	if codeOrID == "" {
		c.JSON(http.StatusBadRequest, httpx.ErrorResponse{
			Error: httpx.ErrorDetail{Code: httpx.ErrCodeValidationFailed, Message: "Parametro code_or_id obrigatorio"},
		})
		return
	}

	// Buscar primeiro a sala para obter o ID correto
	var room *Room
	var err error
	if len(codeOrID) == 6 {
		room, err = h.repo.GetRoomByCode(c.Request.Context(), codeOrID)
	} else {
		room, err = h.repo.GetRoomByID(c.Request.Context(), codeOrID)
	}

	if err != nil {
		if errors.Is(err, ErrRoomNotFound) {
			c.JSON(http.StatusNotFound, httpx.ErrorResponse{
				Error: httpx.ErrorDetail{Code: httpx.ErrCodeNotFound, Message: "Sala nao encontrada"},
			})
			return
		}
		c.JSON(http.StatusInternalServerError, httpx.ErrorResponse{
			Error: httpx.ErrorDetail{Code: httpx.ErrCodeInternal, Message: err.Error()},
		})
		return
	}

	joinedRoom, err := h.repo.JoinRoom(c.Request.Context(), room.ID, userID.(string))
	if err != nil {
		if errors.Is(err, ErrRoomNotFound) {
			c.JSON(http.StatusNotFound, httpx.ErrorResponse{
				Error: httpx.ErrorDetail{Code: httpx.ErrCodeNotFound, Message: "Sala nao encontrada"},
			})
			return
		}
		if errors.Is(err, ErrRoomFull) {
			c.JSON(http.StatusConflict, httpx.ErrorResponse{
				Error: httpx.ErrorDetail{Code: httpx.ErrCodeConflict, Message: "A sala ja esta cheia"},
			})
			return
		}
		if errors.Is(err, ErrRoomExpired) {
			c.JSON(http.StatusGone, httpx.ErrorResponse{
				Error: httpx.ErrorDetail{Code: httpx.ErrCodeConflict, Message: "A sala expirou"},
			})
			return
		}
		if errors.Is(err, ErrRoomAlreadyJoined) {
			c.JSON(http.StatusBadRequest, httpx.ErrorResponse{
				Error: httpx.ErrorDetail{Code: httpx.ErrCodeValidationFailed, Message: "Voce e o criador desta sala e nao pode entrar como oponente"},
			})
			return
		}

		c.JSON(http.StatusInternalServerError, httpx.ErrorResponse{
			Error: httpx.ErrorDetail{Code: httpx.ErrCodeInternal, Message: err.Error()},
		})
		return
	}

	c.JSON(http.StatusOK, toRoomResponse(joinedRoom))
}

func toRoomResponse(r *Room) RoomResponse {
	return RoomResponse{
		ID:         r.ID,
		Code:       r.Code,
		CreatorID:  r.CreatorID,
		OpponentID: r.OpponentID,
		Status:     r.Status,
		IsPrivate:  r.IsPrivate,
		CreatedAt:  r.CreatedAt,
		ExpiresAt:  r.ExpiresAt,
	}
}
