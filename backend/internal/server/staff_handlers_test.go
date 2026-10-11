package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/emails"
	"github.com/stdevmac/payverge/backend/internal/session"
	"github.com/stdevmac/payverge/backend/internal/structs"
)

func setupStaffHandlerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	database.SetTestDB(gormDB)
	structs.SecretKey = []byte("test-secret-key-for-testing-purposes")
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.Staff{},
		&database.StaffIdentity{},
		&database.StaffMembership{},
		&database.StaffPermissionDeny{},
		&database.StaffInvitation{},
		&database.StaffLoginCode{},
		&database.StaffMembershipSelectionToken{},
		&database.RBACAuditLog{},
		&session.UserSession{},
		&database.Plugin{},
		&database.BusinessPlugin{},
	))
	require.NoError(t, gormDB.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_staff_business_email_lower ON staff (business_id, LOWER(TRIM(email)))`).Error)
	require.NoError(t, gormDB.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_staff_invitations_pending_email ON staff_invitations (business_id, LOWER(TRIM(email))) WHERE status = 'pending'`).Error)
	session.GlobalStore = session.NewStore(gormDB)
	InitializeRBAC(database.GetDBWrapper())

	return gormDB
}

func createOwnedBusiness(t *testing.T, ownerAddress, name string) *database.Business {
	t.Helper()

	business := &database.Business{
		BusinessId:     fmt.Sprintf("biz-%s", name),
		Name:           name,
		OwnerAddress:   ownerAddress,
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
	}
	require.NoError(t, database.GetDB().Create(business).Error)
	return business
}

func createStaffMember(t *testing.T, businessID uint, email, name string) *database.Staff {
	t.Helper()

	staff := &database.Staff{
		BusinessID: businessID,
		Email:      email,
		Name:       name,
		Role:       database.StaffRoleManager,
		InvitedBy:  "owner@example.com",
		IsActive:   true,
	}
	require.NoError(t, database.GetDB().Create(staff).Error)
	return staff
}

func createStaffInvitation(t *testing.T, businessID uint, email, name string, role database.StaffRole) *database.StaffInvitation {
	t.Helper()

	invitation := &database.StaffInvitation{
		BusinessID: businessID,
		Email:      email,
		Name:       name,
		Role:       role,
		Token:      fmt.Sprintf("token-%d-%s", businessID, strings.ReplaceAll(email, "@", "-at-")),
		Status:     database.InvitationStatusPending,
		InvitedBy:  "owner@example.com",
		ExpiresAt:  time.Now().Add(24 * time.Hour),
	}
	require.NoError(t, database.GetDB().Create(invitation).Error)
	return invitation
}

func TestRemoveStaff_RejectsCrossBusinessDeletion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)

	ownerBusiness := createOwnedBusiness(t, "0xOwnerA", "Owner A")
	otherBusiness := createOwnedBusiness(t, "0xOwnerB", "Owner B")
	staff := createStaffMember(t, otherBusiness.ID, "staff@example.com", "Cross Biz")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", ownerBusiness.ID)},
		{Key: "staffId", Value: fmt.Sprintf("%d", staff.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodDelete, "/", nil)
	c.Set("address", "0xOwnerA")

	RemoveStaff(c)

	assert.Equal(t, http.StatusForbidden, w.Code)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "Staff member does not belong to this business", resp["error"])

	var persisted database.Staff
	require.NoError(t, database.GetDB().First(&persisted, staff.ID).Error)
	assert.True(t, persisted.IsActive)
}

func TestRemoveStaff_SoftDeletesScopedStaffMember(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)

	business := createOwnedBusiness(t, "0xOwnerA", "Owner A")
	staff := createStaffMember(t, business.ID, "staff@example.com", "Scoped Staff")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
		{Key: "staffId", Value: fmt.Sprintf("%d", staff.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodDelete, "/", nil)
	c.Set("address", "0xOwnerA")

	RemoveStaff(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var persisted database.Staff
	require.NoError(t, database.GetDB().First(&persisted, staff.ID).Error)
	assert.False(t, persisted.IsActive)
}

func TestAcceptInvitation_ReturnsBusinessContextAndSetsCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)

	business := createOwnedBusiness(t, "0xOwnerA", "Invite Biz")
	invitation := createStaffInvitation(t, business.ID, "invitee@example.com", "Invitee", database.StaffRoleHost)

	body, err := json.Marshal(map[string]string{
		"token": invitation.Token,
		"name":  "Accepted Staff",
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	AcceptInvitation(c)

	assert.Equal(t, http.StatusCreated, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "Invite Biz", resp["business_name"])
	assert.Equal(t, string(database.StaffRoleHost), resp["role"])

	var foundStaffCookie bool
	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == "staff_token" && cookie.Value != "" {
			foundStaffCookie = true
			break
		}
	}
	assert.True(t, foundStaffCookie)
}

func TestGetStaffInvitationPreview_ReturnsBusinessContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)

	business := createOwnedBusiness(t, "0xOwnerA", "Preview Biz")
	business.CustomURL = "preview-biz"
	require.NoError(t, database.GetDB().Save(business).Error)
	invitation := createStaffInvitation(t, business.ID, "preview@example.com", "Preview Person", database.StaffRoleServer)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/?token=%s", invitation.Token), nil)

	GetStaffInvitationPreview(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, invitation.Email, resp["email"])
	assert.Equal(t, string(invitation.Role), resp["role"])
	assert.Equal(t, business.Name, resp["business_name"])
	assert.Equal(t, business.CustomURL, resp["business_custom_url"])
	assert.Equal(t, business.BusinessId, resp["business_slug"])
}

func TestAcceptInvitation_RejectsExistingStaffEmailGracefully(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)

	business := createOwnedBusiness(t, "0xOwnerA", "Duplicate Staff Biz")
	invitation := createStaffInvitation(t, business.ID, "duplicate@example.com", "Invitee", database.StaffRoleHost)
	createStaffMember(t, business.ID, invitation.Email, "Existing Staff")

	body, err := json.Marshal(map[string]string{
		"token": invitation.Token,
		"name":  "Accepted Staff",
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	AcceptInvitation(c)

	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Contains(t, w.Body.String(), "already exists")
}

func TestAcceptInvitation_ReactivatesRemovedStaffAndBumpsVersion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gdb := setupStaffHandlerTestDB(t)
	business := createOwnedBusiness(t, "0xRehireOwner", "Rehire Biz")
	staff := createStaffMember(t, business.ID, "rehire@example.com", "Old Name")
	require.NoError(t, gdb.Model(staff).Updates(map[string]any{"is_active": false, "authz_version": 6}).Error)
	inviteBody, err := json.Marshal(map[string]string{"email": "REHIRE@example.com", "name": "New Name", "role": string(database.StaffRoleHost)})
	require.NoError(t, err)
	inviteRecorder := httptest.NewRecorder()
	inviteContext, _ := gin.CreateTestContext(inviteRecorder)
	inviteContext.Params = gin.Params{{Key: "id", Value: fmt.Sprint(business.ID)}}
	inviteContext.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(inviteBody))
	inviteContext.Request.Header.Set("Content-Type", "application/json")
	inviteContext.Set("address", business.OwnerAddress)
	InviteStaff(inviteContext)
	require.Equal(t, http.StatusCreated, inviteRecorder.Code, inviteRecorder.Body.String())
	var invitation database.StaffInvitation
	require.NoError(t, gdb.Where("business_id = ? AND LOWER(TRIM(email)) = ? AND status = ?", business.ID, "rehire@example.com", database.InvitationStatusPending).Take(&invitation).Error)

	body, err := json.Marshal(map[string]string{"token": invitation.Token, "name": "Rehired Name"})
	require.NoError(t, err)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	AcceptInvitation(c)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var rows []database.Staff
	require.NoError(t, gdb.Where("business_id = ? AND LOWER(TRIM(email)) = ?", business.ID, "rehire@example.com").Find(&rows).Error)
	require.Len(t, rows, 1, "rehire must reactivate the legacy row instead of creating a second membership")
	assert.Equal(t, staff.ID, rows[0].ID)
	assert.True(t, rows[0].IsActive)
	assert.Equal(t, database.StaffRoleHost, rows[0].Role)
	assert.Equal(t, 7, rows[0].AuthzVersion)

	var audit database.RBACAuditLog
	require.NoError(t, gdb.Where("staff_id = ? AND action = ?", staff.ID, database.RBACActionStaffReactivated).Take(&audit).Error)
}

