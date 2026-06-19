package lobby

import (
	"time"
)

type RoomStatus string

const (
	StatusWaiting  RoomStatus = "waiting"
	StatusPlaying  RoomStatus = "playing"
	StatusFinished RoomStatus = "finished"
	StatusExpired  RoomStatus = "expired"
)

type Room struct {
	ID         string     `json:"id"`
	Code       *string    `json:"code,omitempty"`
	CreatorID  string     `json:"creator_id"`
	OpponentID *string    `json:"opponent_id,omitempty"`
	Status     RoomStatus `json:"status"`
	IsPrivate  bool       `json:"is_private"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  time.Time  `json:"expires_at"`
}

type CreateRoomRequest struct {
	IsPrivate bool `json:"is_private"`
}

type JoinRoomRequest struct {
	Code string `json:"code" binding:"required"`
}

type RoomResponse struct {
	ID         string     `json:"id"`
	Code       *string    `json:"code,omitempty"`
	CreatorID  string     `json:"creator_id"`
	OpponentID *string    `json:"opponent_id,omitempty"`
	Status     RoomStatus `json:"status"`
	IsPrivate  bool       `json:"is_private"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  time.Time  `json:"expires_at"`
}
