package lobby

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"math"
	"math/rand"
	"sort"
	"strconv"
	"time"
)

const (
	matchStatusAiming   = "aiming"
	matchStatusMoving   = "moving"
	matchStatusFinished = "finished"

	matchTableRadius  = 0.95
	matchBallRadius   = 0.035
	matchPocketRadius = 0.055

	randomPocketCount          = 3
	randomPocketAttempts       = 900
	randomPocketPlacementLimit = matchTableRadius - matchPocketRadius*1.7
	randomPocketMinDistance    = matchPocketRadius * 3.4
	randomPocketBallClearance  = matchPocketRadius + matchBallRadius*1.9

	matchTurnDurationMS = int64(20_000)
)

type matchScoreboard struct {
	Creator  int `json:"creator"`
	Opponent int `json:"opponent"`
}

type matchBall struct {
	ID           int     `json:"id"`
	X            float64 `json:"x"`
	Y            float64 `json:"y"`
	VX           float64 `json:"vx"`
	VY           float64 `json:"vy"`
	SpinX        float64 `json:"spinX"`
	SpinY        float64 `json:"spinY"`
	Radius       float64 `json:"radius"`
	IsWhite      bool    `json:"isWhite"`
	Sunk         bool    `json:"sunk"`
	Sinking      bool    `json:"sinking,omitempty"`
	SinkProgress float64 `json:"sinkProgress,omitempty"`
	SinkStartX   float64 `json:"sinkStartX,omitempty"`
	SinkStartY   float64 `json:"sinkStartY,omitempty"`
	SinkX        float64 `json:"sinkX,omitempty"`
	SinkY        float64 `json:"sinkY,omitempty"`
	Color        string  `json:"color"`
	Owner        string  `json:"owner,omitempty"`
	Points       int     `json:"points,omitempty"`
}

type matchPocket struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type activeShotState struct {
	MatchID           string  `json:"match_id,omitempty"`
	ShotSeq           int     `json:"shot_seq"`
	ShooterUserID     string  `json:"shooter_user_id"`
	Angle             float64 `json:"angle"`
	Power             float64 `json:"power"`
	ServerStartedAtMS int64   `json:"server_started_at_ms"`
}

type matchSnapshot struct {
	MatchID          string           `json:"match_id,omitempty"`
	Balls            []matchBall      `json:"balls"`
	Pockets          []matchPocket    `json:"pockets"`
	Scores           matchScoreboard  `json:"scores"`
	TurnUserID       string           `json:"turn_user_id"`
	TurnSeq          int              `json:"turn_seq"`
	TurnStartedAtMS  int64            `json:"turn_started_at_ms,omitempty"`
	TurnDeadlineAtMS int64            `json:"turn_deadline_at_ms,omitempty"`
	ShotSeq          int              `json:"shot_seq"`
	Status           string           `json:"status"`
	WinnerUserID     string           `json:"winner_user_id,omitempty"`
	AuditHash        string           `json:"audit_hash,omitempty"`
	UpdatedAtMS      int64            `json:"updated_at_ms"`
	ActiveShot       *activeShotState `json:"active_shot,omitempty"`
}

type shotIntentPayload struct {
	MatchID string  `json:"match_id,omitempty"`
	Angle   float64 `json:"angle"`
	Power   float64 `json:"power"`
}

type shotResultPayload struct {
	MatchID        string        `json:"match_id,omitempty"`
	ShotSeq        int           `json:"shot_seq"`
	ShooterUserID  string        `json:"shooter_user_id,omitempty"`
	Balls          []matchBall   `json:"balls"`
	Pockets        []matchPocket `json:"pockets"`
	CueBallSunk    bool          `json:"cue_ball_sunk"`
	AuditHash      string        `json:"audit_hash,omitempty"`
	SimulatedTicks int           `json:"simulated_ticks,omitempty"`
}

func newInitialMatchSnapshot(room *Room) matchSnapshot {
	balls := initMatchBalls()
	now := time.Now().UnixMilli()
	snapshot := matchSnapshot{
		MatchID:          room.ID,
		Balls:            balls,
		Pockets:          createServerPockets(room.ID, 0, balls),
		Scores:           matchScoreboard{},
		TurnUserID:       room.CreatorID,
		TurnSeq:          1,
		TurnStartedAtMS:  now,
		TurnDeadlineAtMS: now + matchTurnDurationMS,
		ShotSeq:          0,
		Status:           matchStatusAiming,
		UpdatedAtMS:      now,
	}
	snapshot.AuditHash = auditHashForBalls(snapshot.Balls)
	return snapshot
}

