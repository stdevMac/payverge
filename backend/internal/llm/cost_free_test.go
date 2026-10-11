package llm

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCallBudget_SelfHostedUnpricedModelReservesAtZeroWithoutShutdownLatch(t *testing.T) {
	db := requireBudgetPostgres(t)
	t.Cleanup(func() { SetUnpricedModelsFree(false) })
	ctx := context.Background()
	budget := NewCallBudget(NewBudgetStore(db), 10_000_000)
	req := GenerateRequest{
		Model: "llama3.1:8b", Feature: "director", BusinessID: 71,
		Messages: []Message{{Role: RoleUser, Text: "hello"}}, MaxTokens: 500,
	}

	SetUnpricedModelsFree(false)
	if _, _, err := budget.ReserveForRequest(ctx, req); !errors.Is(err, ErrUnpricedModel) {
		t.Fatalf("default reserve error = %v, want ErrUnpricedModel", err)
	}

	SetUnpricedModelsFree(true)
	id, applied, err := budget.ReserveForRequest(ctx, req)
	requireNoError(t, err)
	if !applied {
		t.Fatal("expected a $0 audit reservation")
	}
	requireNoError(t, budget.FinalizeRequest(ctx, id, &Response{
		Model: "llama3.1:8b", Usage: Usage{PromptTokens: 10, CompletionTokens: 20},
	}, req.Model))
	shutdown, err := budget.Store.UnpricedModelShutdownActive(ctx)
	requireNoError(t, err)
	if shutdown {
		t.Fatal("free-mode finalize must not latch the unpriced-model shutdown")
	}
	fin, reserved, err := budget.Store.CommittedSpend(ctx, 71, "", time.Now())
	requireNoError(t, err)
	if fin != 0 || reserved != 0 {
		t.Fatalf("committed spend = %d/%d, want 0/0", fin, reserved)
	}
}
