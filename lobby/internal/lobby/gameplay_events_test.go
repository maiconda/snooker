package lobby

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCueStatePayloadAcceptsFiniteValues(t *testing.T) {
	raw, err := json.Marshal(map[string]any{
		"match_id":     "match-1",
		"shot_seq":     2,
		"turn_user_id": "user-1",
		"x":            -0.25,
		"y":            0.4,
		"angle":        1.2,
		"power":        72,
		"is_aiming":    true,
		"client_seq":   12,
	})
	assert.NoError(t, err)

	payload, ok := cueStatePayload(raw, "user-1")

	assert.True(t, ok)
	assert.Equal(t, "match-1", payload["match_id"])
	assert.Equal(t, 2, payload["shot_seq"])
	assert.Equal(t, "user-1", payload["turn_user_id"])
	assert.Equal(t, float64(72), payload["power"])
	assert.NotZero(t, payload["server_received_at_ms"])
}

func TestCueStatePayloadRejectsInvalidValues(t *testing.T) {
	tests := []map[string]any{
		{"shot_seq": -1, "x": 0, "y": 0, "angle": 0, "power": 50},
		{"shot_seq": 1, "x": 0, "y": 0, "angle": 0, "power": -1},
		{"shot_seq": 1, "x": 0, "y": 0, "angle": 0, "power": 101},
	}

	for _, test := range tests {
		raw, err := json.Marshal(test)
		assert.NoError(t, err)

		_, ok := cueStatePayload(raw, "user-1")

		assert.False(t, ok)
	}
}

func TestCueStatePayloadNormalizesAngles(t *testing.T) {
	raw, err := json.Marshal(map[string]any{
		"shot_seq": 1,
		"x":        0,
		"y":        0,
		"angle":    math.Pi * 3,
		"power":    50,
	})
	assert.NoError(t, err)

	payload, ok := cueStatePayload(raw, "user-1")

	assert.True(t, ok)
	assert.InDelta(t, math.Pi, payload["angle"], 0.000001)
}

func TestCueStatePayloadRejectsTurnSpoofing(t *testing.T) {
	raw, err := json.Marshal(map[string]any{
		"shot_seq":     1,
		"turn_user_id": "user-2",
		"x":            0,
		"y":            0,
		"angle":        0,
		"power":        50,
	})
	assert.NoError(t, err)

	_, ok := cueStatePayload(raw, "user-1")

	assert.False(t, ok)
}

func TestCachedSnapshotRejectsOlderVersions(t *testing.T) {
	handler := NewHandler(nil)

	newer := json.RawMessage(`{"shot_seq":2,"updated_at_ms":2000,"scores":{"creator":20,"opponent":0}}`)
	olderSeq := json.RawMessage(`{"shot_seq":1,"updated_at_ms":3000,"scores":{"creator":0,"opponent":0}}`)
	olderTimestamp := json.RawMessage(`{"shot_seq":2,"updated_at_ms":1000,"scores":{"creator":0,"opponent":0}}`)
	fresher := json.RawMessage(`{"shot_seq":2,"updated_at_ms":3000,"scores":{"creator":30,"opponent":0}}`)

	assert.True(t, handler.setCachedSnapshot("room-1", newer))
	assert.False(t, handler.setCachedSnapshot("room-1", olderSeq))
	assert.False(t, handler.setCachedSnapshot("room-1", olderTimestamp))
	assert.True(t, handler.setCachedSnapshot("room-1", fresher))

	cached, ok := handler.getCachedSnapshot("room-1")
	assert.True(t, ok)
	assert.JSONEq(t, string(fresher), string(cached))
}

func TestOnlyCreatorPublishesAuthoritativeGameState(t *testing.T) {
	opponentID := "opponent-1"
	room := &Room{
		CreatorID:  "creator-1",
		OpponentID: &opponentID,
	}

	assert.True(t, isAuthoritativeGameStateSender(room, "creator-1"))
	assert.False(t, isAuthoritativeGameStateSender(room, "opponent-1"))
	assert.False(t, isAuthoritativeGameStateSender(room, "spectator-1"))
	assert.False(t, isAuthoritativeGameStateSender(nil, "creator-1"))
}

func TestRematchStoreRequiresBothConfiguredPlayers(t *testing.T) {
	store := newRematchStore()
	store.ConfigureRoom("room-1", "owner-1", "opponent-1")

	requested, allReady := store.Request("room-1", "owner-1")
	assert.False(t, allReady)
	assert.Equal(t, []string{"owner-1"}, requested)

	requested, allReady = store.Request("room-1", "opponent-1")
	assert.True(t, allReady)
	assert.Equal(t, []string{"opponent-1", "owner-1"}, requested)

	store.ClearRoom("room-1")
	_, allReady = store.Request("room-1", "owner-1")
	assert.False(t, allReady)
}