func normalizeTurnClock(snapshot matchSnapshot, now int64) matchSnapshot {
	if snapshot.TurnSeq <= 0 {
		snapshot.TurnSeq = 1
	}
	if snapshot.Status != matchStatusAiming || snapshot.WinnerUserID != "" {
		snapshot.TurnStartedAtMS = 0
		snapshot.TurnDeadlineAtMS = 0
		return snapshot
	}
	if snapshot.TurnStartedAtMS <= 0 {
		snapshot.TurnStartedAtMS = now
	}
	if snapshot.TurnDeadlineAtMS <= 0 {
		snapshot.TurnDeadlineAtMS = snapshot.TurnStartedAtMS + matchTurnDurationMS
	}
	return snapshot
}

func isTurnExpired(snapshot matchSnapshot, now int64) bool {
	return snapshot.Status == matchStatusAiming &&
		snapshot.WinnerUserID == "" &&
		snapshot.TurnDeadlineAtMS > 0 &&
		now >= snapshot.TurnDeadlineAtMS
}

func initMatchBalls() []matchBall {
	balls := make([]matchBall, 0, 16)
	balls = append(balls, matchBall{
		ID:      0,
		X:       -matchTableRadius * 0.64,
		Y:       0,
		Radius:  matchBallRadius,
		IsWhite: true,
		Sunk:    false,
		Color:   "#ffffff",
	})

	addTargetBall := func(id int, x float64, y float64) {
		owner, points := ownerAndPointsForBall(id)
		balls = append(balls, matchBall{
			ID:      id,
			X:       x,
			Y:       y,
			Radius:  matchBallRadius,
			IsWhite: false,
			Sunk:    false,
			Color:   colorForBall(id),
			Owner:   owner,
			Points:  points,
		})
	}

	addTargetBall(8, 0, 0)

	outerIDs := []int{1, 2, 3, 4, 5, 6, 7, 9, 10, 11, 12, 13, 14, 15}
	const circleRadius = 0.16
	for index, id := range outerIDs {
		angle := (float64(index) / float64(len(outerIDs))) * math.Pi * 2
		addTargetBall(id, math.Cos(angle)*circleRadius, math.Sin(angle)*circleRadius)
	}

	return balls
}

func ownerAndPointsForBall(id int) (string, int) {
	switch {
	case id >= 1 && id <= 7:
		return "creator", 10
	case id >= 9 && id <= 15:
		return "opponent", 10
	case id == 8:
		return "neutral", 30
	default:
		return "", 0
	}
}

func colorForBall(id int) string {
	switch {
	case id >= 1 && id <= 7:
		return "#f4b942"
	case id >= 9 && id <= 15:
		return "#4aa3ff"
	case id == 8:
		return "#111111"
	default:
		return "#ffffff"
	}
}

func createServerPockets(roomID string, shotSeq int, balls []matchBall) []matchPocket {
	rng := rand.New(rand.NewSource(seedForRoomShot(roomID, shotSeq)))
	pockets := make([]matchPocket, 0, randomPocketCount)

	for attempt := 0; len(pockets) < randomPocketCount && attempt < randomPocketAttempts; attempt++ {
		angle := rng.Float64() * math.Pi * 2
		radius := math.Sqrt(rng.Float64()) * randomPocketPlacementLimit
		pocket := matchPocket{
			X: math.Cos(angle) * radius,
			Y: math.Sin(angle) * radius,
		}

		if pocketPlacementValid(pocket, pockets, balls) {
			pockets = append(pockets, pocket)
		}
	}

	for len(pockets) < randomPocketCount {
		angle := (float64(len(pockets)) / randomPocketCount * math.Pi * 2) + rng.Float64()*0.45
		radius := randomPocketPlacementLimit * (0.55 + rng.Float64()*0.32)
		pockets = append(pockets, matchPocket{
			X: math.Cos(angle) * radius,
			Y: math.Sin(angle) * radius,
		})
	}

	return pockets
}

