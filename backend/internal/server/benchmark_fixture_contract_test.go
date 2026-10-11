package server_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"
)

func TestServerBenchmarksSharePostgresSchemaSetup(t *testing.T) {
	files, err := filepath.Glob("*_bench_test.go")
	if err != nil {
		t.Fatalf("glob server benchmarks: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no server benchmark files found")
	}

	var starts int
	for _, file := range files {
		parsed, parseErr := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", file, parseErr)
		}
		starts += countPackageCalls(parsed, "genesisdb", "Start")
		starts += countPackageCalls(parsed, "testperf", "StartPostgres")
		starts += countPackageCalls(parsed, "testperf", "StartIsolatedPostgres")
	}

	// genesisdb.Start builds the production schema (genesis baseline plus
	// pending migrations), so one start is also the one schema setup.
	if starts != 1 {
		t.Fatalf("server benchmarks must start one shared Postgres fixture, found %d starts", starts)
	}
}

func countPackageCalls(file *ast.File, packageName, functionName string) int {
	count := 0
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != functionName {
			return true
		}
		identifier, ok := selector.X.(*ast.Ident)
		if ok && identifier.Name == packageName {
			count++
		}
		return true
	})
	return count
}
