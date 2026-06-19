package lobby

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/nats-io/nats.go"
	"snooker/lobby/internal/httpx"
	natsclient "snooker/lobby/internal/nats"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Permitir todos os origins para conexões locais / CORS
	},
}

// WSEvent define o envelope padrão para mensagens em tempo real
type WSEvent struct {
	Type     string          `json:"type"`
	SenderID string          `json:"sender_id,omitempty"`
	Payload  json.RawMessage `json:"payload,omitempty"`
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

	// Permissão de acesso: participantes (criador ou oponente) sempre entram. 
	// Para salas privadas, não participantes são rejeitados.
	isParticipant := room.CreatorID == userID || (room.OpponentID != nil && *room.OpponentID == userID)
	if room.IsPrivate && !isParticipant {
		c.JSON(http.StatusForbidden, httpx.ErrorResponse{
			Error: httpx.ErrorDetail{Code: httpx.ErrCodeForbidden, Message: "Acesso proibido a esta sala privada"},
		})
		return
	}

	nc := natsclient.GetConn()
	if nc == nil {
		log.Printf("conexao WebSocket recusada: NATS nao esta conectado")
		c.JSON(http.StatusServiceUnavailable, httpx.ErrorResponse{
			Error: httpx.ErrorDetail{Code: httpx.ErrCodeInternal, Message: "Servidor de mensageria temporariamente indisponivel"},
		})
		return
	}

	// Upgrade do protocolo HTTP para WebSocket
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("falha ao realizar upgrade de WebSocket: %v", err)
		return
	}
	defer conn.Close()

	subject := "rooms." + room.ID

	// done sinaliza o encerramento do ciclo de vida das goroutines
	done := make(chan struct{})
	
	// writeChan envia mensagens recebidas do NATS para o cliente via WS
	writeChan := make(chan []byte, 256)

	// Subscrever no tópico NATS da sala
	sub, err := nc.Subscribe(subject, func(msg *nats.Msg) {
		select {
		case writeChan <- msg.Data:
		case <-done:
		}
	})
	if err != nil {
		log.Printf("falha ao subscrever no canal NATS: %v", err)
		return
	}
	defer sub.Unsubscribe()

	// Goroutine de escrita (consome do NATS e envia para o WebSocket)
	go func() {
		defer close(done)
		for {
			select {
			case msgData, ok := <-writeChan:
				if !ok {
					return
				}
				if err := conn.WriteMessage(websocket.TextMessage, msgData); err != nil {
					log.Printf("erro ao enviar mensagem WebSocket: %v", err)
					return
				}
			case <-done:
				return
			}
		}
	}()

	// Evento de broadcast inicial: player_joined
	joinedPayload, _ := json.Marshal(map[string]string{"user_id": userID})
	joinedEvent := WSEvent{
		Type:     "player_joined",
		SenderID: userID,
		Payload:  joinedPayload,
	}
	joinedBytes, _ := json.Marshal(joinedEvent)
	_ = nc.Publish(subject, joinedBytes)

	// Loop de leitura (consome do WebSocket e publica no NATS)
	for {
		_, msgBytes, err := conn.ReadMessage()
		if err != nil {
			break // Desconexão
		}

		var event WSEvent
		if err := json.Unmarshal(msgBytes, &event); err != nil {
			log.Printf("evento WS malformado recebido: %v", err)
			continue
		}

		// Garante que o sender_id é o do usuário autenticado no WebSocket
		event.SenderID = userID
		enrichedBytes, err := json.Marshal(event)
		if err != nil {
			continue
		}

		// Publica no NATS para repassar a todos os ouvintes da sala
		_ = nc.Publish(subject, enrichedBytes)
	}

	// Evento de broadcast final: player_left
	leftPayload, _ := json.Marshal(map[string]string{"user_id": userID})
	leftEvent := WSEvent{
		Type:     "player_left",
		SenderID: userID,
		Payload:  leftPayload,
	}
	leftBytes, _ := json.Marshal(leftEvent)
	_ = nc.Publish(subject, leftBytes)

	// Se quem saiu foi o dono (criador) da sala, encerramos a sala no banco
	// e enviamos o evento room_closed
	if userID == room.CreatorID {
		log.Printf("dono da sala %s desconectou. encerrando a sala no banco de dados.", room.ID)
		_ = h.repo.CloseRoom(context.Background(), room.ID)

		closedEvent := WSEvent{
			Type:     "room_closed",
			SenderID: userID,
		}
		closedBytes, _ := json.Marshal(closedEvent)
		_ = nc.Publish(subject, closedBytes)
	}

	// Limpar recursos e aguardar término
	close(writeChan)
	<-done
}