func seedForRoomShot(roomID string, shotSeq int) int64 {
	sum := sha256.Sum256([]byte(roomID + ":" + strconv.Itoa(shotSeq)))
	return int64(binary.LittleEndian.Uint64(sum[:8]))
}

func pocketPlacementValid(pocket matchPocket, pockets []matchPocket, balls []matchBall) bool {
	minPocketDistanceSq := randomPocketMinDistance * randomPocketMinDistance
	ballClearanceSq := randomPocketBallClearance * randomPocketBallClearance

	for _, placed := range pockets {
		if distanceSq(pocket.X, pocket.Y, placed.X, placed.Y) < minPocketDistanceSq {
			return false
		}
	}

	for _, ball := range balls {
		if ball.Sunk {
			continue
		}
		if distanceSq(pocket.X, pocket.Y, ball.X, ball.Y) < ballClearanceSq {
			return false
		}
	}

	return true
}

func distanceSq(ax float64, ay float64, bx float64, by float64) float64 {
	dx := ax - bx
	dy := ay - by
	return dx*dx + dy*dy
}

func parseShotIntent(raw json.RawMessage) (shotIntentPayload, bool) {
	var payload shotIntentPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return payload, false
	}
	if !finite(payload.Angle) || !finite(payload.Power) || payload.Power <= 0 || payload.Power > 100 {
		return payload, false
	}
	payload.Angle = normalizeAngle(payload.Angle)
	return payload, true
}

func parseShotResult(raw json.RawMessage) (shotResultPayload, bool) {
	var payload shotResultPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return payload, false
	}
	if payload.ShotSeq < 0 || payload.SimulatedTicks < 0 {
		return payload, false
	}
	if payload.AuditHash != "" && len(payload.AuditHash) != 64 {
		return payload, false
	}
	return payload, true
}

func sanitizeSubmittedBalls(submitted []matchBall) ([]matchBall, bool) {
	if len(submitted) != 16 {
		return nil, false
	}

	seen := make(map[int]struct{}, 16)
	balls := make([]matchBall, 0, 16)
	for _, ball := range submitted {
		if ball.ID < 0 || ball.ID > 15 {
			return nil, false
		}
		if _, exists := seen[ball.ID]; exists {
			return nil, false
		}
		seen[ball.ID] = struct{}{}

		if !finite(ball.X) ||
			!finite(ball.Y) ||
			!finite(ball.VX) ||
			!finite(ball.VY) ||
			!finite(ball.SpinX) ||
			!finite(ball.SpinY) ||
			math.Abs(ball.X) > matchTableRadius+0.25 ||
			math.Abs(ball.Y) > matchTableRadius+0.25 ||
			math.Abs(ball.VX) > 8 ||
			math.Abs(ball.VY) > 8 ||
			math.Abs(ball.SpinX) > 500 ||
			math.Abs(ball.SpinY) > 500 {
			return nil, false
		}

		owner, points := ownerAndPointsForBall(ball.ID)
		normalized := matchBall{
			ID:      ball.ID,
			X:       ball.X,
			Y:       ball.Y,
			VX:      0,
			VY:      0,
			SpinX:   0,
			SpinY:   0,
			Radius:  matchBallRadius,
			IsWhite: ball.ID == 0,
			Sunk:    ball.Sunk,
			Color:   colorForBall(ball.ID),
			Owner:   owner,
			Points:  points,
		}
		if normalized.IsWhite {
			normalized.Sunk = false
		}
		balls = append(balls, normalized)
	}

	if len(seen) != 16 {
		return nil, false
	}
	sort.Slice(balls, func(i int, j int) bool {
		return balls[i].ID < balls[j].ID
	})
	return balls, true
}

func sanitizeSubmittedPockets(submitted []matchPocket, roomID string, shotSeq int, balls []matchBall, finished bool) []matchPocket {
	if finished {
		return nil
	}
	return createServerPockets(roomID, shotSeq, balls)
}

