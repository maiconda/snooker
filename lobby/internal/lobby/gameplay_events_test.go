package lobby

import (
	"encoding/json"
	"math"
	"testing"
	"time"

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

	newer := json.RawMessage(`{"shot_seq":2,"turn_seq":3,"updated_at_ms":2000,"status":"aiming","scores":{"creator":20,"opponent":0}}`)
	olderSeq := json.RawMessage(`{"shot_seq":1,"turn_seq":3,"updated_at_ms":3000,"status":"aiming","scores":{"creator":0,"opponent":0}}`)
	olderTurn := json.RawMessage(`{"shot_seq":2,"turn_seq":2,"updated_at_ms":3000,"status":"aiming","scores":{"creator":0,"opponent":0}}`)
	olderTimestamp := json.RawMessage(`{"shot_seq":2,"turn_seq":3,"updated_at_ms":1000,"status":"aiming","scores":{"creator":0,"opponent":0}}`)
	fresher := json.RawMessage(`{"shot_seq":2,"turn_seq":3,"updated_at_ms":3000,"status":"aiming","scores":{"creator":30,"opponent":0}}`)

	assert.True(t, handler.setCachedSnapshot("room-1", newer))
	assert.False(t, handler.setCachedSnapshot("room-1", olderSeq))
	assert.False(t, handler.setCachedSnapshot("room-1", olderTurn))
	assert.False(t, handler.setCachedSnapshot("room-1", olderTimestamp))
	assert.True(t, handler.setCachedSnapshot("room-1", fresher))

	cached, ok := handler.getCachedSnapshot("room-1")
	assert.True(t, ok)
	assert.JSONEq(t, string(fresher), string(cached))
}

func TestCachedSnapshotAcceptsTurnAdvanceInSameMillisecond(t *testing.T) {
	handler := NewHandler(nil)

	moving := json.RawMessage(`{"shot_seq":1,"turn_seq":1,"updated_at_ms":2000,"status":"moving","scores":{"creator":0,"opponent":0}}`)
	nextTurn := json.RawMessage(`{"shot_seq":1,"turn_seq":2,"updated_at_ms":2000,"status":"aiming","scores":{"creator":10,"opponent":0}}`)

	assert.True(t, handler.setCachedSnapshot("room-1", moving))
	assert.True(t, handler.setCachedSnapshot("room-1", nextTurn))

	cached, ok := handler.getCachedSnapshot("room-1")
	assert.True(t, ok)
	assert.JSONEq(t, string(nextTurn), string(cached))
}

func TestCachedSnapshotAcceptsFinishedStatusInSameMillisecond(t *testing.T) {
	handler := NewHandler(nil)

	moving := json.RawMessage(`{"shot_seq":1,"turn_seq":1,"updated_at_ms":2000,"status":"moving","scores":{"creator":0,"opponent":0}}`)
	finished := json.RawMessage(`{"shot_seq":1,"turn_seq":1,"updated_at_ms":2000,"status":"finished","winner_user_id":"opponent-1","scores":{"creator":0,"opponent":0}}`)

	assert.True(t, handler.setCachedSnapshot("room-1", moving))
	assert.True(t, handler.setCachedSnapshot("room-1", finished))

	cached, ok := handler.getCachedSnapshot("room-1")
	assert.True(t, ok)
	assert.JSONEq(t, string(finished), string(cached))
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
	assert.Zero(t, next.TurnStartedAtMS)
	assert.Zero(t, next.TurnDeadlineAtMS)
	assert.Equal(t, matchStatusAiming, next.Status)
	assert.Nil(t, next.ActiveShot)
	assert.NotEmpty(t, next.AuditHash)
}

