package blockchain

import (
	"context"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

// A JSON-RPC stub whose eth_getBlockByNumber returns a block whose reported
// hash cannot be reproduced from its (partial) header fields. The adapter must
// return the node-reported hash, not a locally recomputed one, or every L2
// receipt would look non-canonical.
func newBlockByNumberStub(t *testing.T, result string) (*ethclient.Client, *[]string) {
	t.Helper()
	var params []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			ID     json.RawMessage   `json:"id"`
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			t.Errorf("bad request: %v", err)
			return
		}
		if req.Method != "eth_getBlockByNumber" {
			t.Errorf("unexpected method %s", req.Method)
		}
		for _, p := range req.Params {
			params = append(params, string(p))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":` + string(req.ID) + `,"result":` + result + `}`))
	}))
	t.Cleanup(srv.Close)
	client, err := ethclient.Dial(srv.URL)
	if err != nil {
		t.Fatalf("dial stub: %v", err)
	}
	t.Cleanup(client.Close)
	return client, &params
}

func TestEthRPCCanonicalBlockHash_ReturnsNodeReportedHash(t *testing.T) {
	want := common.HexToHash("0x1111111111111111111111111111111111111111111111111111111111111111")
	client, params := newBlockByNumberStub(t, `{"number":"0x10","hash":"`+want.Hex()+`","parentHash":"0x2222222222222222222222222222222222222222222222222222222222222222"}`)

	got, err := ethRPC{client}.CanonicalBlockHash(context.Background(), big.NewInt(16))
	if err != nil {
		t.Fatalf("CanonicalBlockHash: %v", err)
	}
	if got != want {
		t.Fatalf("hash = %s, want node-reported %s", got.Hex(), want.Hex())
	}
	if len(*params) != 2 || (*params)[0] != `"0x10"` || (*params)[1] != "false" {
		t.Fatalf("params = %v, want [\"0x10\" false]", *params)
	}
}

func TestEthRPCCanonicalBlockHash_MissingBlockIsAnError(t *testing.T) {
	client, _ := newBlockByNumberStub(t, `null`)
	if _, err := (ethRPC{client}).CanonicalBlockHash(context.Background(), big.NewInt(16)); err == nil {
		t.Fatal("expected an error for a block the node does not have")
	}
}
