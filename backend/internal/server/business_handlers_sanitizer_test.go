package server

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestStripOwnerOnlyFieldsForStaff_DropsWalletFieldsForStaff(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("token_type", "staff")

	req := &UpdateBusinessRequest{
		Name:           "New Name",
		SettlementAddr: "0xnewsettlement",
		TippingAddr:    "0xnewtipping",
	}
	stripOwnerOnlyFieldsForStaff(req, c)

	assert.Equal(t, "New Name", req.Name, "non-wallet fields preserved")
	assert.Equal(t, "", req.SettlementAddr, "settlement address stripped for staff")
	assert.Equal(t, "", req.TippingAddr, "tipping address stripped for staff")
}

func TestStripOwnerOnlyFieldsForStaff_PreservesForOwner(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("address", "0xowner") // Web3 owner — token_type empty

	req := &UpdateBusinessRequest{
		SettlementAddr: "0xnewsettlement",
		TippingAddr:    "0xnewtipping",
	}
	stripOwnerOnlyFieldsForStaff(req, c)

	assert.Equal(t, "0xnewsettlement", req.SettlementAddr, "owner can update settlement address")
	assert.Equal(t, "0xnewtipping", req.TippingAddr, "owner can update tipping address")
}

func TestStripOwnerOnlyFieldsForStaff_ReportsSkippedFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("token_type", "staff")

	enabled := true
	name := "Sage"
	req := &UpdateBusinessRequest{
		Name:           "Legit rename",
		SettlementAddr: "0x3333333333333333333333333333333333333333",
		TippingAddr:    "0x4444444444444444444444444444444444444444",
		AiEnabled:      &enabled,
		AiName:         &name,
	}

	skipped := stripOwnerOnlyFieldsForStaff(req, c)

	assert.ElementsMatch(t,
		[]string{"settlement_address", "tipping_address", "ai_enabled", "ai_name"},
		skipped)
	assert.Empty(t, req.SettlementAddr)
	assert.Empty(t, req.TippingAddr)
	assert.Nil(t, req.AiEnabled)
	assert.Nil(t, req.AiName)
	assert.Equal(t, "Legit rename", req.Name)
}

func TestStripOwnerOnlyFieldsForStaff_NoFalsePositives(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Owner token: nothing stripped, nothing reported.
	wOwner := httptest.NewRecorder()
	cOwner, _ := gin.CreateTestContext(wOwner)
	ownerReq := &UpdateBusinessRequest{SettlementAddr: "0x5555555555555555555555555555555555555555"}
	assert.Empty(t, stripOwnerOnlyFieldsForStaff(ownerReq, cOwner))
	assert.NotEmpty(t, ownerReq.SettlementAddr)

	// Staff token with no owner-only fields submitted: no false positives.
	wStaff := httptest.NewRecorder()
	cStaff, _ := gin.CreateTestContext(wStaff)
	cStaff.Set("token_type", "staff")
	staffReq := &UpdateBusinessRequest{Name: "rename only"}
	assert.Empty(t, stripOwnerOnlyFieldsForStaff(staffReq, cStaff))
}
