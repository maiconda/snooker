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

	roomConnectionID := uuid.NewString()
	if _, ok := h.presence.RegisterRoomConnectionIfFree(room.ID, userID, roomConnectionID); !ok {
		c.JSON(http.StatusConflict, httpx.ErrorResponse{
			Error: httpx.ErrorDetail{Code: httpx.ErrCodeConflict, Message: "Voce ja esta em uma partida ativa"},
		})
		return
	}
	defer h.presence.UnregisterRoomConnection(room.ID, userID, roomConnectionID)

	if activeInOtherRoom, err := h.repo.UserHasActiveRoom(c.Request.Context(), userID, room.ID); err != nil {
		c.JSON(http.StatusInternalServerError, httpx.ErrorResponse{
			Error: httpx.ErrorDetail{Code: httpx.ErrCodeInternal, Message: err.Error()},
		})
		return
	} else if activeInOtherRoom {
		c.JSON(http.StatusConflict, httpx.ErrorResponse{
			Error: httpx.ErrorDetail{Code: httpx.ErrCodeConflict, Message: "Voce ja esta em uma partida ativa"},
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
		connectionID := roomConnectionID
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
		connectionID := roomConnectionID
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
		spectatorConnectionID := roomConnectionID
		h.presence.RegisterRoomSpectator(room.ID, userID, spectatorConnectionID)
		state.setSpectatorConnection(spectatorConnectionID)
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
		h.matchStateMu.Lock()
		h.clearCachedSnapshot(startedRoom.ID)
		snapshot := newInitialMatchSnapshot(startedRoom)
		stored := h.storeCanonicalSnapshot(ctx, startedRoom.ID, snapshot)
		h.matchStateMu.Unlock()
		if !stored {
			return false
		}
		_ = nc.Publish(natsclient.RoomEventsSubject(room.ID), eventBytes)
		publishCanonicalSnapshot(nc, startedRoom.ID, snapshot)
		h.clearRoomInvitations(room.ID, "room_filled")
		h.rematches.ClearRoom(room.ID)
		h.broadcastPublicRooms(context.Background())
		return true

	case "cue_state":
		if !isParticipant || room.Status != StatusPlaying || roomHasReconnectingPlayer(room) {
			return false
		}
		payload, ok := cueStatePayload(event.Payload, userID)
		if !ok {
			return false
		}
		snapshot, ok := h.getCanonicalSnapshot(ctx, room.ID)
		if !ok ||
			snapshot.Status != matchStatusAiming ||
			isTurnExpired(snapshot, time.Now().UnixMilli()) ||
			snapshot.TurnUserID != userID ||
			(payload["match_id"] != "" && payload["match_id"] != room.ID) ||
			payload["shot_seq"] != snapshot.ShotSeq {
			return false
		}
		eventBytes, err := makeWSEvent("cue_state", userID, payload)
		if err != nil {
			return false
		}
		_ = nc.Publish(natsclient.RoomEventsSubject(room.ID), eventBytes)
		return true

	case "shot_started":
		if !isParticipant || room.Status != StatusPlaying || roomHasReconnectingPlayer(room) {
			return false
		}
		intent, ok := parseShotIntent(event.Payload)
		if !ok {
			return false
		}
		if intent.MatchID != "" && intent.MatchID != room.ID {
			return false
		}
		var activeShot activeShotState
		var timeoutSnapshot matchSnapshot
		var timeoutPayload turnTimeoutPayload
		var timedOut bool
		started := func() bool {
			h.matchStateMu.Lock()
			defer h.matchStateMu.Unlock()

			snapshot, ok := h.ensureCanonicalSnapshot(ctx, room)
			if !ok ||
				snapshot.Status != matchStatusAiming ||
				snapshot.WinnerUserID != "" ||
				snapshot.TurnUserID != userID {
				return false
			}

			now := time.Now().UnixMilli()
			if isTurnExpired(snapshot, now) {
				nextSnapshot, payload, ok := applyTurnTimeout(room, snapshot, now)
				if ok && h.storeCanonicalSnapshot(ctx, room.ID, nextSnapshot) {
					timeoutSnapshot = nextSnapshot
					timeoutPayload = payload
					timedOut = true
				}
				return false
			}

			nextSeq := snapshot.ShotSeq + 1
			activeShot = activeShotState{
				MatchID:           room.ID,
				ShotSeq:           nextSeq,
				ShooterUserID:     userID,
				Angle:             intent.Angle,
				Power:             intent.Power,
				ServerStartedAtMS: now,
			}
			snapshot.ShotSeq = nextSeq
			snapshot.Status = matchStatusMoving
			snapshot.TurnStartedAtMS = 0
			snapshot.TurnDeadlineAtMS = 0
			snapshot.ActiveShot = &activeShot
			snapshot.UpdatedAtMS = now
			return h.storeCanonicalSnapshot(ctx, room.ID, snapshot)
		}()
		if !started {
			if timedOut {
				publishTurnTimeout(nc, room.ID, timeoutPayload)
				publishCanonicalSnapshot(nc, room.ID, timeoutSnapshot)
				return true
			}
			return false
		}

		eventBytes, err := makeWSEvent("shot_started", userID, activeShot)
		if err != nil {
			return false
		}
		_ = nc.Publish(natsclient.RoomEventsSubject(room.ID), eventBytes)
		return true

	case "shot_result_submitted":
		if !isParticipant || room.Status != StatusPlaying {
			return false
		}
		result, ok := parseShotResult(event.Payload)
		if !ok {
			return false
		}
		if result.MatchID != "" && result.MatchID != room.ID {
			return false
		}
		var nextSnapshot matchSnapshot
		committed := func() bool {
			h.matchStateMu.Lock()
			defer h.matchStateMu.Unlock()

			snapshot, ok := h.getCanonicalSnapshot(ctx, room.ID)
			if !ok || snapshot.ActiveShot == nil || snapshot.ActiveShot.ShooterUserID != userID {
				return false
			}

			var applied bool
			nextSnapshot, applied = applyShotResult(room, snapshot, result)
			if !applied {
				return false
			}
			return h.storeCanonicalSnapshot(ctx, room.ID, nextSnapshot)
		}()
		if !committed {
			return false
		}
		publishCanonicalSnapshot(nc, room.ID, nextSnapshot)

		if nextSnapshot.WinnerUserID == "" {
			return true
		}

		finishedRoom, err := h.repo.FinishRoom(ctx, room.ID)
		if err != nil {
			log.Printf("falha ao finalizar partida da sala %s: %v", room.ID, err)
			return false
		}
		xpAwards, err := h.xpAwarder.AwardMatchXP(ctx, nextSnapshot.WinnerUserID, roomParticipantIDs(finishedRoom))
		if err != nil {
			log.Printf("falha ao premiar XP da sala %s: %v", room.ID, err)
		}
		if len(xpAwards) == 0 {
			participantIDs := roomParticipantIDs(finishedRoom)
			xpAwards = make([]XPAward, 0, len(participantIDs))
			for _, pID := range participantIDs {
				delta := 25 // matchParticipationXP
				if pID == nextSnapshot.WinnerUserID {
					delta += 25 // matchWinnerBonusXP
				}
				xpAwards = append(xpAwards, XPAward{
					UserID:  pID,
					XPDelta: delta,
				})
			}
		}
		if finishedRoom.OpponentID != nil {
			h.rematches.ConfigureRoom(finishedRoom.ID, finishedRoom.CreatorID, *finishedRoom.OpponentID)
		}
		eventBytes, err := makeWSEvent("match_finished", "server", map[string]any{
			"room":           toRoomResponse(finishedRoom),
			"reason":         "normal",
			"winner_user_id": nextSnapshot.WinnerUserID,
			"scores":         nextSnapshot.Scores,
			"xp_awards":      xpAwards,
		})
		if err != nil {
			return false
		}
		_ = nc.Publish(natsclient.RoomEventsSubject(room.ID), eventBytes)
		h.broadcastPublicRooms(context.Background())
		return true

	case "game_state_sync":
		return false

	case "request_game_state":
		if room.Status != StatusPlaying {
			return false
		}
		h.matchStateMu.Lock()
		snapshot, ok := h.ensureCanonicalSnapshot(ctx, room)
		h.syncTurnTimer(room.ID, snapshot)
		h.matchStateMu.Unlock()
		if !ok {
			return false
		}
		publishCanonicalSnapshot(nc, room.ID, snapshot)
		if snapshot.ActiveShot != nil {
			eventBytes, err := makeWSEvent("shot_started", snapshot.ActiveShot.ShooterUserID, snapshot.ActiveShot)
			if err == nil {
				_ = nc.Publish(natsclient.RoomEventsSubject(room.ID), eventBytes)
			}
		}
		return true

	case "match_end":
		return false

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
		h.matchStateMu.Lock()
		h.clearCachedSnapshot(startedRoom.ID)
		snapshot := newInitialMatchSnapshot(startedRoom)
		stored := h.storeCanonicalSnapshot(ctx, startedRoom.ID, snapshot)
		h.matchStateMu.Unlock()
		if !stored {
			return false
		}
		eventBytes, err = makeWSEvent("match_start", userID, map[string]any{
			"room": toRoomResponse(startedRoom),
		})
		if err != nil {
			return false
		}
		_ = nc.Publish(natsclient.RoomEventsSubject(room.ID), eventBytes)
		publishCanonicalSnapshot(nc, startedRoom.ID, snapshot)
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
		h.clearCanonicalSnapshot(ctx, room.ID)
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
		return false
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
	if turnUserID == "" {
		turnUserID = userID
	}

	if !finite(payload.X) ||
		!finite(payload.Y) ||
		!finite(payload.Angle) ||
		!finite(payload.Power) ||
		payload.Power < 0 ||
		payload.Power > 100 ||
		payload.ShotSeq < 0 ||
		payload.ClientSeq < 0 ||
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
		h.clearCanonicalSnapshot(context.Background(), roomID)
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
		h.clearCanonicalSnapshot(context.Background(), roomID)
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
	ShotSeq     int    `json:"shot_seq"`
	TurnSeq     int    `json:"turn_seq"`
	UpdatedAtMS int64  `json:"updated_at_ms"`
	Status      string `json:"status"`
}

func snapshotVersion(raw json.RawMessage) (matchSnapshotVersion, bool) {
	var version matchSnapshotVersion
	if err := json.Unmarshal(raw, &version); err != nil {
		return version, false
	}
	if version.ShotSeq < 0 || version.TurnSeq < 0 || version.UpdatedAtMS <= 0 {
		return version, false
	}
	return version, true
}

func snapshotIsNewer(incoming, current matchSnapshotVersion) bool {
	if incoming.ShotSeq != current.ShotSeq {
		return incoming.ShotSeq > current.ShotSeq
	}
	if incoming.TurnSeq != current.TurnSeq {
		return incoming.TurnSeq > current.TurnSeq
	}
	if incoming.UpdatedAtMS != current.UpdatedAtMS {
		return incoming.UpdatedAtMS > current.UpdatedAtMS
	}
	return snapshotStatusRank(incoming.Status) > snapshotStatusRank(current.Status)
}

func snapshotStatusRank(status string) int {
	switch status {
	case matchStatusAiming:
		return 1
	case matchStatusMoving:
		return 3
	case matchStatusFinished:
		return 4
	default:
		return 0
	}
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
	h.cancelTurnTimer(roomID)
}

func (h *Handler) getCanonicalSnapshot(ctx context.Context, roomID string) (matchSnapshot, bool) {
	if cached, ok := h.getCachedSnapshot(roomID); ok {
		if snapshot, ok := parseCanonicalSnapshot(cached); ok {
			return snapshot, true
		}
	}

	if h.repo == nil {
		return matchSnapshot{}, false
	}

	persisted, err := h.repo.GetMatchSnapshot(ctx, roomID)
	if err != nil {
		return matchSnapshot{}, false
	}
	if !h.setCachedSnapshot(roomID, persisted) {
		return matchSnapshot{}, false
	}
	return parseCanonicalSnapshot(persisted)
}

func (h *Handler) ensureCanonicalSnapshot(ctx context.Context, room *Room) (matchSnapshot, bool) {
	if snapshot, ok := h.getCanonicalSnapshot(ctx, room.ID); ok {
		return snapshot, true
	}
	if room.Status != StatusPlaying {
		return matchSnapshot{}, false
	}
	snapshot := newInitialMatchSnapshot(room)
	if !h.storeCanonicalSnapshot(ctx, room.ID, snapshot) {
		return matchSnapshot{}, false
	}
	return snapshot, true
}

func parseCanonicalSnapshot(raw json.RawMessage) (matchSnapshot, bool) {
	var snapshot matchSnapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return snapshot, false
	}
	if snapshot.ShotSeq < 0 || snapshot.UpdatedAtMS <= 0 || snapshot.TurnUserID == "" || len(snapshot.Balls) == 0 {
		return snapshot, false
	}
	return normalizeTurnClock(snapshot, time.Now().UnixMilli()), true
}

func (h *Handler) storeCanonicalSnapshot(ctx context.Context, roomID string, snapshot matchSnapshot) bool {
	snapshot = normalizeTurnClock(snapshot, time.Now().UnixMilli())
	if snapshot.UpdatedAtMS <= 0 {
		snapshot.UpdatedAtMS = time.Now().UnixMilli()
	}
	if snapshot.AuditHash == "" {
		snapshot.AuditHash = auditHashForBalls(snapshot.Balls)
	}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		log.Printf("falha ao serializar snapshot canonico da sala %s: %v", roomID, err)
		return false
	}
	if !h.setCachedSnapshot(roomID, raw) {
		return false
	}
	if h.repo != nil {
		if err := h.repo.UpsertMatchSnapshot(ctx, roomID, raw); err != nil {
			log.Printf("falha ao persistir snapshot canonico da sala %s: %v", roomID, err)
			h.clearCachedSnapshot(roomID)
			return false
		}
	}
	h.syncTurnTimer(roomID, snapshot)
	return true
}

