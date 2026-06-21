package lobby

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/nats-io/nats.go"
	"snooker/lobby/internal/httpx"
	natsclient "snooker/lobby/internal/nats"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Permitir todos os origins para conexoes locais / CORS
	},
}

// WSEvent define o envelope padrao para mensagens em tempo real.
type WSEvent struct {
	Type     string          `json:"type"`
	SenderID string          `json:"sender_id,omitempty"`
	Payload  json.RawMessage `json:"payload,omitempty"`
}

type roomWebSocketState struct {
	mu                    sync.RWMutex
	room                  *Room
	isParticipant         bool
	connectionRole        LeaveRole
	connectionID          string
	spectatorConnectionID string
}

func newRoomWebSocketState(room *Room, isParticipant bool) *roomWebSocketState {
	return &roomWebSocketState{
		room:           room,
		isParticipant:  isParticipant,
		connectionRole: LeaveRoleSpectator,
	}
}

func (s *roomWebSocketState) setParticipant(room *Room, role LeaveRole, connectionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.room = room
	s.isParticipant = true
	s.connectionRole = role
	s.connectionID = connectionID
	s.spectatorConnectionID = ""
}

func (s *roomWebSocketState) setSpectatorConnection(connectionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.spectatorConnectionID = connectionID
}

func (s *roomWebSocketState) eventSnapshot() (*Room, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.room, s.isParticipant
}

func (s *roomWebSocketState) disconnectSnapshot() (*Room, LeaveRole, string, string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.room, s.connectionRole, s.connectionID, s.spectatorConnectionID, s.isParticipant
}

func (h *Handler) HandlePublicRoomsWS(c *gin.Context) {
	nc := natsclient.GetConn()
	if nc == nil {
		log.Printf("conexao WebSocket recusada: NATS nao esta conectado")
		c.JSON(http.StatusServiceUnavailable, httpx.ErrorResponse{
			Error: httpx.ErrorDetail{Code: httpx.ErrCodeInternal, Message: "Servidor de mensageria temporariamente indisponivel"},
		})
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("falha ao realizar upgrade de WebSocket publico: %v", err)
		return
	}
	defer conn.Close()

	done, signalDone := websocketDone(conn)
	writeChan := make(chan []byte, 256)
	writeDone := make(chan struct{})
	go websocketWriteLoop(conn, writeChan, done, signalDone, writeDone)

	sub, err := nc.Subscribe(natsclient.PublicRoomsSubject(), func(msg *nats.Msg) {
		enqueueWS(writeChan, done, msg.Data)
	})
	if err != nil {
		log.Printf("falha ao subscrever lista publica de salas: %v", err)
		signalDone()
		<-writeDone
		return
	}
	defer sub.Unsubscribe()

	h.sendPublicRoomsSnapshot(c.Request.Context(), writeChan, done)

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}

	signalDone()
	<-writeDone
}

func (h *Handler) HandleNotificationsWS(c *gin.Context) {
	userIDVal, exists := c.Get(httpx.ContextKeyUserID)
	if !exists {
		c.JSON(http.StatusUnauthorized, httpx.ErrorResponse{
			Error: httpx.ErrorDetail{Code: httpx.ErrCodeUnauthorized, Message: "Usuario nao autenticado"},
		})
		return
	}
	userID := userIDVal.(string)

	nc := natsclient.GetConn()
	if nc == nil {
		log.Printf("conexao WebSocket recusada: NATS nao esta conectado")
		c.JSON(http.StatusServiceUnavailable, httpx.ErrorResponse{
			Error: httpx.ErrorDetail{Code: httpx.ErrCodeInternal, Message: "Servidor de notificacoes temporariamente indisponivel"},
		})
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("falha ao realizar upgrade de WebSocket de notificacoes: %v", err)
		return
	}
	defer conn.Close()

	done, signalDone := websocketDone(conn)
	writeChan := make(chan []byte, 256)
	writeDone := make(chan struct{})
	go websocketWriteLoop(conn, writeChan, done, signalDone, writeDone)

	userSub, err := nc.Subscribe(natsclient.UserNotificationsSubject(userID), func(msg *nats.Msg) {
		enqueueWS(writeChan, done, msg.Data)
	})
	if err != nil {
		log.Printf("falha ao subscrever notificacoes do usuario %s: %v", userID, err)
		signalDone()
		<-writeDone
		return
	}
	defer userSub.Unsubscribe()

	onlineSub, err := nc.Subscribe(natsclient.OnlineUsersSubject(), func(msg *nats.Msg) {
		enqueueWS(writeChan, done, msg.Data)
	})
	if err != nil {
		log.Printf("falha ao subscrever usuarios online para %s: %v", userID, err)
		signalDone()
		<-writeDone
		return
	}
	defer onlineSub.Unsubscribe()

	connectionID := uuid.NewString()
	h.presence.RegisterOnline(userID, connectionID)
	h.sendOnlineUsersSnapshot(writeChan, done)
	h.sendPendingInvites(userID, writeChan, done)
	h.broadcastOnlineUsers()

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}

	h.presence.UnregisterOnline(userID, connectionID)
	h.broadcastOnlineUsers()
	signalDone()
	<-writeDone
}

