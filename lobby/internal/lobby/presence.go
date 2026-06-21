package lobby

import (
	"sort"
	"sync"
	"time"
)

type PresenceUser struct {
	UserID string `json:"user_id"`
}

type OnlineUsersSnapshot struct {
	Users []PresenceUser `json:"users"`
	Count int            `json:"count"`
}

type RoomSpectatorsSnapshot struct {
	RoomID     string         `json:"room_id"`
	Spectators []PresenceUser `json:"spectators"`
	Count      int            `json:"count"`
}

type presenceTracker struct {
	mu             sync.RWMutex
	online         map[string]map[string]struct{}
	roomSpectators map[string]map[string]map[string]struct{}
}

func newPresenceTracker() *presenceTracker {
	return &presenceTracker{
		online:         make(map[string]map[string]struct{}),
		roomSpectators: make(map[string]map[string]map[string]struct{}),
	}
}

func (p *presenceTracker) RegisterOnline(userID string, connectionID string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.online[userID] == nil {
		p.online[userID] = make(map[string]struct{})
	}
	p.online[userID][connectionID] = struct{}{}
}

func (p *presenceTracker) UnregisterOnline(userID string, connectionID string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	connections := p.online[userID]
	if connections == nil {
		return
	}
	delete(connections, connectionID)
	if len(connections) == 0 {
		delete(p.online, userID)
	}
}

func (p *presenceTracker) IsOnline(userID string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return len(p.online[userID]) > 0
}

func (p *presenceTracker) OnlineSnapshot() OnlineUsersSnapshot {
	p.mu.RLock()
	defer p.mu.RUnlock()

	users := make([]PresenceUser, 0, len(p.online))
	for userID, connections := range p.online {
		if len(connections) == 0 {
			continue
		}
		users = append(users, PresenceUser{UserID: userID})
	}
	sort.Slice(users, func(i, j int) bool {
		return users[i].UserID < users[j].UserID
	})

	return OnlineUsersSnapshot{Users: users, Count: len(users)}
}

func (p *presenceTracker) RegisterRoomSpectator(roomID string, userID string, connectionID string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.roomSpectators[roomID] == nil {
		p.roomSpectators[roomID] = make(map[string]map[string]struct{})
	}
	if p.roomSpectators[roomID][userID] == nil {
		p.roomSpectators[roomID][userID] = make(map[string]struct{})
	}
	p.roomSpectators[roomID][userID][connectionID] = struct{}{}
}

func (p *presenceTracker) UnregisterRoomSpectator(roomID string, userID string, connectionID string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	users := p.roomSpectators[roomID]
	if users == nil {
		return
	}
	connections := users[userID]
	if connections == nil {
		return
	}
	delete(connections, connectionID)
	if len(connections) == 0 {
		delete(users, userID)
	}
	if len(users) == 0 {
		delete(p.roomSpectators, roomID)
	}
}

func (p *presenceTracker) UnregisterRoomSpectatorUser(roomID string, userID string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	users := p.roomSpectators[roomID]
	if users == nil {
		return false
	}
	if _, ok := users[userID]; !ok {
		return false
	}

	delete(users, userID)
	if len(users) == 0 {
		delete(p.roomSpectators, roomID)
	}
	return true
}

func (p *presenceTracker) RoomSpectatorsSnapshot(roomID string) RoomSpectatorsSnapshot {
	p.mu.RLock()
	defer p.mu.RUnlock()

	roomUsers := p.roomSpectators[roomID]
	users := make([]PresenceUser, 0, len(roomUsers))
	for userID, connections := range roomUsers {
		if len(connections) == 0 {
			continue
		}
		users = append(users, PresenceUser{UserID: userID})
	}
	sort.Slice(users, func(i, j int) bool {
		return users[i].UserID < users[j].UserID
	})

	return RoomSpectatorsSnapshot{RoomID: roomID, Spectators: users, Count: len(users)}
}

type RoomInvitePayload struct {
	InvitationID string       `json:"invitation_id"`
	Room         RoomResponse `json:"room"`
	FromUserID   string       `json:"from_user_id"`
	ToUserID     string       `json:"to_user_id"`
	CreatedAt    time.Time    `json:"created_at"`
}

type RoomInviteClearedPayload struct {
	InvitationID string `json:"invitation_id"`
	RoomID       string `json:"room_id"`
	FromUserID   string `json:"from_user_id"`
	ToUserID     string `json:"to_user_id"`
	Reason       string `json:"reason"`
}

type invitationStore struct {
	mu           sync.RWMutex
	byInvitation map[string]RoomInvitePayload
	byRoom       map[string]map[string]RoomInvitePayload
	byUser       map[string]map[string]RoomInvitePayload
}

func newInvitationStore() *invitationStore {
	return &invitationStore{
		byInvitation: make(map[string]RoomInvitePayload),
		byRoom:       make(map[string]map[string]RoomInvitePayload),
		byUser:       make(map[string]map[string]RoomInvitePayload),
	}
}