func (h *Handler) clearCanonicalSnapshot(ctx context.Context, roomID string) {
	h.clearCachedSnapshot(roomID)
	if h.repo == nil {
		return
	}
	if err := h.repo.DeleteMatchSnapshot(ctx, roomID); err != nil {
		log.Printf("falha ao remover snapshot canonico da sala %s: %v", roomID, err)
	}
}

func publishCanonicalSnapshot(nc *nats.Conn, roomID string, snapshot matchSnapshot) bool {
	eventBytes, err := makeWSEvent("game_state_sync", "server", snapshot)
	if err != nil {
		log.Printf("falha ao serializar snapshot canonico da sala %s: %v", roomID, err)
		return false
	}
	if err := nc.Publish(natsclient.RoomEventsSubject(roomID), eventBytes); err != nil {
		log.Printf("falha ao publicar snapshot canonico da sala %s: %v", roomID, err)
		return false
	}
	return true
}

func publishTurnTimeout(nc *nats.Conn, roomID string, payload turnTimeoutPayload) bool {
	eventBytes, err := makeWSEvent("turn_timeout", "server", payload)
	if err != nil {
		log.Printf("falha ao serializar timeout de turno da sala %s: %v", roomID, err)
		return false
	}
	if err := nc.Publish(natsclient.RoomEventsSubject(roomID), eventBytes); err != nil {
		log.Printf("falha ao publicar timeout de turno da sala %s: %v", roomID, err)
		return false
	}
	return true
}

