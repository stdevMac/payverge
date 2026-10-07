package main

import (
	"log"
	"os"

	"github.com/stdevmac/payverge/backend/internal/activation"
)

func main() {
	if err := activation.ExportDashboardPlan(os.Stdout); err != nil {
		log.Fatal(err)
	}
}