func applyShotResult(room *Room, current matchSnapshot, result shotResultPayload) (matchSnapshot, bool) {
	if current.ActiveShot == nil ||
		current.Status != matchStatusMoving {
		return current, false
	}
	if current.ActiveShot.ShotSeq != result.ShotSeq {
		if result.ShotSeq != current.ActiveShot.ShotSeq-1 {
			return current, false
		}
		result.ShotSeq = current.ActiveShot.ShotSeq
	}

	if result.ShooterUserID != "" && result.ShooterUserID != current.ActiveShot.ShooterUserID {
		return current, false
	}

	nextBalls, ok := sanitizeSubmittedBalls(result.Balls)
	if !ok {
		return current, false
	}
	if !preservesPreviouslySunkTargets(current.Balls, nextBalls) {
		return current, false
	}
	nextAuditHash := auditHashForBalls(nextBalls)
	if result.AuditHash != "" && result.AuditHash != nextAuditHash {
		return current, false
	}

	shooterRole := roleForUser(current.ActiveShot.ShooterUserID, room.CreatorID, room.OpponentID)
	beforeSunk := sunkTargetIDs(current.Balls)
	nextScores := current.Scores
	shooterSunkOwnBall := false
	shooterSunkBlack := false

	for _, ball := range nextBalls {
		if ball.IsWhite || beforeSunk[ball.ID] || !ball.Sunk {
			continue
		}

		if ball.ID == 8 {
			shooterSunkBlack = true
			continue
		}

		switch ball.Owner {
		case "creator":
			nextScores.Creator += ball.Points
		case "opponent":
			nextScores.Opponent += ball.Points
		case "neutral":
			if shooterRole == "creator" {
				nextScores.Creator += ball.Points
			} else if shooterRole == "opponent" {
				nextScores.Opponent += ball.Points
			}
		}

		if !result.CueBallSunk && shooterRole != "" && (ball.Owner == shooterRole || ball.Owner == "neutral") {
			shooterSunkOwnBall = true
		}
	}

	nextWinnerUserID := ""
	if shooterSunkBlack {
		nextWinnerUserID = opponentUserIDFor(current.ActiveShot.ShooterUserID, room)
	} else {
		winnerRole := resolveServerWinnerRole(nextBalls, nextScores, shooterRole)
		if winnerRole == "creator" {
			nextWinnerUserID = room.CreatorID
		} else if winnerRole == "opponent" && room.OpponentID != nil {
			nextWinnerUserID = *room.OpponentID
		}
	}

	nextStatus := matchStatusAiming
	if nextWinnerUserID != "" {
		nextStatus = matchStatusFinished
	}

	nextTurnUserID := current.TurnUserID
	if nextWinnerUserID == "" {
		if shooterSunkOwnBall {
			nextTurnUserID = current.ActiveShot.ShooterUserID
		} else {
			nextTurnUserID = nextTurnUserIDFor(current.TurnUserID, room.CreatorID, room.OpponentID)
		}
	}

	now := time.Now().UnixMilli()
	next := matchSnapshot{
		MatchID:      room.ID,
		Balls:        nextBalls,
		Scores:       nextScores,
		TurnUserID:   nextTurnUserID,
		TurnSeq:      current.TurnSeq,
		ShotSeq:      result.ShotSeq,
		Status:       nextStatus,
		WinnerUserID: nextWinnerUserID,
		UpdatedAtMS:  now,
	}
	if nextStatus == matchStatusAiming {
		next.TurnSeq = current.TurnSeq + 1
		next.TurnStartedAtMS = now
		next.TurnDeadlineAtMS = now + matchTurnDurationMS
	}
	next.Pockets = sanitizeSubmittedPockets(result.Pockets, room.ID, result.ShotSeq, nextBalls, nextWinnerUserID != "")
	next.AuditHash = nextAuditHash
	return next, true
}

type turnTimeoutPayload struct {
	MatchID          string `json:"match_id,omitempty"`
	TurnSeq          int    `json:"turn_seq"`
	TimedOutUserID   string `json:"timed_out_user_id"`
	NextTurnUserID   string `json:"next_turn_user_id"`
	NextTurnSeq      int    `json:"next_turn_seq"`
	TurnStartedAtMS  int64  `json:"turn_started_at_ms"`
	TurnDeadlineAtMS int64  `json:"turn_deadline_at_ms"`
}