func TestServerMakesShooterLoseWhenBlackBallIsSunk(t *testing.T) {
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
		if resultBalls[i].ID == 8 {
			resultBalls[i].Sunk = true
		}
	}

	next, ok := applyShotResult(room, current, shotResultPayload{
		ShotSeq:       1,
		ShooterUserID: "creator-1",
		Balls:         resultBalls,
	})

	assert.True(t, ok)
	assert.Equal(t, "opponent-1", next.WinnerUserID)
	assert.Equal(t, matchStatusFinished, next.Status)
	assert.Equal(t, 0, next.Scores.Creator)
	assert.Equal(t, 0, next.Scores.Opponent)
}

func TestServerAcceptsShotResultWithPreviousClientShotSeq(t *testing.T) {
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

	next, ok := applyShotResult(room, current, shotResultPayload{
		ShotSeq:       0,
		ShooterUserID: "creator-1",
		Balls:         current.Balls,
	})

	assert.True(t, ok)
	assert.Equal(t, 1, next.ShotSeq)
	assert.Equal(t, "opponent-1", next.TurnUserID)
	assert.Equal(t, matchStatusAiming, next.Status)
}

func TestInitialSnapshotWaitsForTurnReady(t *testing.T) {
	room := &Room{
		ID:        "room-1",
		CreatorID: "creator-1",
	}

	snapshot := newInitialMatchSnapshot(room)

	assert.Equal(t, 1, snapshot.TurnSeq)
	assert.Equal(t, "creator-1", snapshot.TurnUserID)
	assert.Equal(t, matchStatusAiming, snapshot.Status)
	assert.Zero(t, snapshot.TurnStartedAtMS)
	assert.Zero(t, snapshot.TurnDeadlineAtMS)
}

func TestStartTurnClockArmsTimedTurn(t *testing.T) {
	room := &Room{
		ID:        "room-1",
		CreatorID: "creator-1",
	}
	snapshot := newInitialMatchSnapshot(room)

	ready := startTurnClock(snapshot, snapshot.UpdatedAtMS)

	assert.Equal(t, 1, ready.TurnSeq)
	assert.Equal(t, "creator-1", ready.TurnUserID)
	assert.Equal(t, matchStatusAiming, ready.Status)
	assert.Greater(t, ready.UpdatedAtMS, snapshot.UpdatedAtMS)
	assert.Equal(t, matchTurnDurationMS, ready.TurnDeadlineAtMS-ready.TurnStartedAtMS)
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
	current.TurnDeadlineAtMS = current.TurnStartedAtMS + matchTurnDurationMS
	current.UpdatedAtMS = 1_000

	next, payload, ok := applyTurnTimeout(room, current, current.TurnDeadlineAtMS)

	assert.True(t, ok)
	assert.Equal(t, "opponent-1", next.TurnUserID)
	assert.Equal(t, 4, next.TurnSeq)
	assert.Equal(t, matchStatusAiming, next.Status)
	assert.Equal(t, current.ShotSeq, next.ShotSeq)
	assert.Equal(t, current.Scores, next.Scores)
	assert.Equal(t, current.Balls, next.Balls)
	assert.Equal(t, current.Pockets, next.Pockets)
	assert.Zero(t, next.TurnStartedAtMS)
	assert.Zero(t, next.TurnDeadlineAtMS)
	assert.Equal(t, "creator-1", payload.TimedOutUserID)
	assert.Equal(t, "opponent-1", payload.NextTurnUserID)
	assert.Equal(t, 4, payload.NextTurnSeq)
	assert.Zero(t, payload.TurnStartedAtMS)
	assert.Zero(t, payload.TurnDeadlineAtMS)
}

func TestTurnTimerReschedulesBeforeDeadline(t *testing.T) {
	room := &Room{
		ID:        "room-1",
		CreatorID: "creator-1",
	}
	current := newInitialMatchSnapshot(room)
	current.TurnStartedAtMS = 1_000
	current.TurnDeadlineAtMS = 2_000

	now, remaining, expired := turnTimerExpiration(current, time.UnixMilli(1_999))
	_, _, applied := applyTurnTimeout(room, current, now)

	assert.False(t, expired)
	assert.False(t, applied)
	assert.Equal(t, int64(1_999), now)
	assert.Equal(t, 100*time.Millisecond, remaining)
}

