package lobby

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"snooker/lobby/internal/httpx"
)

type MockRepository struct {
	mock.Mock
}

func (m *MockRepository) CreateRoom(ctx context.Context, creatorID string, isPrivate bool) (*Room, error) {
	args := m.Called(ctx, creatorID, isPrivate)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Room), args.Error(1)
}

func (m *MockRepository) GetRoomByID(ctx context.Context, id string) (*Room, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Room), args.Error(1)
}

func (m *MockRepository) GetRoomByCode(ctx context.Context, code string) (*Room, error) {
	args := m.Called(ctx, code)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Room), args.Error(1)
}

func (m *MockRepository) ListPublicRooms(ctx context.Context) ([]*Room, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*Room), args.Error(1)
}

func (m *MockRepository) JoinRoom(ctx context.Context, roomID string, opponentID string) (*Room, error) {
	args := m.Called(ctx, roomID, opponentID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Room), args.Error(1)
}

func (m *MockRepository) StartRoom(ctx context.Context, roomID string) (*Room, error) {
	args := m.Called(ctx, roomID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Room), args.Error(1)
}

func (m *MockRepository) FinishRoom(ctx context.Context, roomID string) (*Room, error) {
	args := m.Called(ctx, roomID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Room), args.Error(1)
}

func (m *MockRepository) ResetRoom(ctx context.Context, roomID string) (*Room, error) {
	args := m.Called(ctx, roomID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Room), args.Error(1)
}

func (m *MockRepository) StartRematchRoom(ctx context.Context, roomID string) (*Room, error) {
	args := m.Called(ctx, roomID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Room), args.Error(1)
}

func (m *MockRepository) ExpireRooms(ctx context.Context) ([]string, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), args.Error(1)
}

func (m *MockRepository) CloseRoom(ctx context.Context, roomID string) error {
	args := m.Called(ctx, roomID)
	return args.Error(0)
}

func (m *MockRepository) LeaveRoom(ctx context.Context, roomID string, userID string) (*Room, LeaveRole, error) {
	args := m.Called(ctx, roomID, userID)
	if args.Get(0) == nil {
		return nil, "", args.Error(2)
	}
	return args.Get(0).(*Room), args.Get(1).(LeaveRole), args.Error(2)
}

func (m *MockRepository) MarkCreatorConnected(ctx context.Context, roomID string, creatorID string, connectionID string) (*Room, error) {
	args := m.Called(ctx, roomID, creatorID, connectionID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Room), args.Error(1)
}

func (m *MockRepository) MarkCreatorDisconnected(ctx context.Context, roomID string, creatorID string, connectionID string, disconnectedAt time.Time) (bool, error) {
	args := m.Called(ctx, roomID, creatorID, connectionID, disconnectedAt)
	return args.Bool(0), args.Error(1)
}

func (m *MockRepository) MarkOpponentConnected(ctx context.Context, roomID string, opponentID string, connectionID string) (*Room, error) {
	args := m.Called(ctx, roomID, opponentID, connectionID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Room), args.Error(1)
}

func (m *MockRepository) MarkOpponentDisconnected(ctx context.Context, roomID string, opponentID string, connectionID string, disconnectedAt time.Time) (bool, error) {
	args := m.Called(ctx, roomID, opponentID, connectionID, disconnectedAt)
	return args.Bool(0), args.Error(1)
}

func (m *MockRepository) CloseRoomIfCreatorDisconnected(ctx context.Context, roomID string, disconnectedBefore time.Time) (bool, error) {
	args := m.Called(ctx, roomID, disconnectedBefore)
	return args.Bool(0), args.Error(1)
}

func (m *MockRepository) CloseRoomsWithExpiredCreatorDisconnect(ctx context.Context, disconnectedBefore time.Time) ([]string, error) {
	args := m.Called(ctx, disconnectedBefore)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), args.Error(1)
}

func (m *MockRepository) ReleaseOpponentIfDisconnected(ctx context.Context, roomID string, disconnectedBefore time.Time) (*Room, bool, error) {
	args := m.Called(ctx, roomID, disconnectedBefore)
	if args.Get(0) == nil {
		return nil, args.Bool(1), args.Error(2)
	}
	return args.Get(0).(*Room), args.Bool(1), args.Error(2)
}

func (m *MockRepository) ReleaseRoomsWithExpiredOpponentDisconnect(ctx context.Context, disconnectedBefore time.Time) ([]ReleasedOpponent, error) {
	args := m.Called(ctx, disconnectedBefore)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]ReleasedOpponent), args.Error(1)
}

