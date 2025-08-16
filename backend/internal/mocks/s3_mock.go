package mocks

import (
	"bytes"
	"io"
	"sync"

	"github.com/stretchr/testify/mock"
)

// MockS3Service provides a mock implementation for S3 operations
type MockS3Service struct {
	mock.Mock
	files map[string][]byte
	mu    sync.RWMutex
}

// NewMockS3Service creates a new mock S3 service
func NewMockS3Service() *MockS3Service {
	return &MockS3Service{
		files: make(map[string][]byte),
	}
}

// UploadFile mocks file upload to S3
func (m *MockS3Service) UploadFile(key string, data io.Reader, contentType string) (string, error) {
	args := m.Called(key, data, contentType)
	
	if args.Error(1) != nil {
		return "", args.Error(1)
	}
	
	// Read data and store in memory
	buf := new(bytes.Buffer)
	buf.ReadFrom(data)
	
	m.mu.Lock()
	defer m.mu.Unlock()
	m.files[key] = buf.Bytes()
	
	return args.String(0), nil
}

// DeleteFile mocks file deletion from S3
func (m *MockS3Service) DeleteFile(key string) error {
	args := m.Called(key)
	
	if args.Error(0) != nil {
		return args.Error(0)
	}
	
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.files, key)
	
	return nil
}

// GetFile mocks file retrieval from S3
func (m *MockS3Service) GetFile(key string) ([]byte, error) {
	args := m.Called(key)
	
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	if args.Error(1) != nil {
		return nil, args.Error(1)
	}
	
	if data, exists := m.files[key]; exists {
		return data, nil
	}
	
	return args.Get(0).([]byte), nil
}

// FileExists checks if a file exists in the mock S3
func (m *MockS3Service) FileExists(key string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, exists := m.files[key]
	return exists
}

// GetFileCount returns the number of files in mock S3
func (m *MockS3Service) GetFileCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.files)
}

// ClearFiles removes all files from mock S3
func (m *MockS3Service) ClearFiles() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.files = make(map[string][]byte)
}
