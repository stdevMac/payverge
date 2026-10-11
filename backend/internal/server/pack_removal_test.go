package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The pack economy is deleted, not disabled. This walks the backend source and
// fails if any of it grew back.
func TestImagePackEconomyIsGone(t *testing.T) {
	banned := []string{
		// Pack economy
		"CreateImagePackCheckoutSession",
		"processImagePackPayment",
		"CreditImagePackPurchase",
		"ClawBackImagePackRefund",
		"PurchaseImagePack",
		"addPackCreditsTx",
		"image_pack",
		// Credit model and its read side
		"AIImageCredit",
		"ai_image_credits",
		"ImageCreditReservation",
		"ReserveImageCredit",
		"RefundImageCredit",
		"GetImageCreditSnapshot",
		"GetImageCredits",
		"ErrNoImageCredits",
		"image_credits_exhausted",
		"ai_image_monthly_limit",
	}

	root, err := filepath.Abs("../..")
	require.NoError(t, err)

	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if info.Name() == "vendor" || info.Name() == "bin" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.HasSuffix(path, "pack_removal_test.go") {
			return nil
		}
		src, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for _, token := range banned {
			assert.NotContains(t, string(src), token,
				"%s still references retired pack symbol %q", path, token)
		}
		return nil
	})
	require.NoError(t, err)
}