func (h *Handler) HandleWS(c *gin.Context) {
	userIDVal, exists := c.Get(httpx.ContextKeyUserID)
	if !exists {
		c.JSON(http.StatusUnauthorized, httpx.ErrorResponse{
			Error: httpx.ErrorDetail{Code: httpx.ErrCodeUnauthorized, Message: "Usuario nao autenticado"},
		})
		return
	}
	userID := userIDVal.(string)

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

	isParticipant := isRoomParticipant(room, userID)
	if room.IsPrivate && !isParticipant {
		c.JSON(http.StatusForbidden, httpx.ErrorResponse{
			Error: httpx.ErrorDetail{Code: httpx.ErrCodeForbidden, Message: "Acesso proibido a esta sala privada"},
		})
		return
	}

	nc := natsclient.GetConn()
	js := natsclient.GetJetStream()
	if nc == nil || js == nil {
		log.Printf("conexao WebSocket recusada: NATS/JetStream nao esta conectado")
		c.JSON(http.StatusServiceUnavailable, httpx.ErrorResponse{
			Error: httpx.ErrorDetail{Code: httpx.ErrCodeInternal, Message: "Servidor de mensageria temporariamente indisponivel"},
		})
		return
	}

	if err := natsclient.EnsureRoomStream(room.ID); err != nil {
		log.Printf("falha ao preparar stream da sala %s: %v", room.ID, err)
		c.JSON(http.StatusServiceUnavailable, httpx.ErrorResponse{
			Error: httpx.ErrorDetail{Code: httpx.ErrCodeInternal, Message: "Historico de chat temporariamente indisponivel"},
		})
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("falha ao realizar upgrade de WebSocket: %v", err)
		return
	}
	defer conn.Close()

	done, signalDone := websocketDone(conn)
	writeChan := make(chan []byte, 256)
	writeDone := make(chan struct{})
	go websocketWriteLoop(conn, writeChan, done, signalDone, writeDone)

	state := newRoomWebSocketState(room, isParticipant)
	if userID == room.CreatorID {
		connectionID := uuid.NewString()
		state.setParticipant(room, LeaveRoleCreator, connectionID)
		if connectedRoom, err := h.repo.MarkCreatorConnected(context.Background(), room.ID, userID, connectionID); err == nil {
			room = connectedRoom
			state.setParticipant(room, LeaveRoleCreator, connectionID)
			h.publishRoomEvent("owner_reconnected", userID, room, map[string]any{
				"user_id": userID,
				"room":    toRoomResponse(room),
			})
			h.broadcastPublicRooms(context.Background())
		} else {
			log.Printf("falha ao marcar criador conectado na sala %s: %v", room.ID, err)
		}
	} else if room.OpponentID != nil && *room.OpponentID == userID {
		connectionID := uuid.NewString()
		state.setParticipant(room, LeaveRoleOpponent, connectionID)
		if connectedRoom, err := h.repo.MarkOpponentConnected(context.Background(), room.ID, userID, connectionID); err == nil {
			room = connectedRoom
			state.setParticipant(room, LeaveRoleOpponent, connectionID)
			h.publishRoomEvent("player_reconnected", userID, room, map[string]any{
				"user_id": userID,
				"room":    toRoomResponse(room),
			})
			h.broadcastPublicRooms(context.Background())
		} else {
			log.Printf("falha ao marcar oponente conectado na sala %s: %v", room.ID, err)
		}
	}
	if isParticipant && h.presence.UnregisterRoomSpectatorUser(room.ID, userID) {
		h.publishRoomSpectatorsSnapshot(room.ID)
	}

	roomEventsSubject := natsclient.RoomEventsSubject(room.ID)
	roomChatSubject := natsclient.RoomChatSubject(room.ID)

	roomSub, err := nc.Subscribe(roomEventsSubject, func(msg *nats.Msg) {
		h.promoteRoomWebSocketFromEvent(state, userID, msg.Data)
		enqueueWS(writeChan, done, msg.Data)
	})
	if err != nil {
		log.Printf("falha ao subscrever eventos da sala %s: %v", room.ID, err)
		signalDone()
		<-writeDone
		return
	}
	defer roomSub.Unsubscribe()

	roomStateBytes, err := makeWSEvent("room_state", "", toRoomResponse(room))
	if err == nil {
		enqueueWS(writeChan, done, roomStateBytes)
	}

	chatSub, err := js.Subscribe(roomChatSubject, func(msg *nats.Msg) {
		enqueueWS(writeChan, done, msg.Data)
	}, nats.DeliverAll(), nats.AckNone())
	if err != nil {
		log.Printf("falha ao subscrever historico de chat da sala %s: %v", room.ID, err)
		signalDone()
		<-writeDone
		return
	}
	defer chatSub.Unsubscribe()

	if !isParticipant {
		spectatorConnectionID := uuid.NewString()
		state.setSpectatorConnection(spectatorConnectionID)
		h.presence.RegisterRoomSpectator(room.ID, userID, spectatorConnectionID)
	}
	h.sendRoomSpectatorsSnapshot(room.ID, writeChan, done)
	_, _, _, spectatorConnectionID, _ := state.disconnectSnapshot()
	if spectatorConnectionID != "" {
		h.publishRoomSpectatorsSnapshot(room.ID)
	}

	h.publishRoomEvent("viewer_joined", userID, room, map[string]any{
		"user_id":        userID,
		"is_participant": isParticipant,
	})

	for {
		_, msgBytes, err := conn.ReadMessage()
		if err != nil {
			break
		}

		var event WSEvent
		if err := json.Unmarshal(msgBytes, &event); err != nil {
			log.Printf("evento WS malformado recebido: %v", err)
			continue
		}

		eventRoom, eventIsParticipant := state.eventSnapshot()
		if !h.handleRoomClientEvent(c.Request.Context(), js, nc, eventRoom, userID, eventIsParticipant, event) {
			continue
		}
	}

	disconnectRoom, connectionRole, connectionID, spectatorConnectionID, participantAtDisconnect := state.disconnectSnapshot()
	if spectatorConnectionID != "" {
		h.presence.UnregisterRoomSpectator(disconnectRoom.ID, userID, spectatorConnectionID)
		h.publishRoomSpectatorsSnapshot(disconnectRoom.ID)
	}

	switch connectionRole {
	case LeaveRoleCreator:
		disconnectedAt := time.Now().UTC()
		marked, err := h.repo.MarkCreatorDisconnected(context.Background(), disconnectRoom.ID, userID, connectionID, disconnectedAt)
		if err != nil {
			log.Printf("falha ao marcar criador desconectado na sala %s: %v", disconnectRoom.ID, err)
		}
		if marked {
			h.publishRoomEvent("owner_disconnected", userID, disconnectRoom, map[string]any{
				"user_id":         userID,
				"disconnects_at":  disconnectedAt.Add(h.ownerDisconnectTimeout).Format(time.RFC3339Nano),
				"timeout_seconds": int(h.ownerDisconnectTimeout.Seconds()),
			})
			h.broadcastPublicRooms(context.Background())
			h.scheduleOwnerDisconnectClose(disconnectRoom.ID, userID)
		}

	case LeaveRoleOpponent:
		disconnectedAt := time.Now().UTC()
		marked, err := h.repo.MarkOpponentDisconnected(context.Background(), disconnectRoom.ID, userID, connectionID, disconnectedAt)
		if err != nil {
			log.Printf("falha ao marcar oponente desconectado na sala %s: %v", disconnectRoom.ID, err)
		}
		if marked {
			h.publishRoomEvent("player_disconnected", userID, disconnectRoom, map[string]any{
				"user_id":         userID,
				"disconnects_at":  disconnectedAt.Add(h.ownerDisconnectTimeout).Format(time.RFC3339Nano),
				"timeout_seconds": int(h.ownerDisconnectTimeout.Seconds()),
			})
			h.broadcastPublicRooms(context.Background())
			h.scheduleOpponentDisconnectRelease(disconnectRoom.ID, userID)
		}

	default:
		if participantAtDisconnect {
			h.publishRoomEvent("player_disconnected", userID, disconnectRoom, map[string]any{"user_id": userID})
		}
	}

	signalDone()
	<-writeDone
}