func TestAcceptInvitation_AllowsOneIdentityAcrossBusinesses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gdb := setupStaffHandlerTestDB(t)
	one := createOwnedBusiness(t, "0xMultiA", "Multi A")
	two := createOwnedBusiness(t, "0xMultiB", "Multi B")
	first := createStaffMember(t, one.ID, "multi@example.com", "Multi")
	first.Role = database.StaffRoleServer
	require.NoError(t, database.GetDBWrapper().StaffService.Update(first))
	invitation := createStaffInvitation(t, two.ID, "MULTI@example.com", "Multi", database.StaffRoleManager)

	body, err := json.Marshal(map[string]string{"token": invitation.Token, "name": "Multi"})
	require.NoError(t, err)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	AcceptInvitation(c)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var identities int64
	require.NoError(t, gdb.Model(&database.StaffIdentity{}).Where("normalized_email = ?", "multi@example.com").Count(&identities).Error)
	assert.Equal(t, int64(1), identities)
	var memberships []database.StaffMembership
	require.NoError(t, gdb.Order("business_id").Find(&memberships).Error)
	require.Len(t, memberships, 2)
	assert.Equal(t, database.StaffRoleServer, memberships[0].Role)
	assert.Equal(t, database.StaffRoleManager, memberships[1].Role)
}

func TestVerifyLoginCode_AuthenticatesIdentityThenSelectsScopedMembership(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gdb := setupStaffHandlerTestDB(t)
	one := createOwnedBusiness(t, "0xLoginA", "Login A")
	two := createOwnedBusiness(t, "0xLoginB", "Login B")
	first := &database.Staff{BusinessID: one.ID, Email: "selector@example.com", Name: "Selector", Role: database.StaffRoleServer, IsActive: true, InvitedBy: "owner-a"}
	second := &database.Staff{BusinessID: two.ID, Email: "SELECTOR@example.com", Name: "Selector", Role: database.StaffRoleManager, IsActive: true, InvitedBy: "owner-b"}
	require.NoError(t, database.GetDBWrapper().StaffService.Create(first))
	require.NoError(t, database.GetDBWrapper().StaffService.Create(second))
	require.NoError(t, gdb.Create(&database.StaffLoginCode{
		StaffID: first.ID, Code: "123456", ExpiresAt: time.Now().Add(10 * time.Minute), Used: false,
	}).Error)

	body, err := json.Marshal(map[string]any{"email": "selector@example.com", "code": "123456"})
	require.NoError(t, err)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	VerifyLoginCode(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var selection struct {
		Required       bool   `json:"membership_selection_required"`
		SelectionToken string `json:"selection_token"`
		Memberships    []struct {
			BusinessID uint   `json:"business_id"`
			Role       string `json:"role"`
		} `json:"memberships"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &selection))
	assert.True(t, selection.Required)
	assert.NotEmpty(t, selection.SelectionToken)
	require.Len(t, selection.Memberships, 2)
	assert.Empty(t, w.Result().Cookies(), "identity authentication must not mint a business-scoped cookie before selection")

	body, err = json.Marshal(map[string]any{"selection_token": selection.SelectionToken, "business_id": two.ID})
	require.NoError(t, err)
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	VerifyLoginCode(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var login struct {
		Staff database.Staff `json:"staff"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &login))
	assert.Equal(t, second.ID, login.Staff.ID)
	assert.Equal(t, two.ID, login.Staff.BusinessID)
	assert.Equal(t, database.StaffRoleManager, login.Staff.Role)
}

func TestVerifyLoginCode_SelectionTokenIsSingleUse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gdb := setupStaffHandlerTestDB(t)
	one := createOwnedBusiness(t, "0xReplayA", "Replay A")
	two := createOwnedBusiness(t, "0xReplayB", "Replay B")
	require.NoError(t, database.GetDBWrapper().StaffService.Create(&database.Staff{BusinessID: one.ID, Email: "replay@example.com", Name: "Replay", Role: database.StaffRoleServer, IsActive: true, InvitedBy: "owner-a"}))
	require.NoError(t, database.GetDBWrapper().StaffService.Create(&database.Staff{BusinessID: two.ID, Email: "replay@example.com", Name: "Replay", Role: database.StaffRoleManager, IsActive: true, InvitedBy: "owner-b"}))

	token, err := GenerateStaffMembershipSelectionToken("replay@example.com")
	require.NoError(t, err)
	var rows int64
	require.NoError(t, gdb.Model(&database.StaffMembershipSelectionToken{}).Count(&rows).Error)
	require.Equal(t, int64(1), rows, "issuing a selection token must record its jti hash")

	redeem := func(businessID uint) *httptest.ResponseRecorder {
		body, err := json.Marshal(map[string]any{"selection_token": token, "business_id": businessID})
		require.NoError(t, err)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		VerifyLoginCode(c)
		return w
	}

	first := redeem(one.ID)
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())

	// Replaying the same proof, even for the other membership, is refused.
	replay := redeem(two.ID)
	require.Equal(t, http.StatusUnauthorized, replay.Code, replay.Body.String())
	assert.Contains(t, replay.Body.String(), "MEMBERSHIP_SELECTION_INVALID")
	assert.Empty(t, replay.Result().Cookies(), "a replayed selection token must not mint a session cookie")

	require.NoError(t, gdb.Model(&database.StaffMembershipSelectionToken{}).Count(&rows).Error)
	assert.Equal(t, int64(0), rows, "redemption must delete the jti row")
}

