package lobby

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProfileXPClientAwardsMatchXP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/internal/profiles/xp/match", r.URL.Path)
		assert.Equal(t, "secret", r.Header.Get("X-Internal-API-Key"))

		var req struct {
			WinnerUserID       string   `json:"winner_user_id"`
			ParticipantUserIDs []string `json:"participant_user_ids"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, "winner-1", req.WinnerUserID)
		assert.Equal(t, []string{"winner-1", "opponent-1"}, req.ParticipantUserIDs)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"awards":[{"user_id":"winner-1","xp_delta":50,"total_xp":150}]}`))
	}))
	defer server.Close()

	client := NewProfileXPClient(server.URL, "secret")

	awards, err := client.AwardMatchXP(context.Background(), "winner-1", []string{"winner-1", "opponent-1"})

	require.NoError(t, err)
	assert.Equal(t, []XPAward{{UserID: "winner-1", XPDelta: 50, TotalXP: 150}}, awards)
}
