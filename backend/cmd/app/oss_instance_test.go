package main

import (
	"os"
	"strings"
	"testing"
)

func TestInstanceRuntimeFromBoot(t *testing.T) {
	cases := []struct {
		name            string
		rpc, provider   string
		telegram        bool
		wantProvider    string
		wantCrypto      bool
		wantTelegramOut bool
	}{
		{name: "log transport", provider: "log", wantProvider: "log"},
		{name: "resolved provider is normalised", provider: " Postmark ", wantProvider: "postmark"},
		{name: "smtp", provider: "SMTP", wantProvider: "smtp"},
		{name: "rpc + telegram", rpc: " https://mainnet.base.org ", telegram: true, provider: "log", wantProvider: "log", wantCrypto: true, wantTelegramOut: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := instanceRuntimeFromBoot(tc.rpc, tc.telegram, tc.provider)
			if rt.EmailProvider != tc.wantProvider || rt.CryptoEnabled != tc.wantCrypto || rt.TelegramEnabled != tc.wantTelegramOut {
				t.Fatalf("got %+v", rt)
			}
		})
	}
}

func TestMainWiresInstanceEndpointAndChecks(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	for _, want := range []string{
		`publicRoutes.GET("/instance", server.GetInstanceInfo)`,
		"runInstanceStartupChecks(productionMode)",
		"server.SetInstanceRuntime(instanceRuntimeFromBoot(rpcURLResolved, telegramWorkerEnabled, emailProviderName))",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("main.go missing %q", want)
		}
	}
}
