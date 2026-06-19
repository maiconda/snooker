package httpx

const (
	ContextKeyUserID = "user_id"
	ContextKeyEmail  = "user_email"
	ContextKeyStatus = "user_status"
)

type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

type ErrorDetail struct {
	Code    string       `json:"code"`
	Message string       `json:"message"`
	Details []FieldError `json:"details,omitempty"`
}

type FieldError struct {
	Field string `json:"field"`
	Issue string `json:"issue"`
}

type MessageResponse struct {
	Message string `json:"message"`
}

const (
	ErrCodeConflict         = "CONFLICT"
	ErrCodeForbidden        = "FORBIDDEN"
	ErrCodeNotFound         = "NOT_FOUND"
	ErrCodeTokenExpired     = "TOKEN_EXPIRED"
	ErrCodeUnauthorized     = "UNAUTHORIZED"
	ErrCodeValidationFailed = "VALIDATION_FAILED"
	ErrCodeInternal         = "INTERNAL_ERROR"
)
