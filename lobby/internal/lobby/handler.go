package lobby

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"snooker/lobby/internal/httpx"
	natsclient "snooker/lobby/internal/nats"
)

type Handler struct {
	repo                   Repository
	ownerDisconnectTimeout time.Duration
	presence               *presenceTracker
	invitations            *invitationStore
	rematches              *rematchStore
	xpAwarder              MatchXPAwarder
	matchStateMu           sync.Mutex
	snapshotsMu            sync.RWMutex
	snapshots              map[string]json.RawMessage
	turnTimersMu           sync.Mutex
	turnTimers             map[string]*scheduledTurnTimer
}

type scheduledTurnTimer struct {
	turnSeq    int
	deadlineMS int64
	timer      *time.Timer
}

type HandlerOption func(*Handler)

func WithOwnerDisconnectTimeout(timeout time.Duration) HandlerOption {
	return func(h *Handler) {
		if timeout > 0 {
			h.ownerDisconnectTimeout = timeout
		}
	}
}

func WithMatchXPAwarder(awarder MatchXPAwarder) HandlerOption {
	return func(h *Handler) {
		if awarder != nil {
			h.xpAwarder = awarder
		}
	}
}

func NewHandler(repo Repository, opts ...HandlerOption) *Handler {
	h := &Handler{
		repo:                   repo,
		ownerDisconnectTimeout: 10 * time.Second,
		presence:               newPresenceTracker(),
		invitations:            newInvitationStore(),
		rematches:              newRematchStore(),
		xpAwarder:              noopMatchXPAwarder{},
		snapshots:              make(map[string]json.RawMessage),
		turnTimers:             make(map[string]*scheduledTurnTimer),
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
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

	h.expireRoomsAndCleanup(c.Request.Context())

	room, err := h.repo.CreateRoom(c.Request.Context(), userID.(string), req.IsPrivate)
	if err != nil {
		if errors.Is(err, ErrUserInActiveRoom) {
			c.JSON(http.StatusConflict, httpx.ErrorResponse{
				Error: httpx.ErrorDetail{Code: httpx.ErrCodeConflict, Message: "Voce ja esta em uma partida ativa"},
			})
			return
		}
		c.JSON(http.StatusInternalServerError, httpx.ErrorResponse{
			Error: httpx.ErrorDetail{Code: httpx.ErrCodeInternal, Message: err.Error()},
		})
		return
	}

	if err := natsclient.EnsureRoomStream(room.ID); err != nil {
		log.Printf("falha ao preparar stream JetStream da sala %s: %v", room.ID, err)
	}
	h.broadcastPublicRooms(c.Request.Context())

	c.JSON(http.StatusCreated, toRoomResponse(room))
}

func (h *Handler) ListPublicRooms(c *gin.Context) {
	h.expireRoomsAndCleanup(c.Request.Context())

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

	h.expireRoomsAndCleanup(c.Request.Context())

	room, err := h.findRoom(c.Request.Context(), codeOrID)
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

	if room.Status == StatusExpired {
		c.JSON(http.StatusGone, httpx.ErrorResponse{
			Error: httpx.ErrorDetail{Code: httpx.ErrCodeConflict, Message: "A sala nao esta mais ativa"},
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

	h.expireRoomsAndCleanup(c.Request.Context())
	room, err := h.findRoom(c.Request.Context(), codeOrID)
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

	if room.Status == StatusExpired || room.Status == StatusFinished {
		c.JSON(http.StatusGone, httpx.ErrorResponse{
			Error: httpx.ErrorDetail{Code: httpx.ErrCodeConflict, Message: "A sala nao esta mais ativa"},
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
				Error: httpx.ErrorDetail{Code: httpx.ErrCodeValidationFailed, Message: "Voce ja esta nesta sala"},
			})
			return
		}
		if errors.Is(err, ErrUserInActiveRoom) {
			c.JSON(http.StatusConflict, httpx.ErrorResponse{
				Error: httpx.ErrorDetail{Code: httpx.ErrCodeConflict, Message: "Voce ja esta em uma partida ativa"},
			})
			return
		}

		c.JSON(http.StatusInternalServerError, httpx.ErrorResponse{
			Error: httpx.ErrorDetail{Code: httpx.ErrCodeInternal, Message: err.Error()},
		})
		return
	}

	if err := natsclient.EnsureRoomStream(joinedRoom.ID); err != nil {
		log.Printf("falha ao preparar stream JetStream da sala %s: %v", joinedRoom.ID, err)
	}
	if h.presence.UnregisterRoomSpectatorUser(joinedRoom.ID, userID.(string)) {
		h.publishRoomSpectatorsSnapshot(joinedRoom.ID)
	}
	h.publishRoomEvent("player_joined", userID.(string), joinedRoom, map[string]any{
		"user_id": userID.(string),
		"room":    toRoomResponse(joinedRoom),
	})
	h.clearRoomInvitations(joinedRoom.ID, "room_filled")
	h.broadcastPublicRooms(c.Request.Context())

	c.JSON(http.StatusOK, toRoomResponse(joinedRoom))
}

func (h *Handler) LeaveRoom(c *gin.Context) {
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

	h.expireRoomsAndCleanup(c.Request.Context())
	room, err := h.findRoom(c.Request.Context(), codeOrID)
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

	updatedRoom, role, err := h.repo.LeaveRoom(c.Request.Context(), room.ID, userID.(string))
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

	switch role {
	case LeaveRoleCreator:
		h.rematches.ClearRoom(updatedRoom.ID)
		h.clearCanonicalSnapshot(c.Request.Context(), updatedRoom.ID)
		h.publishRoomEvent("room_closed", userID.(string), updatedRoom, map[string]any{
			"reason": "owner_left",
			"room":   toRoomResponse(updatedRoom),
		})
		h.clearRoomInvitations(updatedRoom.ID, "room_closed")
		if err := natsclient.DeleteRoomStream(updatedRoom.ID); err != nil {
			log.Printf("falha ao remover stream da sala encerrada %s: %v", updatedRoom.ID, err)
		}
		h.broadcastPublicRooms(c.Request.Context())

	case LeaveRoleOpponent:
		h.rematches.ClearRoom(updatedRoom.ID)
		h.clearCanonicalSnapshot(c.Request.Context(), updatedRoom.ID)
		h.publishRoomEvent("player_left", userID.(string), updatedRoom, map[string]any{
			"user_id": userID.(string),
			"room":    toRoomResponse(updatedRoom),
		})
		h.broadcastPublicRooms(c.Request.Context())
	}

	c.JSON(http.StatusOK, toRoomResponse(updatedRoom))
}

func (h *Handler) InviteUser(c *gin.Context) {
	userID, exists := c.Get(httpx.ContextKeyUserID)
	if !exists {
		c.JSON(http.StatusUnauthorized, httpx.ErrorResponse{
			Error: httpx.ErrorDetail{Code: httpx.ErrCodeUnauthorized, Message: "Usuario nao autenticado"},
		})
		return
	}
	ownerID := userID.(string)

	codeOrID := c.Param("code_or_id")
	if codeOrID == "" {
		c.JSON(http.StatusBadRequest, httpx.ErrorResponse{
			Error: httpx.ErrorDetail{Code: httpx.ErrCodeValidationFailed, Message: "Parametro code_or_id obrigatorio"},
		})
		return
	}

	var req InviteUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, httpx.ErrorResponse{
			Error: httpx.ErrorDetail{Code: httpx.ErrCodeValidationFailed, Message: err.Error()},
		})
		return
	}
	targetUserID := strings.TrimSpace(req.UserID)
	if targetUserID == "" || targetUserID == ownerID {
		c.JSON(http.StatusBadRequest, httpx.ErrorResponse{
			Error: httpx.ErrorDetail{Code: httpx.ErrCodeValidationFailed, Message: "Usuario convidado invalido"},
		})
		return
	}

	nc := natsclient.GetConn()
	if nc == nil {
		c.JSON(http.StatusServiceUnavailable, httpx.ErrorResponse{
			Error: httpx.ErrorDetail{Code: httpx.ErrCodeInternal, Message: "Servidor de notificacoes temporariamente indisponivel"},
		})
		return
	}

	h.expireRoomsAndCleanup(c.Request.Context())
	room, err := h.findRoom(c.Request.Context(), codeOrID)
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

	if room.CreatorID != ownerID {
		c.JSON(http.StatusForbidden, httpx.ErrorResponse{
			Error: httpx.ErrorDetail{Code: httpx.ErrCodeForbidden, Message: "Apenas o dono da sala pode convidar jogadores"},
		})
		return
	}

	if !roomAcceptsInvite(room) {
		h.clearRoomInvitations(room.ID, "room_unavailable")
		c.JSON(http.StatusConflict, httpx.ErrorResponse{
			Error: httpx.ErrorDetail{Code: httpx.ErrCodeConflict, Message: "A sala nao possui vaga disponivel"},
		})
		return
	}

	if !h.presence.IsOnline(targetUserID) {
		c.JSON(http.StatusConflict, httpx.ErrorResponse{
			Error: httpx.ErrorDetail{Code: httpx.ErrCodeConflict, Message: "Usuario nao esta online"},
		})
		return
	}

	invite := RoomInvitePayload{
		InvitationID: uuid.NewString(),
		Room:         toRoomResponse(room),
		FromUserID:   ownerID,
		ToUserID:     targetUserID,
		CreatedAt:    time.Now().UTC(),
	}
	h.invitations.Upsert(invite)
	h.publishUserEvent("room_invite", targetUserID, invite)

	c.JSON(http.StatusAccepted, invite)
}

func (h *Handler) DeclineInvite(c *gin.Context) {
	userID, exists := c.Get(httpx.ContextKeyUserID)
	if !exists {
		c.JSON(http.StatusUnauthorized, httpx.ErrorResponse{
			Error: httpx.ErrorDetail{Code: httpx.ErrCodeUnauthorized, Message: "Usuario nao autenticado"},
		})
		return
	}

	invitationID := strings.TrimSpace(c.Param("invitation_id"))
	if invitationID == "" {
		c.JSON(http.StatusBadRequest, httpx.ErrorResponse{
			Error: httpx.ErrorDetail{Code: httpx.ErrCodeValidationFailed, Message: "Parametro invitation_id obrigatorio"},
		})
		return
	}

	cleared, ok := h.invitations.ClearInvitation(invitationID, userID.(string), "declined")
	if !ok {
		c.Status(http.StatusNoContent)
		return
	}

	h.publishInviteCleared(*cleared)
	c.JSON(http.StatusOK, cleared)
}

func toRoomResponse(r *Room) RoomResponse {
	return RoomResponse{
		ID:                     r.ID,
		Code:                   r.Code,
		CreatorID:              r.CreatorID,
		OpponentID:             r.OpponentID,
		Status:                 r.Status,
		IsPrivate:              r.IsPrivate,
		CreatedAt:              r.CreatedAt,
		ExpiresAt:              r.ExpiresAt,
		CreatorDisconnectedAt:  r.CreatorDisconnectedAt,
		OpponentDisconnectedAt: r.OpponentDisconnectedAt,
	}
}

func makeWSEvent(eventType string, senderID string, payload any) ([]byte, error) {
	var raw json.RawMessage
	if payload != nil {
		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		raw = payloadBytes
	}

	return json.Marshal(WSEvent{
		Type:     eventType,
		SenderID: senderID,
		Payload:  raw,
	})
}

func (h *Handler) publishRoomEvent(eventType string, senderID string, room *Room, payload any) {
	nc := natsclient.GetConn()
	if nc == nil {
		return
	}

	eventBytes, err := makeWSEvent(eventType, senderID, payload)
	if err != nil {
		log.Printf("falha ao serializar evento %s da sala %s: %v", eventType, room.ID, err)
		return
	}

	if err := nc.Publish(natsclient.RoomEventsSubject(room.ID), eventBytes); err != nil {
		log.Printf("falha ao publicar evento %s da sala %s: %v", eventType, room.ID, err)
	}
}

func (h *Handler) publishUserEvent(eventType string, userID string, payload any) {
	nc := natsclient.GetConn()
	if nc == nil {
		return
	}

	eventBytes, err := makeWSEvent(eventType, "", payload)
	if err != nil {
		log.Printf("falha ao serializar evento %s do usuario %s: %v", eventType, userID, err)
		return
	}

	if err := nc.Publish(natsclient.UserNotificationsSubject(userID), eventBytes); err != nil {
		log.Printf("falha ao publicar evento %s do usuario %s: %v", eventType, userID, err)
	}
}

func (h *Handler) broadcastOnlineUsers() {
	nc := natsclient.GetConn()
	if nc == nil {
		return
	}

	eventBytes, err := makeWSEvent("online_users_snapshot", "", h.presence.OnlineSnapshot())
	if err != nil {
		log.Printf("falha ao serializar usuarios online: %v", err)
		return
	}

	if err := nc.Publish(natsclient.OnlineUsersSubject(), eventBytes); err != nil {
		log.Printf("falha ao publicar usuarios online: %v", err)
	}
}

func (h *Handler) publishRoomSpectatorsSnapshot(roomID string) {
	room := &Room{ID: roomID}
	h.publishRoomEvent("room_spectators_snapshot", "", room, h.presence.RoomSpectatorsSnapshot(roomID))
}

func (h *Handler) broadcastPublicRooms(ctx context.Context) {
	nc := natsclient.GetConn()
	if nc == nil {
		return
	}

	h.reconcileRoomLifecycle(ctx)

	rooms, err := h.repo.ListPublicRooms(ctx)
	if err != nil {
		log.Printf("falha ao listar salas publicas para broadcast: %v", err)
		return
	}

	resp := make([]RoomResponse, len(rooms))
	for i, room := range rooms {
		resp[i] = toRoomResponse(room)
	}

	eventBytes, err := makeWSEvent("public_rooms_snapshot", "", resp)
	if err != nil {
		log.Printf("falha ao serializar lista publica de salas: %v", err)
		return
	}

	if err := nc.Publish(natsclient.PublicRoomsSubject(), eventBytes); err != nil {
		log.Printf("falha ao publicar lista publica de salas: %v", err)
	}
}

func (h *Handler) expireRoomsAndCleanup(ctx context.Context) {
	if h.reconcileRoomLifecycle(ctx) {
		h.broadcastPublicRooms(ctx)
	}
}

func (h *Handler) reconcileRoomLifecycle(ctx context.Context) bool {
	changed := false

	expiredIDs, err := h.repo.ExpireRooms(ctx)
	if err != nil {
		log.Printf("falha ao expirar salas: %v", err)
	} else if len(expiredIDs) > 0 {
		h.deleteRoomStreams(expiredIDs)
		changed = true
	}

	disconnectCutoff := time.Now().UTC().Add(-h.ownerDisconnectTimeout)
	closedIDs, err := h.repo.CloseRoomsWithExpiredCreatorDisconnect(ctx, disconnectCutoff)
	if err != nil {
		log.Printf("falha ao fechar salas com criador desconectado: %v", err)
	} else if len(closedIDs) > 0 {
		h.deleteRoomStreams(closedIDs)
		changed = true
	}

	releasedOpponents, err := h.repo.ReleaseRoomsWithExpiredOpponentDisconnect(ctx, disconnectCutoff)
	if err != nil {
		log.Printf("falha ao liberar oponentes desconectados: %v", err)
	} else if len(releasedOpponents) > 0 {
		for _, released := range releasedOpponents {
			if released.Room == nil {
				continue
			}
			h.rematches.ClearRoom(released.Room.ID)
			h.clearCanonicalSnapshot(ctx, released.Room.ID)
			h.publishRoomEvent("player_left", released.OpponentID, released.Room, map[string]any{
				"user_id": released.OpponentID,
				"reason":  "opponent_disconnect_timeout",
				"room":    toRoomResponse(released.Room),
			})
		}
		changed = true
	}

	return changed
}

func (h *Handler) deleteRoomStreams(roomIDs []string) {
	for _, roomID := range roomIDs {
		h.rematches.ClearRoom(roomID)
		h.clearCanonicalSnapshot(context.Background(), roomID)
		h.clearRoomInvitations(roomID, "room_closed")
		if err := natsclient.DeleteRoomStream(roomID); err != nil {
			log.Printf("falha ao remover stream da sala expirada %s: %v", roomID, err)
		}
	}
}

func (h *Handler) clearRoomInvitations(roomID string, reason string) {
	for _, cleared := range h.invitations.ClearRoom(roomID, reason) {
		h.publishInviteCleared(cleared)
	}
}

func (h *Handler) publishInviteCleared(cleared RoomInviteClearedPayload) {
	h.publishUserEvent("room_invite_cleared", cleared.ToUserID, cleared)
	if cleared.FromUserID != "" && cleared.FromUserID != cleared.ToUserID {
		h.publishUserEvent("room_invite_cleared", cleared.FromUserID, cleared)
	}
}

func roomAcceptsInvite(room *Room) bool {
	return room.Status == StatusWaiting &&
		room.OpponentID == nil &&
		room.CreatorDisconnectedAt == nil &&
		room.OpponentDisconnectedAt == nil &&
		time.Now().Before(room.ExpiresAt)
}