func TestVerifyLoginCode_SelectionTokenWithoutRecordedJTIIsRejected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)
	one := createOwnedBusiness(t, "0xForgedA", "Forged A")
	require.NoError(t, database.GetDBWrapper().StaffService.Create(&database.Staff{BusinessID: one.ID, Email: "forged@example.com", Name: "Forged", Role: database.StaffRoleServer, IsActive: true, InvitedBy: "owner-a"}))

	// A validly signed token whose jti was never recorded (or already
	// redeemed) cannot be exchanged for a session.
	claims := jwt.MapClaims{
		"email": "forged@example.com",
		"type":  "staff_membership_selection",
		"jti":   "never-recorded",
		"exp":   time.Now().Add(5 * time.Minute).Unix(),
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(structs.GetSecretKey())
	require.NoError(t, err)

	body, err := json.Marshal(map[string]any{"selection_token": token, "business_id": one.ID})
	require.NoError(t, err)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	VerifyLoginCode(c)
	require.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())
	assert.Empty(t, w.Result().Cookies())
}

func TestRemoveStaff_RevokesOnlySelectedBusinessMembershipToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)
	one := createOwnedBusiness(t, "0xScopedA", "Scoped A")
	two := createOwnedBusiness(t, "0xScopedB", "Scoped B")
	first := &database.Staff{BusinessID: one.ID, Email: "scoped@example.com", Name: "Scoped", Role: database.StaffRoleServer, IsActive: true, InvitedBy: "owner-a"}
	second := &database.Staff{BusinessID: two.ID, Email: "SCOPED@example.com", Name: "Scoped", Role: database.StaffRoleManager, IsActive: true, InvitedBy: "owner-b"}
	require.NoError(t, database.GetDBWrapper().StaffService.Create(first))
	require.NoError(t, database.GetDBWrapper().StaffService.Create(second))
	firstToken, err := GenerateStaffToken(first)
	require.NoError(t, err)
	secondToken, err := GenerateStaffToken(second)
	require.NoError(t, err)
	firstClaims, err := VerifyToken(firstToken)
	require.NoError(t, err)
	secondClaims, err := VerifyToken(secondToken)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(one.ID)}, {Key: "staffId", Value: fmt.Sprint(first.ID)}}
	c.Request = httptest.NewRequest(http.MethodDelete, "/", nil)
	c.Set("address", one.OwnerAddress)
	RemoveStaff(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	removedRecorder := httptest.NewRecorder()
	removedContext, _ := gin.CreateTestContext(removedRecorder)
	removedContext.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	_, ok := hydrateLiveStaffContext(removedContext, firstClaims)
	assert.False(t, ok)
	assert.Equal(t, http.StatusUnauthorized, removedRecorder.Code)

	activeRecorder := httptest.NewRecorder()
	activeContext, _ := gin.CreateTestContext(activeRecorder)
	activeContext.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	live, ok := hydrateLiveStaffContext(activeContext, secondClaims)
	require.True(t, ok, activeRecorder.Body.String())
	assert.Equal(t, second.ID, live.ID)
	assert.Equal(t, two.ID, live.BusinessID)
}

func TestAcceptInvitation_RestoresInvitationWhenSessionPreparationFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)

	business := createOwnedBusiness(t, "0xOwnerA", "Invite Recovery Biz")
	invitation := createStaffInvitation(t, business.ID, "recover@example.com", "Recover Invitee", database.StaffRoleHost)

	originalPrepare := prepareAcceptedInvitationSession
	defer func() {
		prepareAcceptedInvitationSession = originalPrepare
	}()

	prepareAcceptedInvitationSession = func(*gin.Context, *database.Staff) (*preparedStaffLogin, error) {
		return nil, fmt.Errorf("session unavailable")
	}

	body, err := json.Marshal(map[string]string{
		"token": invitation.Token,
		"name":  "Recovered Staff",
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	AcceptInvitation(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var staffCount int64
	require.NoError(t, database.GetDB().Model(&database.Staff{}).Where("email = ?", invitation.Email).Count(&staffCount).Error)
	assert.Zero(t, staffCount)

	var persistedInvitation database.StaffInvitation
	require.NoError(t, database.GetDB().First(&persistedInvitation, invitation.ID).Error)
	assert.Equal(t, database.InvitationStatusPending, persistedInvitation.Status)
}

func TestAcceptInvitation_RehireSessionFailureRestoresInactiveMembership(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gdb := setupStaffHandlerTestDB(t)
	business := createOwnedBusiness(t, "0xRehireRestore", "Rehire Restore")
	staff := &database.Staff{BusinessID: business.ID, Email: "restore-rehire@example.com", Name: "Before", Role: database.StaffRoleServer, IsActive: true, InvitedBy: "owner", AuthzVersion: 6}
	require.NoError(t, database.GetDBWrapper().StaffService.Create(staff))
	require.NoError(t, database.GetDBWrapper().StaffService.SoftDelete(staff.ID))
	invitation := createStaffInvitation(t, business.ID, staff.Email, "After", database.StaffRoleManager)

	originalPrepare := prepareAcceptedInvitationSession
	defer func() { prepareAcceptedInvitationSession = originalPrepare }()
	prepareAcceptedInvitationSession = func(*gin.Context, *database.Staff) (*preparedStaffLogin, error) {
		return nil, fmt.Errorf("session unavailable")
	}

	body, err := json.Marshal(map[string]string{"token": invitation.Token, "name": "After"})
	require.NoError(t, err)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	AcceptInvitation(c)
	require.Equal(t, http.StatusInternalServerError, w.Code)

	var restored database.Staff
	require.NoError(t, gdb.First(&restored, staff.ID).Error)
	assert.False(t, restored.IsActive)
	assert.Equal(t, "Before", restored.Name)
	assert.Equal(t, database.StaffRoleServer, restored.Role)
	assert.Equal(t, 6, restored.AuthzVersion)
	var membership database.StaffMembership
	require.NoError(t, gdb.Where("legacy_staff_id = ?", staff.ID).Take(&membership).Error)
	assert.False(t, membership.IsActive)
	assert.Equal(t, 6, membership.AuthzVersion)
	var persistedInvitation database.StaffInvitation
	require.NoError(t, gdb.First(&persistedInvitation, invitation.ID).Error)
	assert.Equal(t, database.InvitationStatusPending, persistedInvitation.Status)
}

func TestGetBusinessStaff_IncludesExpiredInvitations(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)

	business := createOwnedBusiness(t, "0xOwnerA", "Expired Visible Biz")
	pendingInvitation := createStaffInvitation(t, business.ID, "pending@example.com", "Pending Invitee", database.StaffRoleServer)
	expiredInvitation := createStaffInvitation(t, business.ID, "expired-visible@example.com", "Expired Visible", database.StaffRoleHost)
	acceptedInvitation := createStaffInvitation(t, business.ID, "accepted@example.com", "Accepted Invitee", database.StaffRoleManager)

	require.NoError(t, database.GetDB().Model(&database.StaffInvitation{}).Where("id = ?", expiredInvitation.ID).
		Update("status", database.InvitationStatusExpired).Error)
	require.NoError(t, database.GetDB().Model(&database.StaffInvitation{}).Where("id = ?", acceptedInvitation.ID).
		Update("status", database.InvitationStatusAccepted).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set("address", business.OwnerAddress)

	GetBusinessStaff(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		PendingInvitations []database.StaffInvitation `json:"pending_invitations"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.PendingInvitations, 2)

	statusByID := map[uint]database.InvitationStatus{}
	for _, invitation := range resp.PendingInvitations {
		statusByID[invitation.ID] = invitation.Status
	}
	assert.Equal(t, database.InvitationStatusPending, statusByID[pendingInvitation.ID])
	assert.Equal(t, database.InvitationStatusExpired, statusByID[expiredInvitation.ID])
	_, foundAccepted := statusByID[acceptedInvitation.ID]
	assert.False(t, foundAccepted)
}

func TestGetBusinessStaff_IncludesInactiveStaff(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)

	business := createOwnedBusiness(t, "0xInactiveRosterOwner", "Inactive Roster Biz")
	active := &database.Staff{BusinessID: business.ID, Email: "active-roster@example.com", Name: "Active", Role: database.StaffRoleServer, IsActive: true, InvitedBy: business.OwnerAddress}
	inactive := &database.Staff{BusinessID: business.ID, Email: "inactive-roster@example.com", Name: "Inactive", Role: database.StaffRoleHost, IsActive: true, InvitedBy: business.OwnerAddress}
	require.NoError(t, database.GetDBWrapper().StaffService.Create(active))
	require.NoError(t, database.GetDBWrapper().StaffService.Create(inactive))
	require.NoError(t, database.GetDBWrapper().StaffService.SoftDelete(inactive.ID))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set("address", business.OwnerAddress)

	GetBusinessStaff(c)
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Staff []database.Staff `json:"staff"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Staff, 2)
	activeByID := map[uint]bool{}
	for _, member := range resp.Staff {
		activeByID[member.ID] = member.IsActive
	}
	assert.True(t, activeByID[active.ID])
	assert.False(t, activeByID[inactive.ID])
}

// GetBusinessStaff must never serialize invitation secrets. kitchen/server/host
// all hold staff:read; leaking tokens enables unauthenticated accept → privilege
// escalation (kitchen→manager). Tokens stay in DB + email URL only.
func TestGetBusinessStaff_OmitsInvitationTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)

	business := createOwnedBusiness(t, "0xOwnerA", "Token Leak Biz")
	invitation := createStaffInvitation(t, business.ID, "secret@example.com", "Secret Invitee", database.StaffRoleManager)
	require.NotEmpty(t, invitation.Token)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set("address", business.OwnerAddress)

	GetBusinessStaff(c)
	require.Equal(t, http.StatusOK, w.Code)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	var pending []map[string]any
	require.NoError(t, json.Unmarshal(raw["pending_invitations"], &pending))
	require.Len(t, pending, 1)
	_, hasToken := pending[0]["token"]
	assert.False(t, hasToken, "pending_invitations must not include token (got keys %v)", keysOfMap(pending[0]))
	// Confirm the secret is still in the DB so accept-by-email still works.
	var persisted database.StaffInvitation
	require.NoError(t, database.GetDB().First(&persisted, invitation.ID).Error)
	assert.Equal(t, invitation.Token, persisted.Token)
}

