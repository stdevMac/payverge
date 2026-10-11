package database

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Public guest bill resolution must use unguessable public_token only.
// bill_number is operator-facing and must not act as a guest capability.

var (
	// Files/dirs that may still resolve bills by bill_number (operator, demo, seed).
	billNumberLookupAllowPathPrefixes = []string{
		"internal/demo/",
		"internal/database/business.go", // operator list filters; guest token resolvers are scanned below
		"cmd/",                          // not scanned (we walk internal/ only)
	}

	// Production guest/public resolver + handler files that must never match by bill_number.
	guestPublicBillSourceFiles = []string{
		"internal/database/business.go",
		"internal/database/bill_split.go",
		"internal/handlers/payments.go",
		"internal/handlers/splitting.go",
		"internal/handlers/guest_orders.go",
		"internal/handlers/plugin_handlers.go",
		"internal/server/guest_handlers.go",
		"internal/server/public_guest_access_helpers.go",
		"internal/server/bill_fiscal_customer_handler.go",
		"cmd/app/main.go",
	}

	requiredPublicTokenResolvers = []string{
		"GetPublicBillByToken",
		"GetGuestBillIDByToken",
		"GetGuestBillScopeByToken",
		"GetGuestPluginCheckoutBillByToken",
	}

	forbiddenLegacyGuestResolvers = []string{
		"GetPublicGuestBillByNumber",
		"GetGuestBillIDByNumber",
		"GetGuestBillScopeByNumber",
		"GetGuestPluginCheckoutBillByNumber",
	}

	reBillIdentifierWhere = regexp.MustCompile(`\bBillIdentifierWhere\b`)
	reORBillNumberClause  = regexp.MustCompile(`(?i)public_token\s*=\s*\?\s*OR\s+(?:bills\.)?bill_number\s*=\s*\?`)
	reBillNumberWhere     = regexp.MustCompile(`(?i)(?:bills\.)?bill_number\s*=\s*\?`)
	rePublicTokenWhere    = regexp.MustCompile(`\bPublicBillTokenWhere\b`)
)

func backendRoot(t *testing.T) string {
	t.Helper()
	// This file lives at backend/internal/database/ → walk up to backend/
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	// Tests may run with cwd = backend or backend/internal/database
	for i := 0; i < 6; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("could not locate backend root (go.mod) from %s", dir)
	return ""
}

func repoRelative(backendRoot, abs string) string {
	rel, err := filepath.Rel(filepath.Dir(backendRoot), abs) // payverge-relative
	if err != nil {
		return abs
	}
	return filepath.ToSlash(rel)
}

func isAllowlistedBillNumberPath(rel string) bool {
	rel = filepath.ToSlash(rel)
	// The allowlist covers demo generators and operator packages that filter
	// bills by number for staff dashboards. Guest resolvers are scanned
	// separately and must not WHERE on bill_number.
	for _, p := range billNumberLookupAllowPathPrefixes {
		if strings.Contains(rel, p) {
			return true
		}
	}
	// Authenticated/operator packages that legitimately filter by bill_number
	// for staff dashboards, analytics, fiscal, ledger, etc.
	operatorFragments := []string{
		"/demo/",
		"payment_ledger.go",
		"db_config.go",
		"table.go",
		"fiscal/",
		"admin_",
		"analytics",
		"perf/",
	}
	for _, f := range operatorFragments {
		if strings.Contains(rel, f) {
			return true
		}
	}
	return false
}

