package server

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/structs"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var errLaunchIdentityAlreadyExists = errors.New("launch identity already exists")

var (
	ErrLaunchInviteRequired       = errors.New("a valid launch invite is required")
	ErrLaunchInviteExpired        = errors.New("launch invite is expired or inactive")
	ErrLaunchCohortFull           = errors.New("launch cohort is full")
	ErrLaunchInviteAlreadyClaimed = errors.New("identity already claimed a launch invite")
	ErrArchivedWalletIdentity     = errors.New("this account is scheduled for deletion")
	// ErrRegistrationClosed: REGISTRATION_MODE=closed refuses every new
	// identity; existing identities still sign in.
	ErrRegistrationClosed = errors.New("registration is closed on this instance")
)

type launchIdentityAdmissionFunc func(context.Context, string, string, func(*gorm.DB) error) error

var launchIdentityAdmission launchIdentityAdmissionFunc = func(context.Context, string, string, func(*gorm.DB) error) error {
	return ErrLaunchInviteRequired
}

// SetLaunchIdentityAdmission wires the durable runtime-control admission
// service without coupling this legacy auth package back to runtimecontrol.
// Nil restores the fail-closed default.
func SetLaunchIdentityAdmission(admit launchIdentityAdmissionFunc) {
	if admit == nil {
		launchIdentityAdmission = func(context.Context, string, string, func(*gorm.DB) error) error {
			return ErrLaunchInviteRequired
		}
		return
	}
	launchIdentityAdmission = admit
}

func walletInviteIdentity(address string) string {
	return "wallet:" + strings.ToLower(strings.TrimSpace(address))
}

func databaseUserToStruct(user database.User) structs.User {
	return structs.User{
		Address:                 user.Address,
		Email:                   user.Email,
		Name:                    user.Name,
		AuthMethod:              user.AuthMethod,
		Role:                    structs.Role(user.Role),
		NotificationPreferences: user.NotificationPreferences,
		LanguageSelected:        user.LanguageSelected,
	}
}

// getOrCreateWalletUserWithInvite preserves sign-in for an existing SIWE
// identity but makes first-time identity creation consume one durable cohort
// invitation atomically with the user row.
func getOrCreateWalletUserWithInvite(ctx context.Context, address string, role structs.Role, inviteCode string) (structs.User, bool, error) {
	address = strings.ToLower(strings.TrimSpace(address))
	var existingRecord database.User
	lookupErr := database.GetDB().Where("address = ?", address).First(&existingRecord).Error
	if lookupErr == nil {
		if existingRecord.DeletedAt != nil {
			return structs.User{}, false, ErrArchivedWalletIdentity
		}
		return databaseUserToStruct(existingRecord), false, nil
	}
	if !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
		return structs.User{}, false, lookupErr
	}

	var created database.User
	err := launchIdentityAdmission(
		ctx,
		walletInviteIdentity(address),
		inviteCode,
		func(tx *gorm.DB) error {
			var existing database.User
			err := tx.Where("address = ?", address).First(&existing).Error
			if err == nil {
				return errLaunchIdentityAlreadyExists
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			created = database.User{Address: address, Role: string(role)}
			return tx.Create(&created).Error
		},
	)
	if errors.Is(err, errLaunchIdentityAlreadyExists) {
		var raced database.User
		if lookupErr := database.GetDB().Where("address = ?", address).First(&raced).Error; lookupErr != nil {
			return structs.User{}, false, lookupErr
		}
		if raced.DeletedAt != nil {
			return structs.User{}, false, ErrArchivedWalletIdentity
		}
		return databaseUserToStruct(raced), false, nil
	}
	if err != nil {
		return structs.User{}, false, err
	}
	return databaseUserToStruct(created), true, nil
}

func respondLaunchInviteAdmissionError(c *gin.Context, err error) bool {
	switch {
	case errors.Is(err, ErrArchivedWalletIdentity):
		RespondWithError(c, http.StatusForbidden, ErrCodeForbidden,
			"This account is scheduled for deletion. Contact support to restore it.")
		return true
	case errors.Is(err, ErrRegistrationClosed):
		RespondWithErrorParams(c, http.StatusForbidden, ErrCodeForbidden,
			"Registration is closed on this instance", map[string]interface{}{"reason": "registration_closed"})
		return true
	case errors.Is(err, ErrLaunchInviteRequired), errors.Is(err, ErrLaunchInviteExpired):
		RespondWithError(c, http.StatusForbidden, ErrCodeForbidden, "A valid launch invitation is required")
		return true
	case errors.Is(err, ErrLaunchCohortFull), errors.Is(err, ErrLaunchInviteAlreadyClaimed):
		RespondWithError(c, http.StatusConflict, ErrCodeConflict, "This launch invitation is no longer available")
		return true
	default:
		return false
	}
}
