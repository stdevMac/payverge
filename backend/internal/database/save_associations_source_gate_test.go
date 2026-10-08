package database

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// saveAssociationsAllowlist is the exact set of production call sites still
// permitted to invoke GORM .Save( without .Omit(clause.Associations) on the
// same chain. Keys are "rel/path.go:FunctionName" (exact). Every entry needs a
// one-line justification — typically "model has no association struct fields"
// or a documented legitimate association cascade.
//
// New unguarded Saves fail this gate. Prefer adding .Omit(clause.Associations)
// (house default for any model that can carry a preloaded belongs-to/has-many).
// Only allowlist when Omit would break intentional association writes, or when
// the model structurally cannot carry associations.
//
// Root cause (PG-9 / theme T-1): GORM SaveBeforeAssociations upserts non-zero
// belongs-to rows. A partial Business preload (owner_address="", user_id=nil)
// re-INSERT hits businesses_has_canonical_owner (SQLSTATE 23514).
var saveAssociationsAllowlist = map[string]string{
	// --- models with no GORM association struct fields ---
	"internal/auth/service.go:RotateVerificationToken":                    "UserAuth has no association struct fields",
	"internal/auth/service.go:LinkWallet":                                 "UserAuth has no association struct fields",
	"internal/auth/service.go:issuePasswordResetToken":                    "UserAuth has no association struct fields",
	"internal/auth/service.go:SaveAuth":                                   "UserAuth has no association struct fields",
	"internal/auththrottle/auththrottle.go:Record":                        "AuthAttempt has no association struct fields",
	"internal/llm/budget_store.go:RecordConsumed":                         "aiSpendReservation has no association struct fields",
	"internal/llm/budget_store.go:RecordUnpricedConsumed":                 "aiSpendReservation has no association struct fields",
	"internal/llm/budget_store.go:ReconcileConsumed":                      "aiDailySpend / aiSpendReservation have no association struct fields",
	"internal/llm/budget_store.go:Release":                                "aiDailySpend / aiSpendReservation have no association struct fields",
	"internal/llm/budget_store.go:ExpireStale":                            "aiDailySpend / aiSpendReservation have no association struct fields",
	"internal/database/currency.go:SaveTranslation":                       "Translation has no association struct fields",
	"internal/database/business.go:UpdateBusiness":                        "Business has only embedded value types; no belongs-to/has-many associations",
	"internal/database/platform_settings.go:SetPlatformSetting":           "PlatformSettings has no association struct fields",
	"internal/handlers/accounting_categories.go:UpdateAccountingCategory": "AccountingCategory has no association struct fields",
	"internal/handlers/accounting_recurring.go:UpdateRecurringTemplate":   "RecurringEntryTemplate has no association struct fields",
	"internal/handlers/printer_handlers.go:UpdatePrinter":                 "Printer has no association struct fields",
	"internal/server/manager_pin.go:UpdateCompVoidAudit":                  "CompVoidAudit has no association struct fields",
	"internal/crm/handlers.go:ToggleCRM":                                  "saves Business which has no association struct fields",
	"internal/fiscal/repository.go:UpsertSettings":                        "BusinessFiscalSettings has no association struct fields",
}

// TestSaveAssociationsSourceGate freezes the production GORM Save surface
// against unguarded association upserts (PG-9 / T-1 / B-2).
//
// Walks backend/internal/**/*.go (skipping *_test.go and vendored/generated
// dirs). Every .Save( call whose enclosing chain does not contain
// Omit(clause.Associations) must appear in saveAssociationsAllowlist under
// "rel/path.go:FunctionName".
func TestSaveAssociationsSourceGate(t *testing.T) {
	root := sourceGateBackendRoot(t)
	internalRoot := filepath.Join(root, "internal")

	var violations []string
	err := filepath.WalkDir(internalRoot, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			name := d.Name()
			switch name {
			case "vendor", "node_modules", ".git", "testdata", "generated":
				return filepath.SkipDir
			}
			if strings.HasPrefix(name, ".") && name != "." {
				return filepath.SkipDir
			}
			return nil
		}

		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)

		fset := token.NewFileSet()
		file, parseErr := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if parseErr != nil {
			// Fall back to line scan if parse fails (shouldn't for production Go).
			return scanSaveLinesFallback(path, rel, &violations)
		}

		ast.Inspect(file, func(n ast.Node) bool {
			fn, ok := n.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				return true
			}
			funcName := fn.Name.Name
			key := rel + ":" + funcName

			ast.Inspect(fn.Body, func(inner ast.Node) bool {
				call, ok := inner.(*ast.CallExpr)
				if !ok {
					return true
				}
				if !isSaveCall(call) {
					return true
				}
				if chainHasOmitAssociations(call) {
					return true
				}
				pos := fset.Position(call.Pos())
				if _, allowed := saveAssociationsAllowlist[key]; allowed {
					return true
				}
				violations = append(violations, fmt.Sprintf(
					"%s:%d: unguarded GORM .Save( in %s — add .Omit(clause.Associations) before Save, or justify an allowlist entry in saveAssociationsAllowlist (key %q)",
					rel, pos.Line, funcName, key,
				))
				return true
			})
			return false // already walked body
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walk backend/internal: %v", err)
	}

	// Also fail if allowlist entries are stale (no longer match any bare Save).
	// Collect observed bare keys to report unused allowlist entries as noise.
	// (Re-walk is expensive; do a second lightweight pass only for unused keys.)
	observedBare := collectBareSaveKeys(t, root, internalRoot)
	var stale []string
	for key := range saveAssociationsAllowlist {
		if !observedBare[key] {
			stale = append(stale, key)
		}
	}

	if len(violations) > 0 {
		t.Fatalf("Save associations source-gate violations (%d):\n  %s\n\nRule: every production GORM .Save( on a chain without .Omit(clause.Associations) must either add Omit (house default — prevents SaveBeforeAssociations upserting preloaded belongs-to rows such as partial Business) or be explicitly allowlisted in saveAssociationsAllowlist with a one-line justification.\nSee PG-9 / theme T-1 (reservations.go already uses Omit).",
			len(violations), strings.Join(violations, "\n  "))
	}
	if len(stale) > 0 {
		t.Fatalf("saveAssociationsAllowlist has %d stale entries (no matching bare .Save in that function):\n  %s\n\nRemove them or restore the call site.",
			len(stale), strings.Join(stale, "\n  "))
	}
}