func expectNoLifecycleChanges(repo *MockRepository) {
	repo.On("ExpireRooms", mock.Anything).Return([]string{}, nil).Maybe()
	repo.On("CloseRoomsWithExpiredCreatorDisconnect", mock.Anything, mock.Anything).Return([]string{}, nil).Maybe()
	repo.On("ReleaseRoomsWithExpiredOpponentDisconnect", mock.Anything, mock.Anything).Return([]ReleasedOpponent{}, nil).Maybe()
}

func setupTestRouter(repo Repository) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewHandler(repo)
	if mockRepo, ok := repo.(*MockRepository); ok {
		expectNoLifecycleChanges(mockRepo)
	}

	// Simula middleware de autenticacao injetando user_id
	authMiddleware := func(c *gin.Context) {
		c.Set(httpx.ContextKeyUserID, "user-123")
		c.Next()
	}

	v1 := router.Group("/api/v1")
	rooms := v1.Group("/rooms", authMiddleware)
	{
		rooms.POST("", handler.CreateRoom)
		rooms.GET("/public", handler.ListPublicRooms)
		rooms.GET("/:code_or_id", handler.GetRoom)
		rooms.POST("/:code_or_id/join", handler.JoinRoom)
		rooms.POST("/:code_or_id/leave", handler.LeaveRoom)
		rooms.GET("/:code_or_id/ws", handler.HandleWS)
	}
	return router
}

func TestHandler_CreateRoom(t *testing.T) {
	repo := new(MockRepository)
	router := setupTestRouter(repo)

	code := "ABC123"
	expectedRoom := &Room{
		ID:        "room-uuid",
		Code:      &code,
		CreatorID: "user-123",
		Status:    StatusWaiting,
		IsPrivate: false,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(5 * time.Minute),
	}

	repo.On("CreateRoom", mock.Anything, "user-123", false).Return(expectedRoom, nil)

	body, _ := json.Marshal(CreateRoomRequest{IsPrivate: false})
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/rooms", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var resp RoomResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Equal(t, "room-uuid", resp.ID)
	assert.Equal(t, code, *resp.Code)
	assert.Equal(t, "user-123", resp.CreatorID)
	assert.Nil(t, resp.OpponentID)
	assert.Equal(t, StatusWaiting, resp.Status)
	assert.False(t, resp.IsPrivate)

	repo.AssertExpectations(t)
}