func (h *Handler) promoteRoomWebSocketFromEvent(state *roomWebSocketState, userID string, msgData []byte) {
	var event WSEvent
	if err := json.Unmarshal(msgData, &event); err != nil || event.Type != "player_joined" {
		return
	}

	room, isParticipant := state.eventSnapshot()
	if isParticipant || room == nil {
		return
	}

	joinedUserID := event.SenderID
	var payload struct {
		UserID string `json:"user_id"`
	}
	if len(event.Payload) > 0 {
		if err := json.Unmarshal(event.Payload, &payload); err == nil && payload.UserID != "" {
			joinedUserID = payload.UserID
		}
	}
	if joinedUserID != userID {
		return
	}

	currentRoom, err := h.repo.GetRoomByID(context.Background(), room.ID)
	if err != nil {
		log.Printf("falha ao atualizar papel da conexao WebSocket na sala %s: %v", room.ID, err)
		return
	}

	role := roomParticipantRole(currentRoom, userID)
	if role == LeaveRoleSpectator {
		return
	}

	connectionID := uuid.NewString()
	connectedRoom := currentRoom
	switch role {
	case LeaveRoleCreator:
		if roomWithConnection, err := h.repo.MarkCreatorConnected(context.Background(), currentRoom.ID, userID, connectionID); err == nil {
			connectedRoom = roomWithConnection
		} else {
			log.Printf("falha ao promover criador no WebSocket da sala %s: %v", currentRoom.ID, err)
			return
		}
	case LeaveRoleOpponent:
		if roomWithConnection, err := h.repo.MarkOpponentConnected(context.Background(), currentRoom.ID, userID, connectionID); err == nil {
			connectedRoom = roomWithConnection
		} else {
			log.Printf("falha ao promover oponente no WebSocket da sala %s: %v", currentRoom.ID, err)
			return
		}
	}

	state.setParticipant(connectedRoom, role, connectionID)
	if h.presence.UnregisterRoomSpectatorUser(connectedRoom.ID, userID) {
		h.publishRoomSpectatorsSnapshot(connectedRoom.ID)
	}
}

