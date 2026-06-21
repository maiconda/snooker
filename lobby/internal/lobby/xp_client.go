package lobby

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type XPAward struct {
	UserID  string `json:"user_id"`
	XPDelta int    `json:"xp_delta"`
	TotalXP int    `json:"total_xp,omitempty"`
}

type MatchXPAwarder interface {
	AwardMatchXP(ctx context.Context, winnerUserID string, participantUserIDs []string) ([]XPAward, error)
}

type noopMatchXPAwarder struct{}

func (noopMatchXPAwarder) AwardMatchXP(ctx context.Context, winnerUserID string, participantUserIDs []string) ([]XPAward, error) {
	return nil, nil
}

type profileXPClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

func NewProfileXPClient(baseURL string, apiKey string) MatchXPAwarder {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	apiKey = strings.TrimSpace(apiKey)
	if baseURL == "" || apiKey == "" {
		return noopMatchXPAwarder{}
	}
	return &profileXPClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 3 * time.Second,
		},
	}
}

func (c *profileXPClient) AwardMatchXP(ctx context.Context, winnerUserID string, participantUserIDs []string) ([]XPAward, error) {
	requestBody := map[string]any{
		"winner_user_id":       winnerUserID,
		"participant_user_ids": participantUserIDs,
	}
	body, err := json.Marshal(requestBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/internal/profiles/xp/match", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-API-Key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("profile XP service returned status %d", resp.StatusCode)
	}

	var response struct {
		Awards []XPAward `json:"awards"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}

	return response.Awards, nil
}
