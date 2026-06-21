package profile

import (
	"time"

	"github.com/google/uuid"
)

const (
	StatusOnboardingPending = "onboarding_pending"
	StatusActive            = "active"
	StatusBlocked           = "blocked"

	ContentTypeJPEG = "image/jpeg"
	ContentTypePNG  = "image/png"
	ContentTypeWEBP = "image/webp"
)

type Profile struct {
	UserID             uuid.UUID `json:"user_id" db:"user_id"`
	Nickname           string    `json:"nickname" db:"nickname"`
	NicknameNormalized string    `json:"-" db:"nickname_normalized"`
	Bio                string    `json:"bio" db:"bio"`
	PhotoObjectKey     string    `json:"-" db:"photo_object_key"`
	PhotoURL           string    `json:"photo_url" db:"photo_url"`
	XP                 int       `json:"xp" db:"xp"`
	CreatedAt          time.Time `json:"created_at" db:"created_at"`
	UpdatedAt          time.Time `json:"updated_at" db:"updated_at"`
}

type PhotoUploadSession struct {
	ID           uuid.UUID  `db:"id"`
	UserID       uuid.UUID  `db:"user_id"`
	ObjectKey    string     `db:"object_key"`
	ContentType  string     `db:"content_type"`
	MaxSizeBytes int64      `db:"max_size_bytes"`
	ExpiresAt    time.Time  `db:"expires_at"`
	ConsumedAt   *time.Time `db:"consumed_at"`
	CreatedAt    time.Time  `db:"created_at"`
}

type CompleteProfileRequest struct {
	Nickname      string     `json:"nickname"`
	Bio           string     `json:"bio"`
	PhotoUploadID *uuid.UUID `json:"photo_upload_id"`
}

type UpdateProfileRequest struct {
	Nickname      *string    `json:"nickname"`
	Bio           *string    `json:"bio"`
	PhotoUploadID *uuid.UUID `json:"photo_upload_id"`
}

type PhotoUploadURLRequest struct {
	ContentType string `json:"content_type"`
	FileSize    int64  `json:"file_size"`
}

type PhotoUploadURLResponse struct {
	UploadID  uuid.UUID `json:"upload_id"`
	UploadURL string    `json:"upload_url"`
	ObjectKey string    `json:"object_key"`
	ExpiresAt time.Time `json:"expires_at"`
	PublicURL string    `json:"public_url"`
	MaxSize   int64     `json:"max_size"`
}

type CompleteProfileResponse struct {
	Profile     *Profile `json:"profile"`
	AccessToken string   `json:"access_token"`
	Status      string   `json:"status"`
}

type MatchXPRequest struct {
	WinnerUserID       uuid.UUID   `json:"winner_user_id"`
	ParticipantUserIDs []uuid.UUID `json:"participant_user_ids"`
}

type XPAward struct {
	UserID  uuid.UUID `json:"user_id"`
	XPDelta int       `json:"xp_delta"`
	TotalXP int       `json:"total_xp"`
}

type MatchXPResponse struct {
	Awards []XPAward `json:"awards"`
}
