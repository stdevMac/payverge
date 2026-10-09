package auth

import (
	"time"
)

// AuthProvider represents the authentication provider used
type AuthProvider string

const (
	AuthProviderEmail  AuthProvider = "email"
	AuthProviderGoogle AuthProvider = "google"
	AuthProviderWallet AuthProvider = "wallet"
)

// UserAuth represents authentication data for a user
type UserAuth struct {
	ID       uint   `gorm:"primaryKey" json:"id"`
	UserID   uint   `gorm:"index;not null" json:"user_id"`
	Provider string `gorm:"not null" json:"provider"` // email, google, wallet

	// Email/Password authentication
	PasswordHash string `json:"-"` // Never expose password hash

	// OAuth authentication. Provider access and refresh tokens are not stored:
	// login uses the in-memory token from the callback exchange.
	ProviderUserID string `json:"provider_user_id"` // Google user ID

	// Wallet authentication (optional linking)
	WalletAddress string `gorm:"index" json:"wallet_address"`

	// Verification
	EmailVerified      bool       `gorm:"default:false" json:"email_verified"`
	VerificationToken  string     `json:"-"`
	VerificationExpiry *time.Time `json:"-"`

	// Password reset
	ResetToken  string     `json:"-"`
	ResetExpiry *time.Time `json:"-"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// RegisterRequest represents a registration request
type RegisterRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
	Name     string `json:"name" binding:"required"`
	// InviteCode is a one-time launch-cohort credential. Production route
	// wiring installs the durable admission service; plaintext is never stored.
	InviteCode string `json:"invite_code"`
	// Language is the operator locale active in the signup UI. Optional;
	// validated against the operator locale registry (en / es / es-AR). It
	// seeds User.LanguageSelected so the verification email — the very first
	// touchpoint — localizes like every later lifecycle email.
	Language string `json:"language"`
	// SignupSource is optional attribution (organic, concierge, utm campaign…).
	// Truncated server-side; empty is fine.
	SignupSource string `json:"signup_source"`
}

// LoginRequest represents a login request
type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// OAuthCallbackRequest represents an OAuth callback
type OAuthCallbackRequest struct {
	Code     string `json:"code" binding:"required"`
	State    string `json:"state"`
	Provider string `json:"provider" binding:"required"`
}

// LinkWalletRequest represents a request to link a wallet
type LinkWalletRequest struct {
	Address   string `json:"address" binding:"required"`
	Message   string `json:"message" binding:"required"`
	Signature string `json:"signature" binding:"required"`
}

// AuthResponse represents the response after successful authentication
type AuthResponse struct {
	Success              bool   `json:"success"`
	Token                string `json:"token"`
	UserID               uint   `json:"user_id"`
	Email                string `json:"email"`
	Name                 string `json:"name"`
	WalletLinked         bool   `json:"wallet_linked"`
	WalletAddress        string `json:"wallet_address,omitempty"`
	Provider             string `json:"provider"`
	IsNewUser            bool   `json:"is_new_user"`
	Message              string `json:"message,omitempty"`
	VerificationRequired bool   `json:"verification_required,omitempty"`
	VerificationURL      string `json:"verification_url,omitempty"`
}

// PasswordResetRequest represents a password reset request
type PasswordResetRequest struct {
	Email string `json:"email" binding:"required,email"`
}

// PasswordResetConfirm represents password reset confirmation
type PasswordResetConfirm struct {
	Token       string `json:"token" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=8"`
}

// VerifyEmailRequest represents email verification
type VerifyEmailRequest struct {
	Token string `json:"token" binding:"required"`
}

// GoogleUserInfo represents user info from Google OAuth
type GoogleUserInfo struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	VerifiedEmail bool   `json:"verified_email"`
	Name          string `json:"name"`
	GivenName     string `json:"given_name"`
	FamilyName    string `json:"family_name"`
	Picture       string `json:"picture"`
}

func (UserAuth) TableName() string {
	return "user_auths"
}