func (h *Handler) handleRoomClientEvent(ctx context.Context, js nats.JetStreamContext, nc *nats.Conn, room *Room, userID string, isParticipant bool, event WSEvent) bool {
	if currentRoom, err := h.repo.GetRoomByID(ctx, room.ID); err == nil {
		room = currentRoom
		isParticipant = isRoomParticipant(room, userID)
	} else if !errors.Is(err, ErrRoomNotFound) {
		log.Printf("falha ao atualizar sala %s antes de processar evento %s: %v", room.ID, event.Type, err)
	}

	switch event.Type {
	case "chat_message":
		if !isParticipant {
			return false
		}
		payload, ok := chatPayload(event.Payload)
		if !ok {
			return false
		}

		eventBytes, err := makeWSEvent("chat_message", userID, payload)
		if err != nil {
			log.Printf("falha ao serializar mensagem de chat da sala %s: %v", room.ID, err)
			return false
		}
		if _, err := js.Publish(natsclient.RoomChatSubject(room.ID), eventBytes); err != nil {
			log.Printf("falha ao publicar chat da sala %s no JetStream: %v", room.ID, err)
		}
		return true

	case "player_ready":
		if !isParticipant {
			return false
		}
		event.SenderID = userID
		eventBytes, err := json.Marshal(event)
		if err != nil {
			return false
		}
		_ = nc.Publish(natsclient.RoomEventsSubject(room.ID), eventBytes)
		return true

	case "match_start":
		if userID != room.CreatorID {
			return false
		}
		startedRoom, err := h.repo.StartRoom(ctx, room.ID)
		if err != nil {
			log.Printf("falha ao iniciar partida da sala %s: %v", room.ID, err)
			return false
		}
		eventBytes, err := makeWSEvent("match_start", userID, map[string]any{
			"room": toRoomResponse(startedRoom),
		})
		if err != nil {
			return false
		}
		h.clearCachedSnapshot(room.ID)
		_ = nc.Publish(natsclient.RoomEventsSubject(room.ID), eventBytes)
		h.clearRoomInvitations(room.ID, "room_filled")
		h.rematches.ClearRoom(room.ID)
		h.broadcastPublicRooms(context.Background())
		return true

	case "cue_state":
		if !isParticipant || room.Status != StatusPlaying {
			return false
		}
		payload, ok := cueStatePayload(event.Payload, userID)
		if !ok {
			return false
		}
		eventBytes, err := makeWSEvent("cue_state", userID, payload)
		if err != nil {
			return false
		}
		_ = nc.Publish(natsclient.RoomEventsSubject(room.ID), eventBytes)
		return true

	case "game_state_sync":
		if !isParticipant || room.Status != StatusPlaying || !isAuthoritativeGameStateSender(room, userID) {
			return false
		}
		if !h.setCachedSnapshot(room.ID, event.Payload) {
			return false
		}
		event.SenderID = userID
		eventBytes, err := json.Marshal(event)
		if err != nil {
			return false
		}
		_ = nc.Publish(natsclient.RoomEventsSubject(room.ID), eventBytes)
		return true

	case "request_game_state":
		if room.Status != StatusPlaying {
			return false
		}
		if cachedSnap, ok := h.getCachedSnapshot(room.ID); ok {
			var cachedEvent WSEvent
			cachedEvent.Type = "game_state_sync"
			cachedEvent.SenderID = "server"
			cachedEvent.Payload = cachedSnap
			eventBytes, err := json.Marshal(cachedEvent)
			if err == nil {
				_ = nc.Publish(natsclient.RoomEventsSubject(room.ID), eventBytes)
			}
		} else {
			event.SenderID = userID
			eventBytes, err := json.Marshal(event)
			if err == nil {
				_ = nc.Publish(natsclient.RoomEventsSubject(room.ID), eventBytes)
			}
		}
		return true

	case "match_end":
		if !isParticipant || room.Status != StatusPlaying {
			return false
		}
		matchEnd := matchEndPayload(event.Payload)
		finishedRoom, err := h.repo.FinishRoom(ctx, room.ID)
		if err != nil {
			log.Printf("falha ao finalizar partida da sala %s: %v", room.ID, err)
			return false
		}
		h.clearCachedSnapshot(room.ID)
		winnerUserID := matchEnd.WinnerUserID
		if !isRoomParticipant(finishedRoom, winnerUserID) {
			winnerUserID = userID
		}
		xpAwards, err := h.xpAwarder.AwardMatchXP(ctx, winnerUserID, roomParticipantIDs(finishedRoom))
		if err != nil {
			log.Printf("falha ao premiar XP da sala %s: %v", room.ID, err)
		}
		if finishedRoom.OpponentID != nil {
			h.rematches.ConfigureRoom(finishedRoom.ID, finishedRoom.CreatorID, *finishedRoom.OpponentID)
		}
		eventBytes, err := makeWSEvent("match_finished", userID, map[string]any{
			"room":           toRoomResponse(finishedRoom),
			"reason":         matchEnd.Reason,
			"winner_user_id": winnerUserID,
			"xp_awards":      xpAwards,
		})
		if err != nil {
			return false
		}
		_ = nc.Publish(natsclient.RoomEventsSubject(room.ID), eventBytes)
		h.broadcastPublicRooms(context.Background())
		return true

	case "rematch_request":
		if !isParticipant || room.Status != StatusFinished || room.OpponentID == nil {
			return false
		}
		h.rematches.ConfigureRoom(room.ID, room.CreatorID, *room.OpponentID)
		requestedUserIDs, allReady := h.rematches.Request(room.ID, userID)

		eventBytes, err := makeWSEvent("rematch_requested", userID, map[string]any{
			"room_id":            room.ID,
			"user_id":            userID,
			"requested_user_ids": requestedUserIDs,
		})
		if err == nil {
			_ = nc.Publish(natsclient.RoomEventsSubject(room.ID), eventBytes)
		}

		if !allReady {
			return true
		}

		startedRoom, err := h.repo.StartRematchRoom(ctx, room.ID)
		if err != nil {
			log.Printf("falha ao iniciar revanche da sala %s: %v", room.ID, err)
			return false
		}
		h.rematches.ClearRoom(room.ID)
		h.clearCachedSnapshot(room.ID)
		eventBytes, err = makeWSEvent("match_start", userID, map[string]any{
			"room": toRoomResponse(startedRoom),
		})
		if err != nil {
			return false
		}
		_ = nc.Publish(natsclient.RoomEventsSubject(room.ID), eventBytes)
		h.broadcastPublicRooms(context.Background())
		return true

	case "room_close_request":
		if userID != room.CreatorID {
			return false
		}
		if err := h.repo.CloseRoom(ctx, room.ID); err != nil {
			log.Printf("falha ao encerrar sala %s pelo dono: %v", room.ID, err)
			return false
		}
		h.rematches.ClearRoom(room.ID)
		h.clearCachedSnapshot(room.ID)
		h.clearRoomInvitations(room.ID, "room_closed")
		eventBytes, err := makeWSEvent("room_closed", userID, map[string]any{
			"reason": "owner_closed",
			"room":   toRoomResponse(room),
		})
		if err == nil {
			_ = nc.Publish(natsclient.RoomEventsSubject(room.ID), eventBytes)
		}
		if err := natsclient.DeleteRoomStream(room.ID); err != nil {
			log.Printf("falha ao remover stream da sala encerrada %s: %v", room.ID, err)
		}
		h.broadcastPublicRooms(context.Background())
		return true

	default:
		if !isParticipant {
			return false
		}
		event.SenderID = userID
		eventBytes, err := json.Marshal(event)
		if err != nil {
			return false
		}
		_ = nc.Publish(natsclient.RoomEventsSubject(room.ID), eventBytes)
		return true
	}
}

