package server

import (
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/s3"
)

const collidingPublicContactEmail = "public-contact@example.test"

func createCanonicalFileOwnerBusiness(t *testing.T, name string, ownerUserID uint, contactEmail string) *database.Business {
	t.Helper()

	business := createOwnedBusiness(t, fmt.Sprintf("0x%040x", ownerUserID), name)
	business.UserID = &ownerUserID
	business.Email = contactEmail
	require.NoError(t, database.GetDB().Save(business).Error)
	return business
}

func setOAuthFilePrincipal(c *gin.Context, userID uint, email string, businessID uint) {
	c.Set("user_id", userID)
	c.Set("email", email)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", businessID)}}
}

func TestFileMutations_EmailCollisionDoesNotGrantTenantAccess(t *testing.T) {
	const (
		victimOwnerUserID = uint(7001)
		attackerUserID    = uint(7002)
	)

	t.Run("public upload", func(t *testing.T) {
		setupStaffHandlerTestDB(t)
		victim := createCanonicalFileOwnerBusiness(t, "canonical-public-upload-victim", victimOwnerUserID, collidingPublicContactEmail)

		var mutations atomic.Int32
		setPublicUploader(t, func(_ *multipart.FileHeader, _, _ string, _ ...s3.UploadOption) (string, error) {
			mutations.Add(1)
			return "https://storage.example.test/object", nil
		})

		req := newUploadContractRequest(t, map[string]string{
			"business_id": fmt.Sprintf("%d", victim.ID),
			"folder":      "menus",
		}, "menu.pdf", "application/pdf", minimalPDF(4096))
		c, recorder := newUploadUserIDContext(t, attackerUserID, req)
		setOAuthFilePrincipal(c, attackerUserID, collidingPublicContactEmail, victim.ID)

		UploadFile(c)

		assert.Equal(t, http.StatusForbidden, recorder.Code)
		assert.Zero(t, mutations.Load(), "denied public upload must not mutate S3")
	})

	t.Run("protected upload", func(t *testing.T) {
		setupStaffHandlerTestDB(t)
		victim := createCanonicalFileOwnerBusiness(t, "canonical-protected-upload-victim", victimOwnerUserID, collidingPublicContactEmail)

		var mutations atomic.Int32
		setContractUploader(t, func(_ *multipart.FileHeader, _, _ string, _ ...s3.UploadOption) (string, error) {
			mutations.Add(1)
			return "https://storage.example.test/protected-object", nil
		})

		req := newUploadContractRequest(t, map[string]string{
			"businessId": fmt.Sprintf("%d", victim.ID),
		}, "contract.pdf", "application/pdf", minimalPDF(4096))
		c, recorder := newUploadUserIDContext(t, attackerUserID, req)
		setOAuthFilePrincipal(c, attackerUserID, collidingPublicContactEmail, victim.ID)

		UploadFileProtected(c)

		assert.Equal(t, http.StatusForbidden, recorder.Code)
		assert.Zero(t, mutations.Load(), "denied protected upload must not mutate S3")
	})

	t.Run("logo upload", func(t *testing.T) {
		setupStaffHandlerTestDB(t)
		victim := createCanonicalFileOwnerBusiness(t, "canonical-logo-upload-victim", victimOwnerUserID, collidingPublicContactEmail)

		var mutations atomic.Int32
		setPublicUploader(t, func(_ *multipart.FileHeader, _, _ string, _ ...s3.UploadOption) (string, error) {
			mutations.Add(1)
			return "https://storage.example.test/logo", nil
		})

		req := newUploadContractRequest(t, nil, "logo.png", "image/png", minimalPNG(4096))
		c, recorder := newUploadUserIDContext(t, attackerUserID, req)
		setOAuthFilePrincipal(c, attackerUserID, collidingPublicContactEmail, victim.ID)

		UploadBusinessLogo(c)

		assert.Equal(t, http.StatusForbidden, recorder.Code)
		assert.Zero(t, mutations.Load(), "denied logo upload must not mutate S3")
	})

	t.Run("delete", func(t *testing.T) {
		setupStaffHandlerTestDB(t)
		victim := createCanonicalFileOwnerBusiness(t, "canonical-delete-victim", victimOwnerUserID, collidingPublicContactEmail)

		fakeS3, mutations := initFakeDeleteS3(t)
		defer fakeS3.Close()

		key := fmt.Sprintf("businesses/%d/menus/menu.pdf", victim.ID)
		req := httptest.NewRequest(
			http.MethodDelete,
			fmt.Sprintf("/api/v1/inside/businesses/%d/uploads?key=%s", victim.ID, key),
			nil,
		)
		gin.SetMode(gin.TestMode)
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = req
		setOAuthFilePrincipal(c, attackerUserID, collidingPublicContactEmail, victim.ID)

		DeleteUploadedFile(c)

		assert.Equal(t, http.StatusForbidden, recorder.Code)
		assert.Zero(t, mutations.Load(), "denied delete must not mutate S3")
	})
}

