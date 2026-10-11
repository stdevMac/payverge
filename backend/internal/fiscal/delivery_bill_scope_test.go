package fiscal

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// billScopedStubDispatcher records the bill the receipt email was attributed
// to. The production dispatcher uses it to stamp the send with the bill's
// business so it counts against that tenant's outbound email budget.
type billScopedStubDispatcher struct {
	*stubDispatcher
	scopedCalls    int
	lastBusinessID uint
	lastBillID     uint
}

func (s *billScopedStubDispatcher) SendReceiptEmailForBill(businessID, billID uint, to []string, businessName, receiptType, receiptNumber, totalDisplay, language string, pdf []byte) error {
	s.mu.Lock()
	s.scopedCalls++
	s.lastBusinessID = businessID
	s.lastBillID = billID
	s.mu.Unlock()
	return s.SendReceiptEmail(to, businessName, receiptType, receiptNumber, totalDisplay, language, pdf)
}

func TestDeliveryWorker_ReceiptEmailIsAttributedToTheBill(t *testing.T) {
	disp := &billScopedStubDispatcher{stubDispatcher: &stubDispatcher{}}
	w, db, prov := newDeliveryTestWorker(t, disp)
	prov.IssueFn = authorizedIssueProvider()

	seedBillWithCustomer(t, db, 31, 32, 330, "guest@example.com", 12100)
	seedReadySettings(t, db, 31)
	enqueueIssueJob(t, db, 31, 32)

	runIssueThenDelivery(t, w, disp)

	require.Equal(t, 1, disp.scopedCalls, "the bill-scoped sender must be preferred")
	require.Equal(t, 1, disp.emailCalls)
	require.Equal(t, uint(31), disp.lastBusinessID)
	require.Equal(t, uint(32), disp.lastBillID)
}