type cueState struct {
	MatchID    string  `json:"match_id,omitempty"`
	ShotSeq    int     `json:"shot_seq"`
	TurnUserID string  `json:"turn_user_id,omitempty"`
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
	Angle      float64 `json:"angle"`
	Power      float64 `json:"power"`
	IsAiming   bool    `json:"is_aiming"`
	ClientSeq  int64   `json:"client_seq"`
}

func cueStatePayload(raw json.RawMessage, userID string) (map[string]any, bool) {
	var payload cueState
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, false
	}
	turnUserID := strings.TrimSpace(payload.TurnUserID)

	if !finite(payload.X) ||
		!finite(payload.Y) ||
		!finite(payload.Angle) ||
		!finite(payload.Power) ||
		payload.Power < 0 ||
		payload.Power > 100 ||
		payload.ShotSeq < 0 ||
		(turnUserID != "" && turnUserID != userID) {
		return nil, false
	}

	angle := normalizeAngle(payload.Angle)

	return map[string]any{
		"match_id":              strings.TrimSpace(payload.MatchID),
		"shot_seq":              payload.ShotSeq,
		"turn_user_id":          turnUserID,
		"x":                     payload.X,
		"y":                     payload.Y,
		"angle":                 angle,
		"power":                 payload.Power,
		"is_aiming":             payload.IsAiming,
		"client_seq":            payload.ClientSeq,
		"server_received_at_ms": time.Now().UnixMilli(),
	}, true
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func normalizeAngle(value float64) float64 {
	fullTurn := math.Pi * 2
	normalized := math.Mod(value, fullTurn)
	if normalized <= -math.Pi {
		normalized += fullTurn
	}
	if normalized > math.Pi {
		normalized -= fullTurn
	}
	return normalized
}