func applyTurnTimeout(room *Room, current matchSnapshot, now int64) (matchSnapshot, turnTimeoutPayload, bool) {
	if !isTurnExpired(current, now) {
		return current, turnTimeoutPayload{}, false
	}

	nextTurnUserID := nextTurnUserIDFor(current.TurnUserID, room.CreatorID, room.OpponentID)
	next := current
	next.TurnUserID = nextTurnUserID
	next.TurnSeq = current.TurnSeq + 1
	next.TurnStartedAtMS = now
	next.TurnDeadlineAtMS = now + matchTurnDurationMS
	next.Status = matchStatusAiming
	next.ActiveShot = nil
	next.UpdatedAtMS = now

	payload := turnTimeoutPayload{
		MatchID:          room.ID,
		TurnSeq:          current.TurnSeq,
		TimedOutUserID:   current.TurnUserID,
		NextTurnUserID:   nextTurnUserID,
		NextTurnSeq:      next.TurnSeq,
		TurnStartedAtMS:  next.TurnStartedAtMS,
		TurnDeadlineAtMS: next.TurnDeadlineAtMS,
	}
	return next, payload, true
}

func sunkTargetIDs(balls []matchBall) map[int]bool {
	ids := make(map[int]bool)
	for _, ball := range balls {
		if !ball.IsWhite && ball.Sunk {
			ids[ball.ID] = true
		}
	}
	return ids
}

func preservesPreviouslySunkTargets(previous []matchBall, next []matchBall) bool {
	nextByID := make(map[int]matchBall, len(next))
	for _, ball := range next {
		nextByID[ball.ID] = ball
	}
	for _, ball := range previous {
		if ball.IsWhite || !ball.Sunk {
			continue
		}
		nextBall, ok := nextByID[ball.ID]
		if !ok || !nextBall.Sunk {
			return false
		}
	}
	return true
}

func roleForUser(userID string, creatorID string, opponentID *string) string {
	if userID == creatorID {
		return "creator"
	}
	if opponentID != nil && userID == *opponentID {
		return "opponent"
	}
	return ""
}

func opponentUserIDFor(userID string, room *Room) string {
	if room == nil {
		return ""
	}
	if userID == room.CreatorID && room.OpponentID != nil {
		return *room.OpponentID
	}
	if room.OpponentID != nil && userID == *room.OpponentID {
		return room.CreatorID
	}
	return ""
}

func nextTurnUserIDFor(currentTurnUserID string, creatorID string, opponentID *string) string {
	if opponentID == nil {
		return creatorID
	}
	if currentTurnUserID == creatorID {
		return *opponentID
	}
	return creatorID
}

func resolveServerWinnerRole(balls []matchBall, scores matchScoreboard, shooterRole string) string {
	creatorCleared := true
	opponentCleared := true
	neutralSunk := false

	for _, ball := range balls {
		switch ball.Owner {
		case "creator":
			if !ball.Sunk {
				creatorCleared = false
			}
		case "opponent":
			if !ball.Sunk {
				opponentCleared = false
			}
		case "neutral":
			if ball.Sunk {
				neutralSunk = true
			}
		}
	}

	if creatorCleared && !opponentCleared {
		return "creator"
	}
	if opponentCleared && !creatorCleared {
		return "opponent"
	}
	if !neutralSunk && !(creatorCleared && opponentCleared) {
		return ""
	}
	if scores.Creator > scores.Opponent {
		return "creator"
	}
	if scores.Opponent > scores.Creator {
		return "opponent"
	}
	return shooterRole
}

func auditHashForBalls(balls []matchBall) string {
	type auditPosition struct {
		ID   int     `json:"id"`
		X    float64 `json:"x"`
		Y    float64 `json:"y"`
		Sunk bool    `json:"sunk"`
	}

	positions := make([]auditPosition, 0, len(balls))
	sortedBalls := append([]matchBall(nil), balls...)
	sort.Slice(sortedBalls, func(i int, j int) bool {
		return sortedBalls[i].ID < sortedBalls[j].ID
	})
	for _, ball := range sortedBalls {
		positions = append(positions, auditPosition{
			ID:   ball.ID,
			X:    roundTo4(ball.X),
			Y:    roundTo4(ball.Y),
			Sunk: ball.Sunk,
		})
	}

	bytes, _ := json.Marshal(positions)
	sum := sha256.Sum256(bytes)
	return hex.EncodeToString(sum[:])
}

func roundTo4(value float64) float64 {
	return math.Round(value*10000) / 10000
}