func TestHandler_ListPublicRooms(t *testing.T) {
	repo := new(MockRepository)
	router := setupTestRouter(repo)

	code1, code2 := "COD111", "COD222"
	expectedRooms := []*Room{
		{
			ID:        "room-1",
			Code:      &code1,
			CreatorID: "user-abc",
			Status:    StatusWaiting,
			IsPrivate: false,
		},
		{
			ID:        "room-2",
			Code:      &code2,
			CreatorID: "user-xyz",
			Status:    StatusWaiting,
			IsPrivate: false,
		},
	}

	repo.On("ListPublicRooms", mock.Anything).Return(expectedRooms, nil)

	req, _ := http.NewRequest(http.MethodGet, "/api/v1/rooms/public", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp []RoomResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Len(t, resp, 2)
	assert.Equal(t, "room-1", resp[0].ID)
	assert.Equal(t, code1, *resp[0].Code)
	assert.Equal(t, "room-2", resp[1].ID)
	assert.Equal(t, code2, *resp[1].Code)

	repo.AssertExpectations(t)
}

func TestHandler_GetRoomByID(t *testing.T) {
	repo := new(MockRepository)
	router := setupTestRouter(repo)

	code := "XYZ999"
	expectedRoom := &Room{
		ID:        "room-uuid",
		Code:      &code,
		CreatorID: "user-creator",
		Status:    StatusWaiting,
		IsPrivate: false,
	}

	repo.On("GetRoomByID", mock.Anything, "room-uuid").Return(expectedRoom, nil)

	req, _ := http.NewRequest(http.MethodGet, "/api/v1/rooms/room-uuid", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp RoomResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Equal(t, "room-uuid", resp.ID)
	assert.Equal(t, code, *resp.Code)

	repo.AssertExpectations(t)
}

func TestHandler_GetRoomByCode(t *testing.T) {
	repo := new(MockRepository)
	router := setupTestRouter(repo)

	code := "XYZ999"
	expectedRoom := &Room{
		ID:        "room-uuid",
		Code:      &code,
		CreatorID: "user-creator",
		Status:    StatusWaiting,
		IsPrivate: false,
	}

	repo.On("GetRoomByCode", mock.Anything, "XYZ999").Return(expectedRoom, nil)

	req, _ := http.NewRequest(http.MethodGet, "/api/v1/rooms/XYZ999", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp RoomResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Equal(t, "room-uuid", resp.ID)
	assert.Equal(t, "XYZ999", *resp.Code)

	repo.AssertExpectations(t)
}

func TestHandler_GetExpiredRoomReturnsGone(t *testing.T) {
	repo := new(MockRepository)
	router := setupTestRouter(repo)

	code := "XYZ999"
	expectedRoom := &Room{
		ID:        "room-uuid",
		Code:      &code,
		CreatorID: "user-creator",
		Status:    StatusExpired,
		IsPrivate: false,
	}

	repo.On("GetRoomByID", mock.Anything, "room-uuid").Return(expectedRoom, nil)

	req, _ := http.NewRequest(http.MethodGet, "/api/v1/rooms/room-uuid", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusGone, w.Code)

	repo.AssertExpectations(t)
}

func TestHandler_JoinRoom(t *testing.T) {
	repo := new(MockRepository)
	router := setupTestRouter(repo)

	code := "XYZ999"
	roomBeforeJoin := &Room{
		ID:        "room-uuid",
		Code:      &code,
		CreatorID: "user-creator",
		Status:    StatusWaiting,
		IsPrivate: false,
	}

	opponentID := "user-123"
	roomAfterJoin := &Room{
		ID:         "room-uuid",
		Code:       &code,
		CreatorID:  "user-creator",
		OpponentID: &opponentID,
		Status:     StatusWaiting,
		IsPrivate:  false,
	}

	repo.On("GetRoomByID", mock.Anything, "room-uuid").Return(roomBeforeJoin, nil)
	repo.On("JoinRoom", mock.Anything, "room-uuid", "user-123").Return(roomAfterJoin, nil)

	req, _ := http.NewRequest(http.MethodPost, "/api/v1/rooms/room-uuid/join", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp RoomResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Equal(t, "room-uuid", resp.ID)
	assert.Equal(t, "user-123", *resp.OpponentID)

	repo.AssertExpectations(t)
}

func TestHandler_JoinRoomRemovesUserFromSpectators(t *testing.T) {
	repo := new(MockRepository)
	handler := NewHandler(repo)
	expectNoLifecycleChanges(repo)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/v1/rooms/:code_or_id/join", func(c *gin.Context) {
		c.Set(httpx.ContextKeyUserID, "user-123")
		c.Next()
	}, handler.JoinRoom)

	code := "XYZ999"
	roomBeforeJoin := &Room{
		ID:        "room-uuid",
		Code:      &code,
		CreatorID: "user-creator",
		Status:    StatusWaiting,
		IsPrivate: false,
	}

	opponentID := "user-123"
	roomAfterJoin := &Room{
		ID:         "room-uuid",
		Code:       &code,
		CreatorID:  "user-creator",
		OpponentID: &opponentID,
		Status:     StatusWaiting,
		IsPrivate:  false,
	}

	handler.presence.RegisterRoomSpectator("room-uuid", "user-123", "conn-1")
	handler.presence.RegisterRoomSpectator("room-uuid", "user-123", "conn-2")
	handler.presence.RegisterRoomSpectator("room-uuid", "user-other", "conn-3")

	repo.On("GetRoomByID", mock.Anything, "room-uuid").Return(roomBeforeJoin, nil)
	repo.On("JoinRoom", mock.Anything, "room-uuid", "user-123").Return(roomAfterJoin, nil)

	req, _ := http.NewRequest(http.MethodPost, "/api/v1/rooms/room-uuid/join", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	snapshot := handler.presence.RoomSpectatorsSnapshot("room-uuid")
	assert.Equal(t, 1, snapshot.Count)
	assert.ElementsMatch(t, []PresenceUser{{UserID: "user-other"}}, snapshot.Spectators)

	repo.AssertExpectations(t)
}

func TestHandler_ListPublicRoomsAfterOpponentDisconnectCleanup(t *testing.T) {
	repo := new(MockRepository)
	router := setupTestRouter(repo)
	repo.ExpectedCalls = nil

	code := "XYZ999"
	releasedOpponentID := "user-opponent"
	releasedRoom := &Room{
		ID:        "room-uuid",
		Code:      &code,
		CreatorID: "user-creator",
		Status:    StatusWaiting,
		IsPrivate: false,
	}
	expectedRooms := []*Room{releasedRoom}

	repo.On("ExpireRooms", mock.Anything).Return([]string{}, nil).Once()
	repo.On("CloseRoomsWithExpiredCreatorDisconnect", mock.Anything, mock.Anything).Return([]string{}, nil).Once()
	repo.On("ReleaseRoomsWithExpiredOpponentDisconnect", mock.Anything, mock.Anything).Return([]ReleasedOpponent{
		{Room: releasedRoom, OpponentID: releasedOpponentID},
	}, nil).Once()
	repo.On("ListPublicRooms", mock.Anything).Return(expectedRooms, nil)

	req, _ := http.NewRequest(http.MethodGet, "/api/v1/rooms/public", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp []RoomResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Len(t, resp, 1)
	assert.Nil(t, resp[0].OpponentID)
	assert.Equal(t, StatusWaiting, resp[0].Status)

	repo.AssertExpectations(t)
}

func TestHandler_LeaveRoomCreatorClosesRoom(t *testing.T) {
	repo := new(MockRepository)
	router := setupTestRouter(repo)

	code := "XYZ999"
	roomBeforeLeave := &Room{
		ID:        "room-uuid",
		Code:      &code,
		CreatorID: "user-123",
		Status:    StatusWaiting,
		IsPrivate: false,
	}
	roomAfterLeave := &Room{
		ID:        "room-uuid",
		Code:      &code,
		CreatorID: "user-123",
		Status:    StatusExpired,
		IsPrivate: false,
	}

	repo.On("GetRoomByID", mock.Anything, "room-uuid").Return(roomBeforeLeave, nil)
	repo.On("LeaveRoom", mock.Anything, "room-uuid", "user-123").Return(roomAfterLeave, LeaveRoleCreator, nil)

	req, _ := http.NewRequest(http.MethodPost, "/api/v1/rooms/room-uuid/leave", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp RoomResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Equal(t, StatusExpired, resp.Status)

	repo.AssertExpectations(t)
}

func TestHandler_LeaveRoomSpectatorKeepsRoomAvailable(t *testing.T) {
	repo := new(MockRepository)
	router := setupTestRouter(repo)

	code := "XYZ999"
	roomBeforeLeave := &Room{
		ID:        "room-uuid",
		Code:      &code,
		CreatorID: "user-creator",
		Status:    StatusWaiting,
		IsPrivate: false,
	}
	roomAfterLeave := &Room{
		ID:        "room-uuid",
		Code:      &code,
		CreatorID: "user-creator",
		Status:    StatusWaiting,
		IsPrivate: false,
	}

	repo.On("GetRoomByID", mock.Anything, "room-uuid").Return(roomBeforeLeave, nil)
	repo.On("LeaveRoom", mock.Anything, "room-uuid", "user-123").Return(roomAfterLeave, LeaveRoleSpectator, nil)

	req, _ := http.NewRequest(http.MethodPost, "/api/v1/rooms/room-uuid/leave", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp RoomResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Nil(t, resp.OpponentID)
	assert.Equal(t, StatusWaiting, resp.Status)

	repo.AssertExpectations(t)
}

func TestHandler_LeaveRoomOpponentReleasesSlot(t *testing.T) {
	repo := new(MockRepository)
	router := setupTestRouter(repo)

	code := "XYZ999"
	opponentID := "user-123"
	roomBeforeLeave := &Room{
		ID:         "room-uuid",
		Code:       &code,
		CreatorID:  "user-creator",
		OpponentID: &opponentID,
		Status:     StatusWaiting,
		IsPrivate:  false,
	}
	roomAfterLeave := &Room{
		ID:        "room-uuid",
		Code:      &code,
		CreatorID: "user-creator",
		Status:    StatusWaiting,
		IsPrivate: false,
	}

	repo.On("GetRoomByID", mock.Anything, "room-uuid").Return(roomBeforeLeave, nil)
	repo.On("LeaveRoom", mock.Anything, "room-uuid", "user-123").Return(roomAfterLeave, LeaveRoleOpponent, nil)

	req, _ := http.NewRequest(http.MethodPost, "/api/v1/rooms/room-uuid/leave", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp RoomResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Nil(t, resp.OpponentID)
	assert.Equal(t, StatusWaiting, resp.Status)

	repo.AssertExpectations(t)
}

func TestHandler_HandleWS_NatsUnavailable(t *testing.T) {
	repo := new(MockRepository)
	router := setupTestRouter(repo)

	code := "XYZ999"
	expectedRoom := &Room{
		ID:        "room-uuid",
		Code:      &code,
		CreatorID: "user-123",
		Status:    StatusWaiting,
		IsPrivate: false,
	}

	repo.On("GetRoomByID", mock.Anything, "room-uuid").Return(expectedRoom, nil)

	req, _ := http.NewRequest(http.MethodGet, "/api/v1/rooms/room-uuid/ws", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var resp httpx.ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Equal(t, httpx.ErrCodeInternal, resp.Error.Code)
}