type parsedMatchEndPayload struct {
	Reason       string
	WinnerUserID string
}

func matchEndPayload(raw json.RawMessage) parsedMatchEndPayload {
	var payload struct {
		Reason       string `json:"reason"`
		WinnerUserID string `json:"winner_user_id"`
	}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &payload)
	}

	reason := strings.TrimSpace(payload.Reason)
	if reason == "" {
		reason = "normal"
	}
	if len(reason) > 80 {
		reason = reason[:80]
	}

	return parsedMatchEndPayload{
		Reason:       reason,
		WinnerUserID: strings.TrimSpace(payload.WinnerUserID),
	}
}

func (h *Handler) sendPublicRoomsSnapshot(ctx context.Context, writeChan chan<- []byte, done <-chan struct{}) {
	h.expireRoomsAndCleanup(ctx)

	rooms, err := h.repo.ListPublicRooms(ctx)
	if err != nil {
		log.Printf("falha ao carregar snapshot de salas publicas: %v", err)
		return
	}

	resp := make([]RoomResponse, len(rooms))
	for i, room := range rooms {
		resp[i] = toRoomResponse(room)
	}

	eventBytes, err := makeWSEvent("public_rooms_snapshot", "", resp)
	if err != nil {
		log.Printf("falha ao serializar snapshot de salas publicas: %v", err)
		return
	}

	enqueueWS(writeChan, done, eventBytes)
}

