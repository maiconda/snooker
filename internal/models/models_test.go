package models

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
		{
			name:      "token not expired",
			expiresAt: time.Now().Add(1 * time.Hour),
			expected:  false,
		},
		{
			name:      "token expired",
			expiresAt: time.Now().Add(-1 * time.Hour),
			expected:  true,
		},
		{
			name:      "token just expired",
			expiresAt: time.Now().Add(-1 * time.Millisecond),
			expected:  true,
		},
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
		{
			name:      "active token - not revoked and not expired",
			revoked:   false,
			expiresAt: time.Now().Add(1 * time.Hour),
			expected:  true,
		},
		{
			name:      "inactive - revoked",
			revoked:   true,
			expiresAt: time.Now().Add(1 * time.Hour),
			expected:  false,
		},
		{
			name:      "inactive - expired",
			revoked:   false,
			expiresAt: time.Now().Add(-1 * time.Hour),
			expected:  false,
		},
		{
			name:      "inactive - revoked and expired",
			revoked:   true,
			expiresAt: time.Now().Add(-1 * time.Hour),
			expected:  false,
		},
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
		{
			name:      "no revokedAt - not in grace period",
			revokedAt: nil,
			expected:  false,
		},
		{
			name:      "revoked 5 seconds ago - within grace period",
			revokedAt: timePtr(now.Add(-5 * time.Second)),
			expected:  true,
		},
		{
			name:      "revoked 14 seconds ago - within grace period",
			revokedAt: timePtr(now.Add(-14 * time.Second)),
			expected:  true,
		},
		{
			name:      "revoked 16 seconds ago - outside grace period",
			revokedAt: timePtr(now.Add(-16 * time.Second)),
			expected:  false,
		},
		{
			name:      "revoked 5 minutes ago - outside grace period",
			revokedAt: timePtr(now.Add(-5 * time.Minute)),
			expected:  false,
		},
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