func TestTurnTimerAppliesAtDeadline(t *testing.T) {
	opponentID := "opponent-1"
	room := &Room{
		ID:         "room-1",
		CreatorID:  "creator-1",
		OpponentID: &opponentID,
	}
	current := newInitialMatchSnapshot(room)
	current.TurnStartedAtMS = 1_000
	current.TurnDeadlineAtMS = 2_000

	now, remaining, expired := turnTimerExpiration(current, time.UnixMilli(2_000))
	next, _, applied := applyTurnTimeout(room, current, now)

	assert.True(t, expired)
	assert.True(t, applied)
	assert.Zero(t, remaining)
	assert.Equal(t, "opponent-1", next.TurnUserID)
}

func TestApplyShotResultTimeoutRecoversStuckMovingShot(t *testing.T) {
	opponentID := "opponent-1"
	room := &Room{
		ID:         "room-1",
		CreatorID:  "creator-1",
		OpponentID: &opponentID,
	}
	current := newInitialMatchSnapshot(room)
	current.TurnSeq = 3
	current.ShotSeq = 2
	current.Status = matchStatusMoving
	current.TurnUserID = "creator-1"
	current.UpdatedAtMS = 1_000
	current.Balls[1].VX = 2
	current.Balls[1].VY = -1
	current.Balls[1].SpinX = 40
	current.Balls[1].SpinY = -25
	current.Balls[1].Sinking = true
	current.Balls[1].SinkProgress = 0.5
	current.ActiveShot = &activeShotState{
		ShotSeq:           2,
		ShooterUserID:     "creator-1",
		Angle:             0.4,
		Power:             50,
		ServerStartedAtMS: 900,
	}

	next, payload, ok := applyShotResultTimeout(room, current, 2_000)

	assert.True(t, ok)
	assert.Equal(t, "opponent-1", next.TurnUserID)
	assert.Equal(t, 4, next.TurnSeq)
	assert.Equal(t, 2, next.ShotSeq)
	assert.Equal(t, matchStatusAiming, next.Status)
	assert.Nil(t, next.ActiveShot)
	assert.Zero(t, next.TurnStartedAtMS)
	assert.Zero(t, next.TurnDeadlineAtMS)
	assert.Zero(t, next.Balls[1].VX)
	assert.Zero(t, next.Balls[1].VY)
	assert.Zero(t, next.Balls[1].SpinX)
	assert.Zero(t, next.Balls[1].SpinY)
	assert.False(t, next.Balls[1].Sinking)
	assert.Zero(t, next.Balls[1].SinkProgress)
	assert.Equal(t, createServerPockets(room.ID, current.ShotSeq, next.Balls), next.Pockets)
	assert.Equal(t, auditHashForBalls(next.Balls), next.AuditHash)
	assert.Greater(t, next.UpdatedAtMS, current.UpdatedAtMS)
	assert.Equal(t, "creator-1", payload.ShooterUserID)
	assert.Equal(t, 2, payload.ShotSeq)
	assert.Equal(t, "opponent-1", payload.NextTurnUserID)
	assert.Equal(t, 4, payload.NextTurnSeq)
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

func TestServerNormalizesShotResultWithMismatchedAuditHash(t *testing.T) {
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

	next, ok := applyShotResult(room, current, shotResultPayload{
		ShotSeq:       1,
		ShooterUserID: "creator-1",
		Balls:         current.Balls,
		AuditHash:     "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	})

	assert.True(t, ok)
	assert.NotEqual(t, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", next.AuditHash)
	assert.Equal(t, auditHashForBalls(next.Balls), next.AuditHash)
	assert.Equal(t, matchStatusAiming, next.Status)
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
