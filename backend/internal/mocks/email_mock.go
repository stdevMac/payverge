package mocks

import (
	"sync"

	"github.com/stretchr/testify/mock"
)

// EmailMessage represents an email that was sent
type EmailMessage struct {
	To          string
	Subject     string
	Body        string
	ContentType string
	Timestamp   int64
}

// MockEmailService provides a mock implementation for email operations
type MockEmailService struct {
	mock.Mock
	sentEmails []EmailMessage
	mu         sync.RWMutex
}

// NewMockEmailService creates a new mock email service
func NewMockEmailService() *MockEmailService {
	return &MockEmailService{
		sentEmails: make([]EmailMessage, 0),
	}
}

// SendEmail mocks sending an email
func (m *MockEmailService) SendEmail(to, subject, body, contentType string) error {
	args := m.Called(to, subject, body, contentType)
	
	if args.Error(0) != nil {
		return args.Error(0)
	}
	
	m.mu.Lock()
	defer m.mu.Unlock()
	
	email := EmailMessage{
		To:          to,
		Subject:     subject,
		Body:        body,
		ContentType: contentType,
		Timestamp:   args.Get(1).(int64),
	}
	
	m.sentEmails = append(m.sentEmails, email)
	return nil
}

// GetSentEmails returns all sent emails
func (m *MockEmailService) GetSentEmails() []EmailMessage {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	emails := make([]EmailMessage, len(m.sentEmails))
	copy(emails, m.sentEmails)
	return emails
}

// GetEmailCount returns the number of sent emails
func (m *MockEmailService) GetEmailCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sentEmails)
}

// ClearSentEmails removes all sent emails from history
func (m *MockEmailService) ClearSentEmails() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sentEmails = make([]EmailMessage, 0)
}

// FindEmailsByRecipient finds emails sent to a specific recipient
func (m *MockEmailService) FindEmailsByRecipient(to string) []EmailMessage {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	var result []EmailMessage
	for _, email := range m.sentEmails {
		if email.To == to {
			result = append(result, email)
		}
	}
	return result
}

// FindEmailsBySubject finds emails with a specific subject
func (m *MockEmailService) FindEmailsBySubject(subject string) []EmailMessage {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	var result []EmailMessage
	for _, email := range m.sentEmails {
		if email.Subject == subject {
			result = append(result, email)
		}
	}
	return result
}
