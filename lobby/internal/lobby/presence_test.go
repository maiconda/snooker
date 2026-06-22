package lobby

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestPresenceTrackerOnlineUsersAreUnique(t *testing.T) {
	tracker := newPresenceTracker()

	tracker.RegisterOnline("user-1", "conn-1")
	tracker.RegisterOnline("user-1", "conn-2")
	tracker.RegisterOnline("user-2", "conn-3")

	snapshot := tracker.OnlineSnapshot()
	assert.Equal(t, 2, snapshot.Count)
	assert.ElementsMatch(t, []PresenceUser{{UserID: "user-1"}, {UserID: "user-2"}}, snapshot.Users)

	tracker.UnregisterOnline("user-1", "conn-1")
	assert.True(t, tracker.IsOnline("user-1"))

	tracker.UnregisterOnline("user-1", "conn-2")
	assert.False(t, tracker.IsOnline("user-1"))
}

func TestPresenceTrackerRoomSpectatorsAreUnique(t *testing.T) {
	tracker := newPresenceTracker()

	tracker.RegisterRoomSpectator("room-1", "user-1", "conn-1")
	tracker.RegisterRoomSpectator("room-1", "user-1", "conn-2")
	tracker.RegisterRoomSpectator("room-1", "user-2", "conn-3")

	snapshot := tracker.RoomSpectatorsSnapshot("room-1")
	assert.Equal(t, 2, snapshot.Count)
	assert.ElementsMatch(t, []PresenceUser{{UserID: "user-1"}, {UserID: "user-2"}}, snapshot.Spectators)

	tracker.UnregisterRoomSpectator("room-1", "user-1", "conn-1")
	assert.Equal(t, 2, tracker.RoomSpectatorsSnapshot("room-1").Count)

	tracker.UnregisterRoomSpectator("room-1", "user-1", "conn-2")
	assert.Equal(t, 1, tracker.RoomSpectatorsSnapshot("room-1").Count)
}

func TestPresenceTrackerAllowsOneRoomConnectionPerUser(t *testing.T) {
	tracker := newPresenceTracker()

	roomID, ok := tracker.RegisterRoomConnectionIfFree("room-1", "user-1", "conn-1")
	assert.True(t, ok)
	assert.Empty(t, roomID)

	roomID, ok = tracker.RegisterRoomConnectionIfFree("room-1", "user-1", "conn-2")
	assert.False(t, ok)
	assert.Equal(t, "room-1", roomID)

	roomID, busy := tracker.UserInRoomConnection("user-1", "")
	assert.True(t, busy)
	assert.Equal(t, "room-1", roomID)

	roomID, busy = tracker.UserInRoomConnection("user-1", "room-1")
	assert.False(t, busy)
	assert.Empty(t, roomID)

	assert.False(t, tracker.UnregisterRoomConnection("room-1", "user-1", "conn-2"))
	assert.True(t, tracker.UnregisterRoomConnection("room-1", "user-1", "conn-1"))

	roomID, ok = tracker.RegisterRoomConnectionIfFree("room-2", "user-1", "conn-3")
	assert.True(t, ok)
	assert.Empty(t, roomID)
}

func TestPresenceTrackerCanRemoveSpectatorUserFromRoom(t *testing.T) {
	tracker := newPresenceTracker()

	tracker.RegisterRoomSpectator("room-1", "user-1", "conn-1")
	tracker.RegisterRoomSpectator("room-1", "user-1", "conn-2")
	tracker.RegisterRoomSpectator("room-1", "user-2", "conn-3")

	assert.True(t, tracker.UnregisterRoomSpectatorUser("room-1", "user-1"))
	snapshot := tracker.RoomSpectatorsSnapshot("room-1")
	assert.Equal(t, 1, snapshot.Count)
	assert.ElementsMatch(t, []PresenceUser{{UserID: "user-2"}}, snapshot.Spectators)

	assert.False(t, tracker.UnregisterRoomSpectatorUser("room-1", "user-1"))
}

func TestInvitationStoreUpsertListAndClearRoom(t *testing.T) {
	store := newInvitationStore()
	room := RoomResponse{ID: "room-1", CreatorID: "owner-1", Status: StatusWaiting}
	firstInvite := RoomInvitePayload{
		InvitationID: "invite-1",
		Room:         room,
		FromUserID:   "owner-1",
		ToUserID:     "user-1",
		CreatedAt:    time.Now().UTC(),
	}
	secondInvite := firstInvite
	secondInvite.InvitationID = "invite-2"

	store.Upsert(firstInvite)
	store.Upsert(secondInvite)

	invites := store.ListForUser("user-1")
	assert.Len(t, invites, 1)
	assert.Equal(t, "invite-2", invites[0].InvitationID)

	cleared := store.ClearRoom("room-1", "room_filled")
	assert.Len(t, cleared, 1)
	assert.Equal(t, "invite-2", cleared[0].InvitationID)
	assert.Equal(t, "owner-1", cleared[0].FromUserID)
	assert.Equal(t, "room_filled", cleared[0].Reason)
	assert.Empty(t, store.ListForUser("user-1"))
}

func TestInvitationStoreClearInvitation(t *testing.T) {
	store := newInvitationStore()
	invite := RoomInvitePayload{
		InvitationID: "invite-1",
		Room:         RoomResponse{ID: "room-1", CreatorID: "owner-1", Status: StatusWaiting},
		FromUserID:   "owner-1",
		ToUserID:     "user-1",
		CreatedAt:    time.Now().UTC(),
	}
	store.Upsert(invite)

	cleared, ok := store.ClearInvitation("invite-1", "user-1", "declined")
	assert.True(t, ok)
	assert.NotNil(t, cleared)
	assert.Equal(t, "owner-1", cleared.FromUserID)
	assert.Equal(t, "user-1", cleared.ToUserID)
	assert.Equal(t, "declined", cleared.Reason)
	assert.Empty(t, store.ListForUser("user-1"))

	_, ok = store.ClearInvitation("invite-1", "user-1", "declined")
	assert.False(t, ok)
}
