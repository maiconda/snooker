package profile

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

type AuthClient struct {
	baseURL        string
	internalAPIKey string
	httpClient     *http.Client
}

func NewAuthClient(baseURL string, internalAPIKey string) *AuthClient {
	return &AuthClient{
		baseURL:        strings.TrimRight(baseURL, "/"),
		internalAPIKey: internalAPIKey,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

type activateUserResponse struct {
	AccessToken string `json:"access_token"`
	Status      string `json:"status"`
}

func (c *AuthClient) ActivateUser(ctx context.Context, userID uuid.UUID) (string, string, error) {
	endpoint := fmt.Sprintf("%s/api/v1/internal/users/%s/activate", c.baseURL, userID.String())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader([]byte("{}")))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-API-Key", c.internalAPIKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("auth retornou status %d: %s", resp.StatusCode, string(body))
	}

	var parsed activateUserResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", "", err
	}
	if parsed.AccessToken == "" || parsed.Status == "" {
		return "", "", fmt.Errorf("resposta de ativacao invalida")
	}
	return parsed.AccessToken, parsed.Status, nil
}