func TestDeleteUploadedFile_CrossTenantObjectKeyFailsClosed(t *testing.T) {
	setupStaffHandlerTestDB(t)
	const (
		attackerUserID = uint(7101)
		victimUserID   = uint(7102)
	)
	attacker := createCanonicalFileOwnerBusiness(t, "cross-key-attacker", attackerUserID, "attacker@example.test")
	victim := createCanonicalFileOwnerBusiness(t, "cross-key-victim", victimUserID, "victim@example.test")

	fakeS3, mutations := initFakeDeleteS3(t)
	defer fakeS3.Close()

	victimKey := fmt.Sprintf("businesses/%d/contracts/private.pdf", victim.ID)
	req := httptest.NewRequest(
		http.MethodDelete,
		fmt.Sprintf("/api/v1/inside/businesses/%d/uploads?key=%s", attacker.ID, victimKey),
		nil,
	)
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = req
	setOAuthFilePrincipal(c, attackerUserID, "attacker@example.test", attacker.ID)

	DeleteUploadedFile(c)

	assert.Equal(t, http.StatusForbidden, recorder.Code)
	assert.Zero(t, mutations.Load(), "cross-tenant key denial must not mutate S3")
}

func TestBusinessLookup_EmailCollisionDoesNotExposeOrClaimTenant(t *testing.T) {
	const (
		victimOwnerUserID = uint(7201)
		attackerUserID    = uint(7202)
	)

	t.Run("business list", func(t *testing.T) {
		setupStaffHandlerTestDB(t)
		_ = createCanonicalFileOwnerBusiness(t, "business-list-victim", victimOwnerUserID, collidingPublicContactEmail)

		gin.SetMode(gin.TestMode)
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/inside/businesses", nil)
		c.Set("token_type", "user")
		c.Set("user_id", float64(attackerUserID))
		c.Set("email", collidingPublicContactEmail)

		GetMyBusinesses(c)

		assert.Equal(t, http.StatusOK, recorder.Code)
		var businesses []database.Business
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &businesses))
		assert.Empty(t, businesses, "public contact email must not reveal another owner's tenant")
	})

	t.Run("legacy detail does not auto claim", func(t *testing.T) {
		setupStaffHandlerTestDB(t)
		victim := createOwnedBusiness(t, "0xVictimOwner", "business-detail-victim")
		victim.Email = collidingPublicContactEmail
		require.NoError(t, database.GetDB().Save(victim).Error)

		gin.SetMode(gin.TestMode)
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/inside/businesses/%d", victim.ID), nil)
		c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", victim.ID)}}
		c.Set("token_type", "user")
		c.Set("user_id", float64(attackerUserID))
		c.Set("email", collidingPublicContactEmail)

		GetBusiness(c)

		assert.Equal(t, http.StatusOK, recorder.Code)
		assert.NotContains(t, recorder.Body.String(), collidingPublicContactEmail)
		assert.NotContains(t, recorder.Body.String(), victim.SettlementAddr)
		var persisted database.Business
		require.NoError(t, database.GetDB().First(&persisted, victim.ID).Error)
		assert.Nil(t, persisted.UserID, "public contact email must never auto-claim a business")
	})
}

func initFakeDeleteS3(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()

	mutations := &atomic.Int32{}
	fakeS3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			mutations.Add(1)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	// s3.Init rewires the process-wide stores; restore them for later tests.
	t.Cleanup(s3.SetStores(s3.PublicStore(), s3.ProtectedStore()))
	_, err := s3.Init(context.Background(), s3.Config{
		Driver: s3.DriverS3,
		S3: s3.S3Settings{
			// An underscore is intentionally not DNS-compatible, which makes the AWS
			// SDK use path-style addressing against the local fake endpoint.
			Bucket:          "test_bucket",
			AccessKey:       "test-access-key",
			SecretKey:       "test-secret-key",
			Region:          "us-east-1",
			Endpoint:        fakeS3.URL,
			ProtectedBucket: "test_protected",
		},
	})
	require.NoError(t, err)
	// Init's protected-exposure probe writes and deletes a canary object;
	// count only the mutations the handler under test makes.
	mutations.Store(0)
	return fakeS3, mutations
}
