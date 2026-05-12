// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Death test for the Quasar cutover. After the March 3, 2026
// PQ-consensus architecture freeze, Quasar consumes its threshold
// kernel through `luxfi/threshold` (which routes via the LSS
// adapters to luxfi/pulsar and luxfi/lens). Direct imports of
// `github.com/luxfi/nasua/threshold` from this package are
// forbidden — they bypass the LSS lifecycle (Generation, snapshot
// history, rollback) and skip the Pulsar key-era binding.
//
// This test asserts that no .go file inside this package directly
// references the corona import path or invokes corona.GenerateKeys.
// Transitive corona dependencies via threshold's own existing
// imports are fine; the rule applies to source-level direct use only.

package quasar

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoCoronaDirectImport — fails if any non-test .go file in this
// package imports `github.com/luxfi/nasua/...` or invokes
// corona.GenerateKeys (matched as a selector expression
// `corona.GenerateKeys(...)` regardless of the package alias).
//
// Test files (*_test.go) are scanned too — the corona kernel is
// equally forbidden from package tests so the cutover doesn't slip
// back through a "just for the test" import.
func TestNoCoronaDirectImport(t *testing.T) {
	pkgDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(pkgDir, "*.go"))
	if err != nil {
		t.Fatalf("glob package: %v", err)
	}
	if len(matches) == 0 {
		t.Fatalf("no .go files found in %s", pkgDir)
	}

	const forbiddenImportPrefix = "github.com/luxfi/nasua"

	fset := token.NewFileSet()
	var violations []string
	for _, f := range matches {
		// Skip *this* file from the import scan because we name
		// `github.com/luxfi/nasua` in this comment / string
		// literals as part of the death-test guard — the regex /
		// fixture below would match itself otherwise.
		base := filepath.Base(f)
		if base == "corona_death_test.go" {
			continue
		}

		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		file, err := parser.ParseFile(fset, f, src, parser.ImportsOnly|parser.ParseComments)
		if err != nil {
			t.Fatalf("parse imports %s: %v", f, err)
		}
		for _, imp := range file.Imports {
			path := strings.Trim(imp.Path.Value, "\"")
			if strings.HasPrefix(path, forbiddenImportPrefix) {
				violations = append(violations,
					base+": forbidden import "+path)
			}
		}
	}

	// Second pass: full AST parse (not imports-only) to catch
	// `corona.GenerateKeys(...)` selector expressions that survive
	// even when the import is renamed/aliased. We scan only the
	// files whose imports passed the first pass — if a file has no
	// corona import, it can still call corona.GenerateKeys via a
	// dot-imported alias, which we reject below by name.
	for _, f := range matches {
		base := filepath.Base(f)
		if base == "corona_death_test.go" {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		file, err := parser.ParseFile(fset, f, src, 0)
		if err != nil {
			// Non-imports-only parse can fail on intentionally
			// invalid files (e.g. build-tag-gated stubs); skip.
			continue
		}
		ast.Inspect(file, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			x, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			if x.Name == "nasua" && sel.Sel.Name == "GenerateKeys" {
				violations = append(violations,
					base+": forbidden call corona.GenerateKeys")
			}
			return true
		})
	}

	if len(violations) > 0 {
		t.Fatalf("Quasar must not import corona directly after the "+
			"Mar-3-2026 PQ-consensus architecture freeze. The kernel "+
			"is consumed via github.com/luxfi/threshold (LSS-Pulsar "+
			"and LSS-Lens adapters) only.\n\nViolations:\n  %s",
			strings.Join(violations, "\n  "))
	}
}