func TestGetInvitationLink_ReturnsURLForAuthorizedInviter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)

	business := createOwnedBusiness(t, "0xOwnerA", "Link Biz")
	invitation := createStaffInvitation(t, business.ID, "link@example.com", "Link Invitee", database.StaffRoleServer)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
		{Key: "invitationId", Value: fmt.Sprintf("%d", invitation.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set("address", business.OwnerAddress)

	GetInvitationLink(c)
	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	url, _ := resp["invitation_url"].(string)
	assert.Contains(t, url, "/staff/accept-invitation?token=")
	assert.Contains(t, url, invitation.Token)
	assert.Equal(t, float64(invitation.ID), resp["invitation_id"])
}

func TestGetInvitationLink_RejectsCrossBusiness(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)

	ownerBiz := createOwnedBusiness(t, "0xOwnerA", "Owner A")
	otherBiz := createOwnedBusiness(t, "0xOwnerB", "Owner B")
	invitation := createStaffInvitation(t, otherBiz.ID, "cross@example.com", "Cross", database.StaffRoleServer)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", ownerBiz.ID)},
		{Key: "invitationId", Value: fmt.Sprintf("%d", invitation.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set("address", ownerBiz.OwnerAddress)

	GetInvitationLink(c)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func keysOfMap(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestResendInvitation_AllowsExpiredInvitation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)

	business := createOwnedBusiness(t, "0xOwnerA", "Expired Invite Biz")
	invitation := createStaffInvitation(t, business.ID, "expired@example.com", "Expired Invitee", database.StaffRoleServer)
	originalToken := invitation.Token
	require.NoError(t, database.GetDB().Model(&database.StaffInvitation{}).Where("id = ?", invitation.ID).
		Updates(map[string]any{
			"expires_at": time.Now().Add(-2 * time.Hour),
			"status":     database.InvitationStatusExpired,
		}).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
		{Key: "invitationId", Value: fmt.Sprintf("%d", invitation.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	c.Set("address", business.OwnerAddress)

	ResendInvitation(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var updated database.StaffInvitation
	require.NoError(t, database.GetDB().First(&updated, invitation.ID).Error)
	assert.Equal(t, database.InvitationStatusPending, updated.Status)
	assert.NotEqual(t, originalToken, updated.Token)
	assert.True(t, updated.ExpiresAt.After(time.Now()))
}

func TestInviteStaff_SucceedsWithoutConfiguredEmailServer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)

	originalEmailServer := emails.EmailServerInstance
	emails.EmailServerInstance = nil
	defer func() {
		emails.EmailServerInstance = originalEmailServer
	}()

	business := createOwnedBusiness(t, "0xOwnerA", "No Email Server Biz")

	body, err := json.Marshal(map[string]string{
		"email": "invitee@example.com",
		"name":  "Invitee Person",
		"role":  string(database.StaffRoleServer),
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("address", business.OwnerAddress)

	assert.NotPanics(t, func() {
		InviteStaff(c)
	})
	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestVerifyLoginCode_RejectsInactiveStaff(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)

	business := createOwnedBusiness(t, "0xOwnerA", "Inactive Biz")
	staff := createStaffMember(t, business.ID, "inactive@example.com", "Inactive Staff")
	require.NoError(t, database.GetDB().Model(&database.Staff{}).Where("id = ?", staff.ID).Update("is_active", false).Error)
	require.NoError(t, database.GetDB().Create(&database.StaffLoginCode{
		StaffID:   staff.ID,
		Code:      "123456",
		ExpiresAt: time.Now().Add(10 * time.Minute),
		Used:      false,
	}).Error)

	body, err := json.Marshal(map[string]string{
		"email": staff.Email,
		"code":  "123456",
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	VerifyLoginCode(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Invalid or expired login code")
}

func TestRequestLoginCode_DoesNotRevealMissingStaff(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)

	originalSender := sendStaffLoginCodeEmail
	defer func() {
		sendStaffLoginCodeEmail = originalSender
	}()

	sent := false
	sendStaffLoginCodeEmail = func(_ *database.Staff, _ *database.Business, _ string) error {
		sent = true
		return nil
	}

	body, err := json.Marshal(map[string]string{
		"email": "missing@example.com",
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	RequestLoginCode(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.False(t, sent)

	var loginCodeCount int64
	require.NoError(t, database.GetDB().Model(&database.StaffLoginCode{}).Count(&loginCodeCount).Error)
	assert.Zero(t, loginCodeCount)
}

func TestRequestLoginCode_DoesNotRevealInactiveStaff(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)

	business := createOwnedBusiness(t, "0xOwnerA", "Inactive Request Biz")
	staff := createStaffMember(t, business.ID, "inactive-request@example.com", "Inactive Request")
	require.NoError(t, database.GetDB().Model(&database.Staff{}).Where("id = ?", staff.ID).Update("is_active", false).Error)

	originalSender := sendStaffLoginCodeEmail
	defer func() {
		sendStaffLoginCodeEmail = originalSender
	}()

	sent := false
	sendStaffLoginCodeEmail = func(_ *database.Staff, _ *database.Business, _ string) error {
		sent = true
		return nil
	}

	body, err := json.Marshal(map[string]string{
		"email": staff.Email,
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	RequestLoginCode(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.False(t, sent)

	var loginCodeCount int64
	require.NoError(t, database.GetDB().Model(&database.StaffLoginCode{}).Count(&loginCodeCount).Error)
	assert.Zero(t, loginCodeCount)
}

func TestVerifyLoginCode_DoesNotRevealMissingStaff(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)

	body, err := json.Marshal(map[string]string{
		"email": "missing@example.com",
		"code":  "123456",
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	VerifyLoginCode(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Invalid or expired login code")
}

func TestRemoveStaff_RevokesScopedStaffSessions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)

	business := createOwnedBusiness(t, "0xOwnerA", "Revoked Staff Biz")
	staff := createStaffMember(t, business.ID, "revoked-staff@example.com", "Revoked Staff")

	sess, err := session.GlobalStore.Create(session.CreateInput{
		UserID:    &staff.ID,
		TokenHash: session.HashToken("revoked-staff-token"),
		Provider:  "staff_code",
		ExpiresAt: time.Now().Add(time.Hour),
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
		{Key: "staffId", Value: fmt.Sprintf("%d", staff.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodDelete, "/", nil)
	c.Set("address", business.OwnerAddress)

	RemoveStaff(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var persistedSession session.UserSession
	require.NoError(t, database.GetDB().First(&persistedSession, sess.ID).Error)
	assert.True(t, persistedSession.Revoked)
	assert.Equal(t, string(session.RevocationReasonAdministrative), persistedSession.RevocationReason)
}

func TestStaffLogout_RecordsUserLogoutReason(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)

	staffID := uint(77)
	rawToken := "staff-logout-token"
	sess, err := session.GlobalStore.Create(session.CreateInput{
		UserID:    &staffID,
		TokenHash: session.HashToken(rawToken),
		Provider:  "staff_code",
		ExpiresAt: time.Now().Add(time.Hour),
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/staff/logout", nil)
	c.Request.AddCookie(&http.Cookie{Name: "staff_token", Value: rawToken})
	c.Set("staff_id", staffID)

	StaffLogout(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var persisted session.UserSession
	require.NoError(t, database.GetDB().First(&persisted, sess.ID).Error)
	assert.True(t, persisted.Revoked)
	assert.Equal(t, string(session.RevocationReasonUserLogout), persisted.RevocationReason)
}

func TestRequestLoginCode_RetriesGeneratedCodeCollision(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)

	business := createOwnedBusiness(t, "0xOwnerA", "Collision Biz")
	existingStaff := createStaffMember(t, business.ID, "existing@example.com", "Existing Staff")
	targetStaff := createStaffMember(t, business.ID, "target@example.com", "Target Staff")

	require.NoError(t, database.GetDB().Create(&database.StaffLoginCode{
		StaffID:   existingStaff.ID,
		Code:      "111111",
		ExpiresAt: time.Now().Add(10 * time.Minute),
		Used:      false,
	}).Error)
	require.NoError(t, database.GetDB().Exec(`DROP INDEX IF EXISTS idx_staff_login_codes_code`).Error)
	require.NoError(t, database.GetDB().Exec(`CREATE UNIQUE INDEX idx_staff_login_codes_code ON staff_login_codes(code)`).Error)

	originalGenerator := staffLoginCodeGenerator
	originalSender := sendStaffLoginCodeEmail
	defer func() {
		staffLoginCodeGenerator = originalGenerator
		sendStaffLoginCodeEmail = originalSender
	}()

	codes := []string{"111111", "222222"}
	staffLoginCodeGenerator = func() (string, error) {
		code := codes[0]
		codes = codes[1:]
		return code, nil
	}
	sendStaffLoginCodeEmail = func(_ *database.Staff, _ *database.Business, _ string) error {
		return nil
	}

	body, err := json.Marshal(map[string]string{
		"email": targetStaff.Email,
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	RequestLoginCode(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var loginCodes []database.StaffLoginCode
	require.NoError(t, database.GetDB().Where("staff_id = ?", targetStaff.ID).Find(&loginCodes).Error)
	require.Len(t, loginCodes, 1)
	assert.Equal(t, "222222", loginCodes[0].Code)
}

func TestRequestLoginCode_AllowsSharedCodeAcrossDifferentStaff(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)

	business := createOwnedBusiness(t, "0xOwnerA", "Shared Code Biz")
	existingStaff := createStaffMember(t, business.ID, "existing-shared@example.com", "Existing Shared")
	targetStaff := createStaffMember(t, business.ID, "target-shared@example.com", "Target Shared")

	require.NoError(t, database.GetDB().Create(&database.StaffLoginCode{
		StaffID:   existingStaff.ID,
		Code:      "333333",
		ExpiresAt: time.Now().Add(10 * time.Minute),
		Used:      false,
	}).Error)

	originalGenerator := staffLoginCodeGenerator
	originalSender := sendStaffLoginCodeEmail
	defer func() {
		staffLoginCodeGenerator = originalGenerator
		sendStaffLoginCodeEmail = originalSender
	}()

	callCount := 0
	staffLoginCodeGenerator = func() (string, error) {
		callCount++
		return "333333", nil
	}
	sendStaffLoginCodeEmail = func(_ *database.Staff, _ *database.Business, _ string) error {
		return nil
	}

	body, err := json.Marshal(map[string]string{
		"email": targetStaff.Email,
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	RequestLoginCode(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 1, callCount)

	var loginCodes []database.StaffLoginCode
	require.NoError(t, database.GetDB().Where("staff_id = ?", targetStaff.ID).Find(&loginCodes).Error)
	require.Len(t, loginCodes, 1)
	assert.Equal(t, "333333", loginCodes[0].Code)
}

func TestRequestLoginCode_ReplacesExistingActiveCodesForStaff(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)

	business := createOwnedBusiness(t, "0xOwnerA", "Replace Codes Biz")
	staff := createStaffMember(t, business.ID, "replace@example.com", "Replace Codes")

	require.NoError(t, database.GetDB().Create(&database.StaffLoginCode{
		StaffID:   staff.ID,
		Code:      "444444",
		ExpiresAt: time.Now().Add(10 * time.Minute),
		Used:      false,
	}).Error)

	originalGenerator := staffLoginCodeGenerator
	originalSender := sendStaffLoginCodeEmail
	defer func() {
		staffLoginCodeGenerator = originalGenerator
		sendStaffLoginCodeEmail = originalSender
	}()

	staffLoginCodeGenerator = func() (string, error) {
		return "555555", nil
	}
	sendStaffLoginCodeEmail = func(_ *database.Staff, _ *database.Business, _ string) error {
		return nil
	}

	body, err := json.Marshal(map[string]string{
		"email": staff.Email,
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	RequestLoginCode(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var loginCodes []database.StaffLoginCode
	require.NoError(t, database.GetDB().Where("staff_id = ?", staff.ID).Order("id ASC").Find(&loginCodes).Error)
	require.Len(t, loginCodes, 2)
	assert.Equal(t, "444444", loginCodes[0].Code)
	assert.True(t, loginCodes[0].Used)
	assert.Equal(t, "555555", loginCodes[1].Code)
	assert.False(t, loginCodes[1].Used)
}

func TestRequestLoginCode_PreservesExistingCodeWhenEmailFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)

	business := createOwnedBusiness(t, "0xOwnerA", "Email Failure Biz")
	staff := createStaffMember(t, business.ID, "email-failure@example.com", "Email Failure")

	require.NoError(t, database.GetDB().Create(&database.StaffLoginCode{
		StaffID:   staff.ID,
		Code:      "565656",
		ExpiresAt: time.Now().Add(10 * time.Minute),
		Used:      false,
	}).Error)

	originalGenerator := staffLoginCodeGenerator
	originalSender := sendStaffLoginCodeEmail
	defer func() {
		staffLoginCodeGenerator = originalGenerator
		sendStaffLoginCodeEmail = originalSender
	}()

	staffLoginCodeGenerator = func() (string, error) {
		return "575757", nil
	}
	sendStaffLoginCodeEmail = func(_ *database.Staff, _ *database.Business, _ string) error {
		return fmt.Errorf("smtp unavailable")
	}

	body, err := json.Marshal(map[string]string{
		"email": staff.Email,
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	RequestLoginCode(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var loginCodes []database.StaffLoginCode
	require.NoError(t, database.GetDB().Where("staff_id = ?", staff.ID).Order("id ASC").Find(&loginCodes).Error)
	require.Len(t, loginCodes, 1)
	assert.Equal(t, "565656", loginCodes[0].Code)
	assert.False(t, loginCodes[0].Used)
}

func TestVerifyLoginCode_MarksDuplicateRowsUsedAndRejectsReuse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)

	business := createOwnedBusiness(t, "0xOwnerA", "Verify Duplicate Biz")
	staff := createStaffMember(t, business.ID, "verify-duplicate@example.com", "Verify Duplicate")

	firstCode := &database.StaffLoginCode{
		StaffID:   staff.ID,
		Code:      "666666",
		ExpiresAt: time.Now().Add(10 * time.Minute),
		Used:      false,
	}
	secondCode := &database.StaffLoginCode{
		StaffID:   staff.ID,
		Code:      "666666",
		ExpiresAt: time.Now().Add(10 * time.Minute),
		Used:      false,
	}
	require.NoError(t, database.GetDB().Create(firstCode).Error)
	require.NoError(t, database.GetDB().Create(secondCode).Error)

	body, err := json.Marshal(map[string]string{
		"email": staff.Email,
		"code":  "666666",
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	VerifyLoginCode(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var loginCodes []database.StaffLoginCode
	require.NoError(t, database.GetDB().Where("staff_id = ? AND code = ?", staff.ID, "666666").Order("id ASC").Find(&loginCodes).Error)
	require.Len(t, loginCodes, 2)
	for _, loginCode := range loginCodes {
		assert.True(t, loginCode.Used)
	}

	reuseRecorder := httptest.NewRecorder()
	reuseContext, _ := gin.CreateTestContext(reuseRecorder)
	reuseContext.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	reuseContext.Request.Header.Set("Content-Type", "application/json")

	VerifyLoginCode(reuseContext)

	assert.Equal(t, http.StatusBadRequest, reuseRecorder.Code)
}

func TestVerifyLoginCode_PreservesCodeWhenSessionPreparationFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)

	business := createOwnedBusiness(t, "0xOwnerA", "Session Failure Biz")
	staff := createStaffMember(t, business.ID, "session-failure@example.com", "Session Failure")

	loginCode := &database.StaffLoginCode{
		StaffID:   staff.ID,
		Code:      "787878",
		ExpiresAt: time.Now().Add(10 * time.Minute),
		Used:      false,
	}
	require.NoError(t, database.GetDB().Create(loginCode).Error)

	originalPrepare := prepareStaffLoginSession
	defer func() {
		prepareStaffLoginSession = originalPrepare
	}()

	prepareStaffLoginSession = func(*gin.Context, *database.Staff) (*preparedStaffLogin, error) {
		return nil, fmt.Errorf("session unavailable")
	}

	body, err := json.Marshal(map[string]string{
		"email": staff.Email,
		"code":  "787878",
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	VerifyLoginCode(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var persisted database.StaffLoginCode
	require.NoError(t, database.GetDB().First(&persisted, loginCode.ID).Error)
	assert.False(t, persisted.Used)
}

func TestVerifyLoginCode_InvalidCodeDoesNotPrepareSessionOrCreateSessionRow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)

	business := createOwnedBusiness(t, "0xOwnerA", "Invalid Code Biz")
	staff := createStaffMember(t, business.ID, "invalid-code@example.com", "Invalid Code")

	require.NoError(t, database.GetDB().Create(&database.StaffLoginCode{
		StaffID:   staff.ID,
		Code:      "797979",
		ExpiresAt: time.Now().Add(10 * time.Minute),
		Used:      false,
	}).Error)

	originalPrepare := prepareStaffLoginSession
	defer func() {
		prepareStaffLoginSession = originalPrepare
	}()

	preparedCalled := false
	prepareStaffLoginSession = func(*gin.Context, *database.Staff) (*preparedStaffLogin, error) {
		preparedCalled = true
		return &preparedStaffLogin{token: "should-not-be-used"}, nil
	}

	body, err := json.Marshal(map[string]string{
		"email": staff.Email,
		"code":  "000000",
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	VerifyLoginCode(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.False(t, preparedCalled)

	var sessionCount int64
	require.NoError(t, database.GetDB().Model(&session.UserSession{}).Count(&sessionCount).Error)
	assert.Zero(t, sessionCount)
}

// generateStaffTokenWithSession creates a real session in the store and returns a signed token
// containing the session ID. This satisfies the session_id enforcement in validateSession.
func generateStaffTokenWithSession(t *testing.T, staff *database.Staff) string {
	t.Helper()
	// Pre-generate the token once to get its raw value, then store the hash.
	// We need to know the session ID before signing, so we create a placeholder session first
	// and then update it with the real token hash after signing.
	dummyUID := staff.ID
	sess, err := session.GlobalStore.Create(session.CreateInput{
		UserID:    &dummyUID,
		TokenHash: "placeholder",
		Provider:  "staff",
		ExpiresAt: time.Now().Add(1 * time.Hour),
	})
	require.NoError(t, err)

	tokenString, err := GenerateStaffToken(staff, sess.ID)
	require.NoError(t, err)

	require.NoError(t, session.GlobalStore.UpdateTokenHash(sess.ID, session.HashToken(tokenString)))
	return tokenString
}

func TestStaffAuthenticationMiddleware_UsesCurrentDatabaseRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)

	business := createOwnedBusiness(t, "0xOwnerA", "Role Biz")
	staff := createStaffMember(t, business.ID, "role@example.com", "Role Staff")
	token := generateStaffTokenWithSession(t, staff)
	require.NoError(t, database.GetDB().Model(&database.Staff{}).Where("id = ?", staff.ID).Update("role", database.StaffRoleKitchen).Error)
	require.NoError(t, database.GetDB().Model(&database.Staff{}).Where("id = ?", staff.ID).Update("role", database.StaffRoleKitchen).Error)

	router := gin.New()
	router.Use(StaffAuthenticationMiddleware())
	router.GET("/staff/profile", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"role": c.GetString("staff_role")})
	})

	req := httptest.NewRequest(http.MethodGet, "/staff/profile", nil)
	req.AddCookie(&http.Cookie{Name: "staff_token", Value: token})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, string(database.StaffRoleKitchen), resp["role"])
}

func TestStaffAuthenticationMiddleware_RejectsInactiveStaff(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)

	business := createOwnedBusiness(t, "0xOwnerA", "Inactive Middleware Biz")
	staff := createStaffMember(t, business.ID, "middleware@example.com", "Middleware Staff")
	token := generateStaffTokenWithSession(t, staff)
	require.NoError(t, database.GetDB().Model(&database.Staff{}).Where("id = ?", staff.ID).Update("is_active", false).Error)

	router := gin.New()
	router.Use(StaffAuthenticationMiddleware())
	router.GET("/staff/profile", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/staff/profile", nil)
	req.AddCookie(&http.Cookie{Name: "staff_token", Value: token})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestHybridAuthenticationMiddleware_UsesCurrentDatabaseRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)

	business := createOwnedBusiness(t, "0xOwnerA", "Hybrid Role Biz")
	staff := createStaffMember(t, business.ID, "hybrid-role@example.com", "Hybrid Role Staff")
	token := generateStaffTokenWithSession(t, staff)
	require.NoError(t, database.GetDB().Model(&database.Staff{}).Where("id = ?", staff.ID).Update("role", database.StaffRoleKitchen).Error)

	router := gin.New()
	router.Use(HybridAuthenticationMiddleware())
	router.GET("/inside/businesses/:id/dashboard", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"role": c.GetString("staff_role")})
	})

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/inside/businesses/%d/dashboard", business.ID), nil)
	req.AddCookie(&http.Cookie{Name: "staff_token", Value: token})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, string(database.StaffRoleKitchen), resp["role"])
}

func TestHybridAuthenticationMiddleware_RejectsInactiveStaff(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)

	business := createOwnedBusiness(t, "0xOwnerA", "Hybrid Inactive Biz")
	staff := createStaffMember(t, business.ID, "hybrid-inactive@example.com", "Hybrid Inactive Staff")
	token := generateStaffTokenWithSession(t, staff)
	require.NoError(t, database.GetDB().Model(&database.Staff{}).Where("id = ?", staff.ID).Update("is_active", false).Error)

	router := gin.New()
	router.Use(HybridAuthenticationMiddleware())
	router.GET("/inside/businesses/:id/dashboard", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/inside/businesses/%d/dashboard", business.ID), nil)
	req.AddCookie(&http.Cookie{Name: "staff_token", Value: token})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestHybridAuthenticationMiddleware_UsesCurrentStaffBusiness(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)

	originalBusiness := createOwnedBusiness(t, "0xOwnerA", "Original Hybrid Biz")
	newBusiness := createOwnedBusiness(t, "0xOwnerA", "Moved Hybrid Biz")
	staff := createStaffMember(t, originalBusiness.ID, "hybrid-move@example.com", "Hybrid Moved Staff")
	token := generateStaffTokenWithSession(t, staff)
	require.NoError(t, database.GetDB().Model(&database.Staff{}).Where("id = ?", staff.ID).Update("business_id", newBusiness.ID).Error)

	router := gin.New()
	router.Use(HybridAuthenticationMiddleware())
	router.GET("/inside/businesses/:id/dashboard", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/inside/businesses/%d/dashboard", originalBusiness.ID), nil)
	req.AddCookie(&http.Cookie{Name: "staff_token", Value: token})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestSetStaffPinRoute_OperationalGateCanResolveBusinessFromStaffID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)

	business := createOwnedBusiness(t, "0xOwnerPinRoute", "PIN Route Biz")
	staff := createStaffMember(t, business.ID, "pin-route@example.com", "PIN Route Staff")
	token := generateStaffTokenWithSession(t, staff)

	router := gin.New()
	router.Use(HybridAuthenticationMiddleware())
	router.POST("/inside/staff/:staff_id/pin", RequireOperationalBusiness(), SetStaffPin)

	body, err := json.Marshal(map[string]string{"pin": "1234"})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/inside/staff/%d/pin", staff.ID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "staff_token", Value: token})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	hasPIN, err := database.StaffHasPin(staff.ID)
	require.NoError(t, err)
	assert.True(t, hasPIN)
}

// Offboarding must revoke the push channel: when a staff member is removed,
// their PushSubscription rows for THAT business are deleted (matched through
// the same staff.email → users.email join notifyStaff uses, case-insensitive).
// Subscriptions for other businesses stay.
func TestRemoveStaff_DeletesPushSubscriptionsForBusiness(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupStaffHandlerTestDB(t)
	require.NoError(t, db.AutoMigrate(&database.User{}, &database.PushSubscription{}))

	business := createOwnedBusiness(t, "0xOwnerPush", "Push Cleanup Biz")
	staff := createStaffMember(t, business.ID, "offboard@example.com", "Offboarded")

	// User email stored with different casing — cleanup must still match.
	user := &database.User{Email: "Offboard@Example.COM", AuthMethod: "email"}
	require.NoError(t, db.Create(user).Error)

	require.NoError(t, db.Create(&database.PushSubscription{
		UserID: user.ID, BusinessID: business.ID,
		PrincipalType: database.PushPrincipalOwnerUser, PrincipalID: user.ID,
		Endpoint: "https://push.example/offboard-this-biz", P256dhKey: "k", AuthKey: "a",
	}).Error)
	require.NoError(t, db.Create(&database.PushSubscription{
		UserID: user.ID, BusinessID: business.ID + 999,
		PrincipalType: database.PushPrincipalOwnerUser, PrincipalID: user.ID,
		Endpoint: "https://push.example/offboard-other-biz", P256dhKey: "k", AuthKey: "a",
	}).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
		{Key: "staffId", Value: fmt.Sprintf("%d", staff.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodDelete, "/", nil)
	c.Set("address", "0xOwnerPush")

	RemoveStaff(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var remaining []database.PushSubscription
	require.NoError(t, db.Find(&remaining).Error)
	require.Len(t, remaining, 1, "only the removed business's subscription is deleted")
	assert.Equal(t, business.ID+999, remaining[0].BusinessID)
}

func TestInviteStaff_EmailFailureDisclosedWithInviteLink(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)
	business := createOwnedBusiness(t, "0xInviteOwner", "invite-email-fail")

	orig := sendStaffInvitationEmailIfConfigured
	sendStaffInvitationEmailIfConfigured = func(uint, []string, string, string, string, string, string, int) (bool, error) {
		return false, fmt.Errorf("provider outage")
	}
	t.Cleanup(func() { sendStaffInvitationEmailIfConfigured = orig })

	body, _ := json.Marshal(map[string]string{
		"email": "new.hire@example.com",
		"name":  "New Hire",
		"role":  "server",
	})
	c, w := makeTestContext("POST", "/businesses/"+business.BusinessId+"/staff/invite",
		gin.Params{{Key: "id", Value: business.BusinessId}})
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("address", "0xInviteOwner")

	InviteStaff(c)

	require.Equal(t, http.StatusCreated, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, false, resp["email_sent"], "a failed send must not report success")
	url, _ := resp["invitation_url"].(string)
	assert.Contains(t, url, "/staff/accept-invitation?token=",
		"the inviter must get a copyable fallback link")
}

func TestInviteStaff_EmailSuccessReportsSent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)
	business := createOwnedBusiness(t, "0xInviteOwner2", "invite-email-ok")

	orig := sendStaffInvitationEmailIfConfigured
	sendStaffInvitationEmailIfConfigured = func(uint, []string, string, string, string, string, string, int) (bool, error) {
		return true, nil
	}
	t.Cleanup(func() { sendStaffInvitationEmailIfConfigured = orig })

	body, _ := json.Marshal(map[string]string{
		"email": "second.hire@example.com",
		"name":  "Second Hire",
		"role":  "server",
	})
	c, w := makeTestContext("POST", "/businesses/"+business.BusinessId+"/staff/invite",
		gin.Params{{Key: "id", Value: business.BusinessId}})
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("address", "0xInviteOwner2")

	InviteStaff(c)

	require.Equal(t, http.StatusCreated, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, true, resp["email_sent"])
	assert.NotEmpty(t, resp["invitation_id"])
	assert.NotEmpty(t, resp["expires_at"])
}

func TestInviteStaff_DuplicateGuardDBErrorSurfaces(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gdb := setupStaffHandlerTestDB(t)
	business := createOwnedBusiness(t, "0xGuardOwner", "invite-guard-db")

	// Break the staff lookup: drop the table so GetByEmail returns a real
	// error (not gorm.ErrRecordNotFound).
	require.NoError(t, gdb.Migrator().DropTable(&database.Staff{}))

	body, _ := json.Marshal(map[string]string{
		"email": "guard@example.com",
		"name":  "Guard",
		"role":  "server",
	})
	c, w := makeTestContext("POST", "/businesses/"+business.BusinessId+"/staff/invite",
		gin.Params{{Key: "id", Value: business.BusinessId}})
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("address", "0xGuardOwner")

	InviteStaff(c)

	require.Equal(t, http.StatusInternalServerError, w.Code,
		"a broken duplicate guard must fail loud, not create a possibly-duplicate invite")
	assert.Contains(t, w.Body.String(), "INVITE_VALIDATION_FAILED")
}

func TestStaffInvitationErrors_CarryStableCodes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gdb := setupStaffHandlerTestDB(t)
	business := createOwnedBusiness(t, "0xCodeOwner", "invite-codes")

	// Expired invitation -> INVITE_EXPIRED on accept.
	expired := &database.StaffInvitation{
		BusinessID: business.ID,
		Email:      "expired@example.com",
		Name:       "Expired",
		Role:       "server",
		Token:      "expired-token-0001",
		Status:     database.InvitationStatusPending,
		ExpiresAt:  time.Now().Add(-1 * time.Hour),
	}
	require.NoError(t, gdb.Create(expired).Error)

	body, _ := json.Marshal(map[string]string{"token": "expired-token-0001", "name": "Expired"})
	c, w := makeTestContext("POST", "/staff/accept-invitation", nil)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	AcceptInvitation(c)
	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "INVITE_EXPIRED")

	// Unknown token -> INVITE_INVALID on accept.
	body, _ = json.Marshal(map[string]string{"token": "no-such-token", "name": "Nobody"})
	c, w = makeTestContext("POST", "/staff/accept-invitation", nil)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	AcceptInvitation(c)
	require.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "INVITE_INVALID")

	// Email already a staff account -> STAFF_ACCOUNT_EXISTS on accept.
	staffRow := &database.Staff{
		BusinessID: business.ID, Email: "taken@example.com", Name: "Taken",
		Role: "server", IsActive: true,
	}
	require.NoError(t, gdb.Create(staffRow).Error)
	pending := &database.StaffInvitation{
		BusinessID: business.ID, Email: "taken@example.com", Name: "Taken",
		Role: "server", Token: "taken-token-0001",
		Status: database.InvitationStatusPending, ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	require.NoError(t, gdb.Create(pending).Error)
	body, _ = json.Marshal(map[string]string{"token": "taken-token-0001", "name": "Taken"})
	c, w = makeTestContext("POST", "/staff/accept-invitation", nil)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	AcceptInvitation(c)
	require.Equal(t, http.StatusConflict, w.Code)
	assert.Contains(t, w.Body.String(), "STAFF_ACCOUNT_EXISTS")
}

// TestRevokeInvitation_InvalidatesTokenAndMarksRevoked proves revoke rotates the
// token (killing the original link) and flips the status to revoked; an accepted
// invitation cannot be revoked.
func TestRevokeInvitation_InvalidatesTokenAndMarksRevoked(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)

	business := createOwnedBusiness(t, "0xOwnerA", "Revoke Biz")
	invitation := createStaffInvitation(t, business.ID, "revoke@example.com", "Revoke Invitee", database.StaffRoleServer)
	originalToken := invitation.Token

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
		{Key: "invitationId", Value: fmt.Sprintf("%d", invitation.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	c.Set("address", business.OwnerAddress)

	RevokeInvitation(c)
	assert.Equal(t, http.StatusOK, w.Code)

	var updated database.StaffInvitation
	require.NoError(t, database.GetDB().First(&updated, invitation.ID).Error)
	assert.Equal(t, database.InvitationStatusRevoked, updated.Status)
	assert.NotEqual(t, originalToken, updated.Token, "the original invitation link is dead")

	// An accepted invitation cannot be revoked.
	accepted := createStaffInvitation(t, business.ID, "accepted@example.com", "Accepted", database.StaffRoleServer)
	require.NoError(t, database.GetDB().Model(&database.StaffInvitation{}).Where("id = ?", accepted.ID).
		Update("status", database.InvitationStatusAccepted).Error)
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
		{Key: "invitationId", Value: fmt.Sprintf("%d", accepted.ID)},
	}
	c2.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	c2.Set("address", business.OwnerAddress)
	RevokeInvitation(c2)
	assert.Equal(t, http.StatusBadRequest, w2.Code)
}
