package storage

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockRelationalStorage 模拟关系型存储
type MockRelationalStorage struct {
	mock.Mock
}

func (m *MockRelationalStorage) Exec(ctx context.Context, sql string, args ...interface{}) (Result, error) {
	argsMock := m.Called(ctx, sql, args)
	return argsMock.Get(0).(Result), argsMock.Error(1)
}

func (m *MockRelationalStorage) Query(ctx context.Context, sql string, args ...interface{}) (Rows, error) {
	argsMock := m.Called(ctx, sql, args)
	return argsMock.Get(0).(Rows), argsMock.Error(1)
}

func (m *MockRelationalStorage) QueryRow(ctx context.Context, sql string, args ...interface{}) Row {
	argsMock := m.Called(ctx, sql, args)
	return argsMock.Get(0).(Row)
}

func (m *MockRelationalStorage) BeginTx(ctx context.Context) (Tx, error) {
	argsMock := m.Called(ctx)
	return argsMock.Get(0).(Tx), argsMock.Error(1)
}

func (m *MockRelationalStorage) Ping(ctx context.Context) error {
	return m.Called(ctx).Error(0)
}

func (m *MockRelationalStorage) Close() error {
	return m.Called().Error(0)
}

func (m *MockRelationalStorage) RawDB() interface{} {
	return nil
}

func TestDualWriteMode_String(t *testing.T) {
	tests := []struct {
		mode DualWriteMode
		want string
	}{
		{ModeOldOnly, "ModeOldOnly"},
		{ModeAsyncWrite, "ModeAsyncWrite"},
		{ModeSyncWrite, "ModeSyncWrite"},
		{ModeReadSplit, "ModeReadSplit"},
		{ModeNewOnly, "ModeNewOnly"},
		{DualWriteMode(999), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.mode.String())
		})
	}
}

func TestDualWriteRelationalStorage_ModeOldOnly(t *testing.T) {
	ctx := context.Background()
	primary := &MockRelationalStorage{}
	secondary := &MockRelationalStorage{}

	storage := &DualWriteRelationalStorage{
		primary:   primary,
		secondary: secondary,
		mode:      ModeOldOnly,
	}

	primary.On("Exec", ctx, "SELECT 1", mock.Anything).Return(&sqlResult{}, nil)

	_, err := storage.Exec(ctx, "SELECT 1")
	assert.NoError(t, err)

	primary.AssertExpectations(t)
	secondary.AssertNotCalled(t, "Exec")
}

func TestDualWriteRelationalStorage_ModeNewOnly(t *testing.T) {
	ctx := context.Background()
	primary := &MockRelationalStorage{}
	secondary := &MockRelationalStorage{}

	storage := &DualWriteRelationalStorage{
		primary:   primary,
		secondary: secondary,
		mode:      ModeNewOnly,
	}

	secondary.On("Exec", ctx, "SELECT 1", mock.Anything).Return(&sqlResult{}, nil)

	_, err := storage.Exec(ctx, "SELECT 1")
	assert.NoError(t, err)

	secondary.AssertExpectations(t)
	primary.AssertNotCalled(t, "Exec")
}

func TestDualWriteRelationalStorage_ModeSyncWrite(t *testing.T) {
	ctx := context.Background()
	primary := &MockRelationalStorage{}
	secondary := &MockRelationalStorage{}

	storage := &DualWriteRelationalStorage{
		primary:   primary,
		secondary: secondary,
		mode:      ModeSyncWrite,
	}

	primary.On("Exec", ctx, "SELECT 1", mock.Anything).Return(&sqlResult{}, nil)
	secondary.On("Exec", ctx, "SELECT 1", mock.Anything).Return(&sqlResult{}, nil)

	_, err := storage.Exec(ctx, "SELECT 1")
	assert.NoError(t, err)

	primary.AssertExpectations(t)
	secondary.AssertExpectations(t)
}

func TestIsNotFound(t *testing.T) {
	assert.True(t, IsNotFound(ErrNotFound))
	assert.False(t, IsNotFound(nil))
	assert.False(t, IsNotFound(assert.AnError))
}