func TestPublicBillCapabilityContract(t *testing.T) {
	root := backendRoot(t)
	internalDir := filepath.Join(root, "internal")

	// --- Required public token resolvers must exist ---
	businessSrc, err := os.ReadFile(filepath.Join(root, "internal", "database", "business.go"))
	if err != nil {
		t.Fatalf("read business.go: %v", err)
	}
	businessText := string(businessSrc)

	if !strings.Contains(businessText, `const PublicBillTokenWhere = "bills.public_token = ?"`) {
		t.Errorf("PublicBillTokenWhere must be defined as bills.public_token = ? (single bind)")
	}
	if reBillIdentifierWhere.MatchString(businessText) {
		t.Errorf("BillIdentifierWhere must be removed; guest paths must use PublicBillTokenWhere")
	}
	if reORBillNumberClause.MatchString(businessText) {
		t.Errorf("OR (public_token|bill_number) guest capability clause must not appear in business.go")
	}

	for _, name := range requiredPublicTokenResolvers {
		if !regexp.MustCompile(`func\s+` + name + `\s*\(`).MatchString(businessText) {
			t.Errorf("required public token resolver %s is missing", name)
		}
		if !strings.HasPrefix(name, "GetPublicBillByToken") && !strings.HasPrefix(name, "GetGuest") {
			t.Errorf("resolver %s does not follow GetPublicBillByToken / GetGuest...ByToken naming", name)
		}
		if !strings.Contains(name, "ByToken") {
			t.Errorf("resolver %s must be token-named (…ByToken)", name)
		}
	}
	for _, name := range forbiddenLegacyGuestResolvers {
		if regexp.MustCompile(`func\s+` + name + `\s*\(`).MatchString(businessText) {
			t.Errorf("legacy guest resolver %s must be renamed to a …ByToken function", name)
		}
	}

	// Unscoped bill_number resolution must not come back. Operator detail loads
	// by id (GetBillByID); guest routes resolve public_token only.
	if regexp.MustCompile(`func\s+GetBillByNumber\s*\(`).MatchString(businessText) {
		t.Errorf("GetBillByNumber must stay deleted; bills must not resolve by bill_number")
	}

	// --- Walk internal/ production sources ---
	var violations []string
	err = filepath.WalkDir(internalDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			name := d.Name()
			if name == "testdata" || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel := repoRelative(root, path)
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		text := string(data)

		if reBillIdentifierWhere.MatchString(text) {
			violations = append(violations, rel+": references removed BillIdentifierWhere")
		}
		if reORBillNumberClause.MatchString(text) {
			violations = append(violations, rel+": contains public_token OR bill_number guest capability clause")
		}

		// Guest public files: no bill_number = ? for capability resolution.
		isGuestFile := false
		for _, g := range guestPublicBillSourceFiles {
			if strings.HasSuffix(rel, g) || strings.Contains(rel, g) {
				isGuestFile = true
				break
			}
		}
		if !isGuestFile {
			return nil
		}

		scanText := text

		// Allow bill_number as a SELECT projection / JSON field / response metadata,
		// but not as a WHERE capability: look for Where(...bill_number = ?) patterns.
		if reBillNumberWhere.MatchString(scanText) {
			// Ignore pure string literals that are not SQL WHEREs if only in comments —
			// still flag any production occurrence in guest files.
			for _, line := range strings.Split(scanText, "\n") {
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, "//") {
					continue
				}
				if reBillNumberWhere.MatchString(line) {
					// SELECT "bill_number" is fine; WHERE bill_number = ? is not.
					if strings.Contains(line, "Select(") || strings.Contains(line, `"bill_number"`) && !strings.Contains(line, "bill_number =") {
						continue
					}
					if strings.Contains(line, "bill_number = ?") || strings.Contains(line, "bills.bill_number = ?") {
						violations = append(violations, rel+": guest/public path resolves by bill_number: "+trimmed)
					}
				}
			}
		}

		// Guest resolvers in database package must use PublicBillTokenWhere.
		if strings.HasSuffix(rel, "internal/database/business.go") || strings.HasSuffix(rel, "internal/database/bill_split.go") {
			// Public token resolvers / split public resolvers should reference the constant.
			if strings.Contains(scanText, "GetPublicBillByToken") || strings.Contains(scanText, "GetGuestBillIDByToken") ||
				strings.Contains(scanText, "GetBillSplitStateByNumber") || strings.Contains(scanText, "GetGuestBillSplitSharesByNumber") {
				if !rePublicTokenWhere.MatchString(scanText) {
					violations = append(violations, rel+": public guest resolvers must use PublicBillTokenWhere")
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk internal: %v", err)
	}

	// Also scan main.go public route params (outside internal/).
	mainPath := filepath.Join(root, "cmd", "app", "main.go")
	if mainSrc, err := os.ReadFile(mainPath); err == nil {
		mainText := string(mainSrc)
		if strings.Contains(mainText, `"/guest/bill/:bill_number"`) || strings.Contains(mainText, `"/guest/bill/:bill_number/`) {
			violations = append(violations, "cmd/app/main.go: public guest bill routes must use :bill_token not :bill_number")
		}
		if reBillIdentifierWhere.MatchString(mainText) {
			violations = append(violations, "cmd/app/main.go: references BillIdentifierWhere")
		}
	}

	// AST check: no file under internal (except allowlist) should declare BillIdentifierWhere.
	fset := token.NewFileSet()
	_ = filepath.WalkDir(internalDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return walkErr
		}
		f, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return nil // skip unparseable
		}
		ast.Inspect(f, func(n ast.Node) bool {
			vs, ok := n.(*ast.ValueSpec)
			if !ok {
				return true
			}
			for _, name := range vs.Names {
				if name.Name == "BillIdentifierWhere" {
					violations = append(violations, repoRelative(root, path)+": still declares BillIdentifierWhere")
				}
			}
			return true
		})
		return nil
	})

	if len(violations) > 0 {
		t.Fatalf("public bill capability contract violations (%d):\n  - %s",
			len(violations), strings.Join(violations, "\n  - "))
	}

	// Sanity: core guest resolver files must not be blanket-allowlisted for
	// bill_number WHERE resolution (business.go is dual-purpose and scanned
	// for bill_number WHERE; main.go is route wiring only).
	for _, g := range []string{
		"internal/handlers/payments.go",
		"internal/handlers/splitting.go",
		"internal/handlers/guest_orders.go",
		"internal/server/guest_handlers.go",
	} {
		if isAllowlistedBillNumberPath(g) {
			t.Fatalf("guest file %s is incorrectly allowlisted for bill_number lookup", g)
		}
	}
}