func (h *Handler) sendOnlineUsersSnapshot(writeChan chan<- []byte, done <-chan struct{}) {
	eventBytes, err := makeWSEvent("online_users_snapshot", "", h.presence.OnlineSnapshot())
	if err != nil {
		log.Printf("falha ao serializar snapshot de usuarios online: %v", err)
		return
	}
	enqueueWS(writeChan, done, eventBytes)
}

func (h *Handler) sendRoomSpectatorsSnapshot(roomID string, writeChan chan<- []byte, done <-chan struct{}) {
	eventBytes, err := makeWSEvent("room_spectators_snapshot", "", h.presence.RoomSpectatorsSnapshot(roomID))
	if err != nil {
		log.Printf("falha ao serializar snapshot de espectadores da sala %s: %v", roomID, err)
		return
	}
	enqueueWS(writeChan, done, eventBytes)
}

func (h *Handler) sendPendingInvites(userID string, writeChan chan<- []byte, done <-chan struct{}) {
	for _, invite := range h.invitations.ListForUser(userID) {
		eventBytes, err := makeWSEvent("room_invite", "", invite)
		if err != nil {
			log.Printf("falha ao serializar convite pendente %s: %v", invite.InvitationID, err)
			continue
		}
		enqueueWS(writeChan, done, eventBytes)
	}
}

func (h *Handler) scheduleOwnerDisconnectClose(roomID string, creatorID string) {
	timeout := h.ownerDisconnectTimeout
	go func() {
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		<-timer.C

		closed, err := h.repo.CloseRoomIfCreatorDisconnected(context.Background(), roomID, time.Now().UTC().Add(-timeout))
		if err != nil {
			log.Printf("falha ao fechar sala %s apos timeout do criador: %v", roomID, err)
			return
		}
		if !closed {
			return
		}

		h.rematches.ClearRoom(roomID)
		eventBytes, err := makeWSEvent("room_closed", creatorID, map[string]string{"reason": "owner_disconnect_timeout"})
		if err == nil {
			if nc := natsclient.GetConn(); nc != nil {
				_ = nc.Publish(natsclient.RoomEventsSubject(roomID), eventBytes)
			}
		}

		if err := natsclient.DeleteRoomStream(roomID); err != nil {
			log.Printf("falha ao remover stream da sala %s: %v", roomID, err)
		}
		h.clearRoomInvitations(roomID, "room_closed")
		h.broadcastPublicRooms(context.Background())
	}()
}

func (h *Handler) scheduleOpponentDisconnectRelease(roomID string, opponentID string) {
	timeout := h.ownerDisconnectTimeout
	go func() {
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		<-timer.C

		room, released, err := h.repo.ReleaseOpponentIfDisconnected(context.Background(), roomID, time.Now().UTC().Add(-timeout))
		if err != nil {
			log.Printf("falha ao liberar oponente %s da sala %s apos timeout: %v", opponentID, roomID, err)
			return
		}
		if !released || room == nil {
			return
		}

		h.rematches.ClearRoom(roomID)
		h.publishRoomEvent("player_left", opponentID, room, map[string]any{
			"user_id": opponentID,
			"reason":  "opponent_disconnect_timeout",
			"room":    toRoomResponse(room),
		})
		h.broadcastPublicRooms(context.Background())
	}()
}

func (h *Handler) findRoom(ctx context.Context, codeOrID string) (*Room, error) {
	if len(codeOrID) == 6 {
		return h.repo.GetRoomByCode(ctx, codeOrID)
	}
	return h.repo.GetRoomByID(ctx, codeOrID)
}