func (h *Handler) cancelTurnTimer(roomID string) {
	h.turnTimersMu.Lock()
	defer h.turnTimersMu.Unlock()
	if scheduled, ok := h.turnTimers[roomID]; ok {
		scheduled.timer.Stop()
		delete(h.turnTimers, roomID)
	}
}

func (h *Handler) forgetTurnTimer(roomID string, turnSeq int) {
	h.turnTimersMu.Lock()
	defer h.turnTimersMu.Unlock()
	if scheduled, ok := h.turnTimers[roomID]; ok && scheduled.turnSeq == turnSeq {
		delete(h.turnTimers, roomID)
	}
}

func (h *Handler) syncTurnTimer(roomID string, snapshot matchSnapshot) {
	if snapshot.Status != matchStatusAiming || snapshot.WinnerUserID != "" || snapshot.TurnDeadlineAtMS <= 0 {
		h.cancelTurnTimer(roomID)
		return
	}

	turnSeq := snapshot.TurnSeq
	deadlineMS := snapshot.TurnDeadlineAtMS
	delay := time.Until(time.UnixMilli(deadlineMS))
	if delay < 0 {
		delay = 0
	}

	h.turnTimersMu.Lock()
	defer h.turnTimersMu.Unlock()
	if scheduled, ok := h.turnTimers[roomID]; ok {
		if scheduled.turnSeq == turnSeq && scheduled.deadlineMS == deadlineMS {
			return
		}
		scheduled.timer.Stop()
	}

	h.turnTimers[roomID] = &scheduledTurnTimer{
		turnSeq:    turnSeq,
		deadlineMS: deadlineMS,
		timer: time.AfterFunc(delay, func() {
			h.handleTurnTimer(context.Background(), roomID, turnSeq)
		}),
	}
}

