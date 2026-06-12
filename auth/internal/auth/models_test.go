package auth

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestRefreshToken_IsExpired(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt time.Time
		expected  bool
	}{
		{"token not expired", time.Now().Add(1 * time.Hour), false},
		{"token expired", time.Now().Add(-1 * time.Hour), true},
		{"token just expired", time.Now().Add(-1 * time.Millisecond), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := &RefreshToken{ExpiresAt: tt.expiresAt}
			assert.Equal(t, tt.expected, rt.IsExpired())
		})
	}
}

func TestRefreshToken_IsActive(t *testing.T) {
	tests := []struct {
		name      string
		revoked   bool
		expiresAt time.Time
		expected  bool
	}{
		{"active token", false, time.Now().Add(1 * time.Hour), true},
		{"inactive revoked", true, time.Now().Add(1 * time.Hour), false},
		{"inactive expired", false, time.Now().Add(-1 * time.Hour), false},
		{"inactive revoked and expired", true, time.Now().Add(-1 * time.Hour), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := &RefreshToken{
				Revoked:   tt.revoked,
				ExpiresAt: tt.expiresAt,
			}
			assert.Equal(t, tt.expected, rt.IsActive())
		})
	}
}

func TestRefreshToken_IsWithinGracePeriod(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name      string
		revokedAt *time.Time
		expected  bool
	}{
		{"no revokedAt", nil, false},
		{"revoked 5 seconds ago", timePtr(now.Add(-5 * time.Second)), true},
		{"revoked 14 seconds ago", timePtr(now.Add(-14 * time.Second)), true},
		{"revoked 16 seconds ago", timePtr(now.Add(-16 * time.Second)), false},
		{"revoked 5 minutes ago", timePtr(now.Add(-5 * time.Minute)), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := &RefreshToken{
				ID:        uuid.New(),
				RevokedAt: tt.revokedAt,
			}
			assert.Equal(t, tt.expected, rt.IsWithinGracePeriod())
		})
	}
}

func TestUserStatus_Constants(t *testing.T) {
	assert.Equal(t, UserStatus("onboarding_pending"), StatusOnboardingPending)
	assert.Equal(t, UserStatus("active"), StatusActive)
	assert.Equal(t, UserStatus("blocked"), StatusBlocked)
}

func TestAuthProvider_Constants(t *testing.T) {
	assert.Equal(t, AuthProvider("local"), ProviderLocal)
	assert.Equal(t, AuthProvider("google"), ProviderGoogle)
}

func timePtr(t time.Time) *time.Time {
	return &t
}
