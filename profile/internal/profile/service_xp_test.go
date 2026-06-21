package profile

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestAwardMatchXPAddsParticipationAndWinnerBonus(t *testing.T) {
	winnerID := uuid.New()
	opponentID := uuid.New()
	repo := &fakeXPRepository{
		profiles: map[uuid.UUID]*Profile{
			winnerID:   {UserID: winnerID, XP: 90},
			opponentID: {UserID: opponentID, XP: 10},
		},
	}
	service := NewService(repo, nil, nil, 0)

	resp, err := service.AwardMatchXP(context.Background(), &MatchXPRequest{
		WinnerUserID:       winnerID,
		ParticipantUserIDs: []uuid.UUID{winnerID, opponentID, winnerID},
	})

	assert.NoError(t, err)
	assert.Equal(t, []XPAward{
		{UserID: winnerID, XPDelta: 50, TotalXP: 140},
		{UserID: opponentID, XPDelta: 25, TotalXP: 35},
	}, resp.Awards)
}

type fakeXPRepository struct {
	profiles map[uuid.UUID]*Profile
}

func (r *fakeXPRepository) FindByUserID(ctx context.Context, userID uuid.UUID) (*Profile, error) {
	profile, ok := r.profiles[userID]
	if !ok {
		return nil, ErrNotFound
	}
	copy := *profile
	return &copy, nil
}

func (r *fakeXPRepository) IncrementXP(ctx context.Context, userID uuid.UUID, delta int) (*Profile, error) {
	profile, ok := r.profiles[userID]
	if !ok {
		return nil, ErrNotFound
	}
	profile.XP += delta
	copy := *profile
	return &copy, nil
}

func (r *fakeXPRepository) Upsert(ctx context.Context, p *Profile) (*Profile, error) {
	return p, nil
}

func (r *fakeXPRepository) Update(ctx context.Context, p *Profile) (*Profile, error) {
	return p, nil
}

func (r *fakeXPRepository) CreateUploadSession(ctx context.Context, session *PhotoUploadSession) (*PhotoUploadSession, error) {
	return session, nil
}

func (r *fakeXPRepository) FindUploadSession(ctx context.Context, id uuid.UUID, userID uuid.UUID) (*PhotoUploadSession, error) {
	return nil, ErrNotFound
}

func (r *fakeXPRepository) MarkUploadConsumed(ctx context.Context, id uuid.UUID) error {
	return nil
}
