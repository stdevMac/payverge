package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/stdevmac/payverge/backend/internal/paymentcontract"
)

func main() {
	eventsPath := flag.String("events", "", "path to go test -json events")
	sourceSHA := flag.String("source-sha", "", "full source Git SHA")
	outPath := flag.String("out", "", "output proof path")
	flag.Parse()

	if *eventsPath == "" || *sourceSHA == "" || *outPath == "" {
		fatalf("-events, -source-sha, and -out are required")
	}
	events, err := os.Open(*eventsPath)
	if err != nil {
		fatalf("open events: %v", err)
	}
	defer events.Close()

	proof, err := paymentcontract.BuildHermeticProof(events, *sourceSHA, time.Now().UTC())
	if err != nil {
		fatalf("build proof: %v", err)
	}
	encoded, err := json.MarshalIndent(proof, "", "  ")
	if err != nil {
		fatalf("encode proof: %v", err)
	}
	encoded = append(encoded, '\n')
	if err := os.WriteFile(*outPath, encoded, 0o600); err != nil {
		fatalf("write proof: %v", err)
	}
}

func fatalf(format string, args ...interface{}) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