// isSaveCall reports whether call is a selector call to Save(...).
func isSaveCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	return sel.Sel.Name == "Save"
}

// chainHasOmitAssociations walks the call receiver chain leftward looking for
// .Omit(clause.Associations) (or Omit with any arg containing Associations).
func chainHasOmitAssociations(call *ast.CallExpr) bool {
	// call is X.Save(...); walk X if it is itself a call chain.
	cur := call.Fun
	for {
		sel, ok := cur.(*ast.SelectorExpr)
		if !ok {
			return false
		}
		// If this selector is Omit, inspect its CallExpr parent — but we only
		// have the selector of Save. Walk into sel.X:
		//   (...).Omit(...).Save(...)  →  Save's X is CallExpr Omit
		//   db.Save(...)               →  Save's X is Ident
		switch x := sel.X.(type) {
		case *ast.CallExpr:
			if omitSel, ok := x.Fun.(*ast.SelectorExpr); ok && omitSel.Sel.Name == "Omit" {
				if callArgsReferenceAssociations(x.Args) {
					return true
				}
			}
			// Continue walking left of the Omit (or other) call.
			cur = x.Fun
		case *ast.SelectorExpr:
			cur = x
		default:
			return false
		}
	}
}

// callArgsReferenceAssociations is true when any arg is clause.Associations
// (Ident Associations or SelectorExpr clause.Associations).
func callArgsReferenceAssociations(args []ast.Expr) bool {
	for _, arg := range args {
		switch a := arg.(type) {
		case *ast.Ident:
			if a.Name == "Associations" {
				return true
			}
		case *ast.SelectorExpr:
			if a.Sel.Name == "Associations" {
				return true
			}
		}
	}
	return false
}

// collectBareSaveKeys returns the set of "rel:Func" keys that currently have
// at least one unguarded .Save( (used to detect stale allowlist entries).
func collectBareSaveKeys(t *testing.T, root, internalRoot string) map[string]bool {
	t.Helper()
	out := make(map[string]bool)
	_ = filepath.WalkDir(internalRoot, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			if d != nil && d.IsDir() {
				name := d.Name()
				switch name {
				case "vendor", "node_modules", ".git", "testdata", "generated":
					return filepath.SkipDir
				}
				if strings.HasPrefix(name, ".") && name != "." {
					return filepath.SkipDir
				}
			}
			return walkErr
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		fset := token.NewFileSet()
		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return nil
		}
		ast.Inspect(file, func(n ast.Node) bool {
			fn, ok := n.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				return true
			}
			ast.Inspect(fn.Body, func(inner ast.Node) bool {
				call, ok := inner.(*ast.CallExpr)
				if !ok || !isSaveCall(call) || chainHasOmitAssociations(call) {
					return true
				}
				out[rel+":"+fn.Name.Name] = true
				return true
			})
			return false
		})
		return nil
	})
	return out
}

// scanSaveLinesFallback is used only if a file fails to parse.
func scanSaveLinesFallback(path, rel string, violations *[]string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		if strings.Contains(line, ".Save(") && !strings.Contains(line, "Omit(clause.Associations)") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "//") {
				continue
			}
			*violations = append(*violations,
				rel+":"+strconv.Itoa(lineNo)+": unguarded GORM .Save( (parse fallback) — add .Omit(clause.Associations) or allowlist")
		}
	}
	return scanner.Err()
}
