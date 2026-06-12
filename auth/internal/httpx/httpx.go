package httpx

// Context keys compartilhadas entre middlewares HTTP e handlers.
const (
	ContextKeyUserID = "user_id"
	ContextKeyEmail  = "user_email"
	ContextKeyStatus = "user_status"
)

// ErrorResponse e tipos relacionados padronizam erros HTTP da API.
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
	ErrCodeInvalidCredentials = "INVALID_CREDENTIALS"
	ErrCodeEmailAlreadyExists = "EMAIL_ALREADY_EXISTS"
	ErrCodeOnboardingPending  = "ONBOARDING_PENDING"
	ErrCodeValidationFailed   = "VALIDATION_FAILED"
	ErrCodeTokenExpired       = "TOKEN_EXPIRED"
	ErrCodeUnauthorized       = "UNAUTHORIZED"
	ErrCodeInternal           = "INTERNAL_ERROR"
	ErrCodeTooManyRequests    = "TOO_MANY_REQUESTS"
)

func ValidationDetails(err error) []FieldError {
	return []FieldError{
		{
			Field: "body",
			Issue: err.Error(),
		},
	}
}
