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

func (m *MockRepository) ExpireRooms(ctx context.Context) (int64, error) {
	args := m.Called(ctx)
	return int64(args.Int(0)), args.Error(1)
}

func (m *MockRepository) CloseRoom(ctx context.Context, roomID string) error {
	args := m.Called(ctx, roomID)
	return args.Error(0)
}

func setupTestRouter(repo Repository) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewHandler(repo)

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
		rooms.GET("/:code_or_id/ws", handler.HandleWS)
	}
	return router
}

func TestHandler_CreateRoom(t *testing.T) {
	repo := new(MockRepository)
	router := setupTestRouter(repo)

	code := "ABC123"
	expectedRoom := &Room{
		ID:         "room-uuid",
		Code:       &code,
		CreatorID:  "user-123",
		Status:     StatusWaiting,
		IsPrivate:  false,
		CreatedAt:  time.Now(),
		ExpiresAt:  time.Now().Add(5 * time.Minute),
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