func (s *invitationStore) Upsert(invite RoomInvitePayload) {
	s.mu.Lock()
	defer s.mu.Unlock()

	roomID := invite.Room.ID
	if s.byRoom[roomID] == nil {
		s.byRoom[roomID] = make(map[string]RoomInvitePayload)
	}
	if s.byUser[invite.ToUserID] == nil {
		s.byUser[invite.ToUserID] = make(map[string]RoomInvitePayload)
	}

	if previous, ok := s.byRoom[roomID][invite.ToUserID]; ok {
		delete(s.byUser[invite.ToUserID], previous.InvitationID)
		delete(s.byInvitation, previous.InvitationID)
	}

	s.byRoom[roomID][invite.ToUserID] = invite
	s.byUser[invite.ToUserID][invite.InvitationID] = invite
	s.byInvitation[invite.InvitationID] = invite
}

func (s *invitationStore) ListForUser(userID string) []RoomInvitePayload {
	s.mu.RLock()
	defer s.mu.RUnlock()

	invites := make([]RoomInvitePayload, 0, len(s.byUser[userID]))
	for _, invite := range s.byUser[userID] {
		invites = append(invites, invite)
	}
	sort.Slice(invites, func(i, j int) bool {
		return invites[i].CreatedAt.Before(invites[j].CreatedAt)
	})
	return invites
}

func (s *invitationStore) ClearRoom(roomID string, reason string) []RoomInviteClearedPayload {
	s.mu.Lock()
	defer s.mu.Unlock()

	invites := s.byRoom[roomID]
	if len(invites) == 0 {
		return nil
	}

	cleared := make([]RoomInviteClearedPayload, 0, len(invites))
	for toUserID, invite := range invites {
		cleared = append(cleared, RoomInviteClearedPayload{
			InvitationID: invite.InvitationID,
			RoomID:       roomID,
			FromUserID:   invite.FromUserID,
			ToUserID:     toUserID,
			Reason:       reason,
		})
		delete(s.byInvitation, invite.InvitationID)
		delete(s.byUser[toUserID], invite.InvitationID)
		if len(s.byUser[toUserID]) == 0 {
			delete(s.byUser, toUserID)
		}
	}
	delete(s.byRoom, roomID)

	sort.Slice(cleared, func(i, j int) bool {
		return cleared[i].ToUserID < cleared[j].ToUserID
	})
	return cleared
}

func (s *invitationStore) ClearInvitation(invitationID string, userID string, reason string) (*RoomInviteClearedPayload, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	invite, ok := s.byInvitation[invitationID]
	if !ok || invite.ToUserID != userID {
		return nil, false
	}

	roomID := invite.Room.ID
	delete(s.byInvitation, invitationID)
	delete(s.byUser[invite.ToUserID], invitationID)
	if len(s.byUser[invite.ToUserID]) == 0 {
		delete(s.byUser, invite.ToUserID)
	}
	if s.byRoom[roomID] != nil {
		delete(s.byRoom[roomID], invite.ToUserID)
		if len(s.byRoom[roomID]) == 0 {
			delete(s.byRoom, roomID)
		}
	}

	return &RoomInviteClearedPayload{
		InvitationID: invitationID,
		RoomID:       roomID,
		FromUserID:   invite.FromUserID,
		ToUserID:     invite.ToUserID,
		Reason:       reason,
	}, true
}

type rematchStore struct {
	mu       sync.RWMutex
	byRoom   map[string]map[string]struct{}
	required map[string]map[string]struct{}
}

func newRematchStore() *rematchStore {
	return &rematchStore{
		byRoom:   make(map[string]map[string]struct{}),
		required: make(map[string]map[string]struct{}),
	}
}

func (s *rematchStore) ConfigureRoom(roomID string, userIDs ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	required := make(map[string]struct{}, len(userIDs))
	for _, userID := range userIDs {
		if userID == "" {
			continue
		}
		required[userID] = struct{}{}
	}
	if len(required) == 0 {
		delete(s.required, roomID)
		delete(s.byRoom, roomID)
		return
	}

	s.required[roomID] = required
	if s.byRoom[roomID] == nil {
		s.byRoom[roomID] = make(map[string]struct{})
	}
	for userID := range s.byRoom[roomID] {
		if _, ok := required[userID]; !ok {
			delete(s.byRoom[roomID], userID)
		}
	}
}

func (s *rematchStore) Request(roomID string, userID string) (requested []string, allReady bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.byRoom[roomID] == nil {
		s.byRoom[roomID] = make(map[string]struct{})
	}
	s.byRoom[roomID][userID] = struct{}{}

	requested = make([]string, 0, len(s.byRoom[roomID]))
	for requestedUserID := range s.byRoom[roomID] {
		requested = append(requested, requestedUserID)
	}
	sort.Strings(requested)

	required := s.required[roomID]
	if len(required) == 0 {
		return requested, false
	}
	for requiredUserID := range required {
		if _, ok := s.byRoom[roomID][requiredUserID]; !ok {
			return requested, false
		}
	}
	return requested, true
}

func (s *rematchStore) ClearRoom(roomID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.byRoom, roomID)
	delete(s.required, roomID)
}