func isRoomParticipant(room *Room, userID string) bool {
	return room.CreatorID == userID || (room.OpponentID != nil && *room.OpponentID == userID)
}

func roomParticipantIDs(room *Room) []string {
	participants := []string{room.CreatorID}
	if room.OpponentID != nil && *room.OpponentID != "" {
		participants = append(participants, *room.OpponentID)
	}
	return participants
}

func roomParticipantRole(room *Room, userID string) LeaveRole {
	if room.CreatorID == userID {
		return LeaveRoleCreator
	}
	if room.OpponentID != nil && *room.OpponentID == userID {
		return LeaveRoleOpponent
	}
	return LeaveRoleSpectator
}

func isAuthoritativeGameStateSender(room *Room, userID string) bool {
	return room != nil && room.CreatorID == userID
}

func chatPayload(raw json.RawMessage) (map[string]any, bool) {
	var payload struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, false
	}

	text := strings.TrimSpace(payload.Text)
	if text == "" {
		return nil, false
	}

	runes := []rune(text)
	if len(runes) > 500 {
		text = string(runes[:500])
	}

	return map[string]any{
		"message_id": uuid.NewString(),
		"text":       text,
		"created_at": time.Now().UTC().Format(time.RFC3339Nano),
	}, true
}

func websocketDone(conn *websocket.Conn) (<-chan struct{}, func()) {
	done := make(chan struct{})
	var once sync.Once
	signalDone := func() {
		once.Do(func() {
			close(done)
			_ = conn.Close()
		})
	}
	return done, signalDone
}

func websocketWriteLoop(conn *websocket.Conn, writeChan <-chan []byte, done <-chan struct{}, signalDone func(), writeDone chan<- struct{}) {
	defer close(writeDone)
	for {
		select {
		case msgData := <-writeChan:
			if err := conn.WriteMessage(websocket.TextMessage, msgData); err != nil {
				log.Printf("erro ao enviar mensagem WebSocket: %v", err)
				signalDone()
				return
			}
		case <-done:
			return
		}
	}
}

func enqueueWS(writeChan chan<- []byte, done <-chan struct{}, msg []byte) {
	select {
	case writeChan <- msg:
	case <-done:
	default:
		log.Printf("mensagem WebSocket descartada: cliente lento")
	}
}

func (h *Handler) getCachedSnapshot(roomID string) (json.RawMessage, bool) {
	h.snapshotsMu.RLock()
	defer h.snapshotsMu.RUnlock()
	if h.snapshots == nil {
		return nil, false
	}
	snap, ok := h.snapshots[roomID]
	return snap, ok
}

type matchSnapshotVersion struct {
	ShotSeq     int   `json:"shot_seq"`
	UpdatedAtMS int64 `json:"updated_at_ms"`
}

func snapshotVersion(raw json.RawMessage) (matchSnapshotVersion, bool) {
	var version matchSnapshotVersion
	if err := json.Unmarshal(raw, &version); err != nil {
		return version, false
	}
	if version.ShotSeq < 0 || version.UpdatedAtMS <= 0 {
		return version, false
	}
	return version, true
}

func snapshotIsNewer(incoming, current matchSnapshotVersion) bool {
	if incoming.ShotSeq < current.ShotSeq {
		return false
	}
	if incoming.ShotSeq == current.ShotSeq && incoming.UpdatedAtMS <= current.UpdatedAtMS {
		return false
	}
	return true
}

func (h *Handler) setCachedSnapshot(roomID string, snap json.RawMessage) bool {
	incomingVersion, ok := snapshotVersion(snap)
	if !ok {
		return false
	}

	h.snapshotsMu.Lock()
	defer h.snapshotsMu.Unlock()
	if h.snapshots == nil {
		h.snapshots = make(map[string]json.RawMessage)
	}
	if currentSnap, exists := h.snapshots[roomID]; exists {
		currentVersion, ok := snapshotVersion(currentSnap)
		if ok && !snapshotIsNewer(incomingVersion, currentVersion) {
			return false
		}
	}

	h.snapshots[roomID] = append(json.RawMessage(nil), snap...)
	return true
}

func (h *Handler) clearCachedSnapshot(roomID string) {
	h.snapshotsMu.Lock()
	defer h.snapshotsMu.Unlock()
	if h.snapshots != nil {
		delete(h.snapshots, roomID)
	}
}
