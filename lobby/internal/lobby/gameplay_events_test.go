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
		{"shot_seq": 1, "x": 0, "y": 0, "angle": 0, "power": 50, "client_seq": -1},
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
	assert.Equal(t, "user-1", payload["turn_user_id"])
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

func TestServerAppliesShotResultRulesFromCanonicalBallIDs(t *testing.T) {
	opponentID := "opponent-1"
	room := &Room{
		ID:         "room-1",
		CreatorID:  "creator-1",
		OpponentID: &opponentID,
	}
	current := newInitialMatchSnapshot(room)
	current.ShotSeq = 1
	current.Status = matchStatusMoving
	current.ActiveShot = &activeShotState{
		ShotSeq:       1,
		ShooterUserID: "creator-1",
		Angle:         0.4,
		Power:         50,
	}

	resultBalls := append([]matchBall(nil), current.Balls...)
	for i := range resultBalls {
		if resultBalls[i].ID == 1 {
			resultBalls[i].Sunk = true
			resultBalls[i].Owner = "opponent"
			resultBalls[i].Points = 999
		}
	}

	next, ok := applyShotResult(room, current, shotResultPayload{
		ShotSeq:       1,
		ShooterUserID: "creator-1",
		Balls:         resultBalls,
	})

	assert.True(t, ok)
	assert.Equal(t, 10, next.Scores.Creator)
	assert.Equal(t, 0, next.Scores.Opponent)
	assert.Equal(t, "creator-1", next.TurnUserID)
	assert.Equal(t, current.TurnSeq+1, next.TurnSeq)
	assert.Equal(t, int64(10_000), next.TurnDeadlineAtMS-next.TurnStartedAtMS)
	assert.Equal(t, matchStatusAiming, next.Status)
	assert.Nil(t, next.ActiveShot)
	assert.NotEmpty(t, next.AuditHash)
}

func TestInitialSnapshotStartsTimedTurn(t *testing.T) {
	room := &Room{
		ID:        "room-1",
		CreatorID: "creator-1",
	}

	snapshot := newInitialMatchSnapshot(room)

	assert.Equal(t, 1, snapshot.TurnSeq)
	assert.Equal(t, "creator-1", snapshot.TurnUserID)
	assert.Equal(t, matchStatusAiming, snapshot.Status)
	assert.Equal(t, int64(10_000), snapshot.TurnDeadlineAtMS-snapshot.TurnStartedAtMS)
}

func TestApplyTurnTimeoutPassesTurnWithoutChangingTable(t *testing.T) {
	opponentID := "opponent-1"
	room := &Room{
		ID:         "room-1",
		CreatorID:  "creator-1",
		OpponentID: &opponentID,
	}
	current := newInitialMatchSnapshot(room)
	current.TurnSeq = 3
	current.TurnStartedAtMS = 1_000
	current.TurnDeadlineAtMS = 11_000
	current.UpdatedAtMS = 1_000

	next, payload, ok := applyTurnTimeout(room, current, 11_000)

	assert.True(t, ok)
	assert.Equal(t, "opponent-1", next.TurnUserID)
	assert.Equal(t, 4, next.TurnSeq)
	assert.Equal(t, matchStatusAiming, next.Status)
	assert.Equal(t, current.ShotSeq, next.ShotSeq)
	assert.Equal(t, current.Scores, next.Scores)
	assert.Equal(t, current.Balls, next.Balls)
	assert.Equal(t, current.Pockets, next.Pockets)
	assert.Equal(t, "creator-1", payload.TimedOutUserID)
	assert.Equal(t, "opponent-1", payload.NextTurnUserID)
	assert.Equal(t, 4, payload.NextTurnSeq)
	assert.Equal(t, int64(10_000), payload.TurnDeadlineAtMS-payload.TurnStartedAtMS)
}

func TestServerRejectsShotResultFromWrongShooter(t *testing.T) {
	opponentID := "opponent-1"
	room := &Room{
		ID:         "room-1",
		CreatorID:  "creator-1",
		OpponentID: &opponentID,
	}
	current := newInitialMatchSnapshot(room)
	current.ShotSeq = 1
	current.Status = matchStatusMoving
	current.ActiveShot = &activeShotState{
		ShotSeq:       1,
		ShooterUserID: "creator-1",
		Angle:         0.4,
		Power:         50,
	}

	_, ok := applyShotResult(room, current, shotResultPayload{
		ShotSeq:       1,
		ShooterUserID: "opponent-1",
		Balls:         current.Balls,
	})

	assert.False(t, ok)
}

func TestServerRejectsShotResultThatUnsinksCommittedBalls(t *testing.T) {
	opponentID := "opponent-1"
	room := &Room{
		ID:         "room-1",
		CreatorID:  "creator-1",
		OpponentID: &opponentID,
	}
	current := newInitialMatchSnapshot(room)
	for i := range current.Balls {
		if current.Balls[i].ID == 1 {
			current.Balls[i].Sunk = true
		}
	}
	current.ShotSeq = 2
	current.Status = matchStatusMoving
	current.ActiveShot = &activeShotState{
		ShotSeq:       2,
		ShooterUserID: "opponent-1",
		Angle:         0.4,
		Power:         50,
	}

	resultBalls := append([]matchBall(nil), current.Balls...)
	for i := range resultBalls {
		if resultBalls[i].ID == 1 {
			resultBalls[i].Sunk = false
		}
	}

	_, ok := applyShotResult(room, current, shotResultPayload{
		ShotSeq:       2,
		ShooterUserID: "opponent-1",
		Balls:         resultBalls,
	})

	assert.False(t, ok)
}

func TestServerRejectsShotResultWithMismatchedAuditHash(t *testing.T) {
	opponentID := "opponent-1"
	room := &Room{
		ID:         "room-1",
		CreatorID:  "creator-1",
		OpponentID: &opponentID,
	}
	current := newInitialMatchSnapshot(room)
	current.ShotSeq = 1
	current.Status = matchStatusMoving
	current.ActiveShot = &activeShotState{
		ShotSeq:       1,
		ShooterUserID: "creator-1",
		Angle:         0.4,
		Power:         50,
	}

	_, ok := applyShotResult(room, current, shotResultPayload{
		ShotSeq:       1,
		ShooterUserID: "creator-1",
		Balls:         current.Balls,
		AuditHash:     "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	})

	assert.False(t, ok)
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
