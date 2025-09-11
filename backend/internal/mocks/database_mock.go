package mocks

import (
	"errors"
	"sync"

	"github.com/stretchr/testify/mock"
	"payverge/internal/structs"
)

// MockDatabase provides a mock implementation for database operations
type MockDatabase struct {
	mock.Mock
	users map[string]*structs.User
	mu    sync.RWMutex
}

// NewMockDatabase creates a new mock database instance
func NewMockDatabase() *MockDatabase {
	return &MockDatabase{
		users: make(map[string]*structs.User),
	}
}

// GetUserByAddress mocks getting a user by address
func (m *MockDatabase) GetUserByAddress(address string) (*structs.User, error) {
	args := m.Called(address)
	
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	if user, exists := m.users[address]; exists {
		return user, nil
	}
	
	return nil, args.Error(1)
}

// RegisterUser mocks user registration
func (m *MockDatabase) RegisterUser(user *structs.User) error {
	args := m.Called(user)
	
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if args.Error(0) != nil {
		return args.Error(0)
	}
	
	m.users[user.Address] = user
	return nil
}

// UpdateUser mocks user updates
func (m *MockDatabase) UpdateUser(user *structs.User) error {
	args := m.Called(user)
	
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if args.Error(0) != nil {
		return args.Error(0)
	}
	
	if _, exists := m.users[user.Address]; !exists {
		return errors.New("user not found")
	}
	
	m.users[user.Address] = user
	return nil
}

// GetAllUsers mocks getting all users
func (m *MockDatabase) GetAllUsers() ([]*structs.User, error) {
	args := m.Called()
	
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	if args.Error(1) != nil {
		return nil, args.Error(1)
	}
	
	users := make([]*structs.User, 0, len(m.users))
	for _, user := range m.users {
		users = append(users, user)
	}
	
	return users, nil
}

// DeleteUser mocks user deletion
func (m *MockDatabase) DeleteUser(address string) error {
	args := m.Called(address)
	
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if args.Error(0) != nil {
		return args.Error(0)
	}
	
	delete(m.users, address)
	return nil
}

// SetupTestUser adds a test user to the mock database
func (m *MockDatabase) SetupTestUser(user *structs.User) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.users[user.Address] = user
}

// ClearUsers removes all users from the mock database
func (m *MockDatabase) ClearUsers() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.users = make(map[string]*structs.User)
}

// GetUserCount returns the number of users in the mock database
func (m *MockDatabase) GetUserCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.users)
}