func (h *Handler) rescheduleTurnTimerAfter(roomID string, snapshot matchSnapshot, delay time.Duration) {
	if snapshot.Status != matchStatusAiming || snapshot.WinnerUserID != "" || snapshot.TurnDeadlineAtMS <= 0 {
		h.cancelTurnTimer(roomID)
		return
	}

	turnSeq := snapshot.TurnSeq
	deadlineMS := snapshot.TurnDeadlineAtMS

	h.turnTimersMu.Lock()
	defer h.turnTimersMu.Unlock()
	if scheduled, ok := h.turnTimers[roomID]; ok {
		scheduled.timer.Stop()
	}

	h.turnTimers[roomID] = &scheduledTurnTimer{
		turnSeq:    turnSeq,
		deadlineMS: deadlineMS,
		timer: time.AfterFunc(delay, func() {
			h.handleTurnTimer(context.Background(), roomID, turnSeq)
		}),
	}
}

func (h *Handler) handleTurnTimer(ctx context.Context, roomID string, turnSeq int) {
	h.forgetTurnTimer(roomID, turnSeq)

	nc := natsclient.GetConn()
	if nc == nil {
		return
	}

	var nextSnapshot matchSnapshot
	var payload turnTimeoutPayload
	applied := func() bool {
		h.matchStateMu.Lock()
		defer h.matchStateMu.Unlock()

		snapshot, ok := h.getCanonicalSnapshot(ctx, roomID)
		if !ok || snapshot.TurnSeq != turnSeq {
			return false
		}

		now := time.Now().UnixMilli()
		// Allow a 250ms buffer for early triggers; otherwise reschedule
		if !isTurnExpired(snapshot, now+250) {
			remaining := time.UnixMilli(snapshot.TurnDeadlineAtMS).Sub(time.Now())
			if remaining < 100*time.Millisecond {
				remaining = 100 * time.Millisecond
			}
			h.rescheduleTurnTimerAfter(roomID, snapshot, remaining)
			return false
		}

		if h.repo == nil {
			return false
		}
		room, err := h.repo.GetRoomByID(ctx, roomID)
		if err != nil || room.Status != StatusPlaying {
			return false
		}
		if roomHasReconnectingPlayer(room) {
			h.rescheduleTurnTimerAfter(roomID, snapshot, time.Second)
			return false
		}

		next, timeoutPayload, ok := applyTurnTimeout(room, snapshot, now)
		if !ok || !h.storeCanonicalSnapshot(ctx, roomID, next) {
			return false
		}
		nextSnapshot = next
		payload = timeoutPayload
		return true
	}()
	if !applied {
		return
	}

	publishTurnTimeout(nc, roomID, payload)
	publishCanonicalSnapshot(nc, roomID, nextSnapshot)
}

func roomHasReconnectingPlayer(room *Room) bool {
	return room != nil && (room.CreatorDisconnectedAt != nil || room.OpponentDisconnectedAt != nil)
}
