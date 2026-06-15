package conformance

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// targets lists package directories (relative to the repo root) that
// must pass the conformance checks.  Packages are added here once they
// have been migrated off direct etcd / mutex state.
var targets = []string{
	// NOTE: migrations are in progress; only packages known to be clean
	// are listed here.  Expanding this list is how we ratchet the
	// checklist forward.
	"internal/resources",
	"internal/reconciler",
	"internal/platform/timing",
	"internal/platform/errs",
	"internal/logging",
}

// forbiddenImports are fully-qualified import paths that must not
// appear in the targets above.
var forbiddenImports = map[string]string{
	`"go.etcd.io/etcd/client/v3"`: "use internal/platform/store.ResourceStore[T] instead of a raw etcd client",
	`"database/sql"`:              "persistence must go through a repository / Store[T]",
}

func TestConformance_NoRawPersistenceInTargets(t *testing.T) {
	root := repoRoot(t)
	for _, rel := range targets {
		dir := filepath.Join(root, filepath.FromSlash(rel))
		fset := token.NewFileSet()
		pkgs, err := parser.ParseDir(fset, dir, skipTests, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", rel, err)
		}
		for _, pkg := range pkgs {
			for fname, f := range pkg.Files {
				for _, imp := range f.Imports {
					if reason, bad := forbiddenImports[imp.Path.Value]; bad {
						t.Errorf("%s: forbidden import %s (%s)",
							filepath.Base(fname), imp.Path.Value, reason)
					}
				}
			}
		}
	}
}

// handlerDirs lists directories that contain handler code to check.
var handlerDirs = []string{
	"internal/storage/admin",
	"internal/iam/admin",
	"internal/antivirus",
	"internal/scanner",
	"internal/apibuilder",
}

// TestConformance_HandlersAreThin flags handler structs that still hold
// a raw etcd client or *sql.DB field.  This is informational for now.
func TestConformance_HandlersAreThin(t *testing.T) {
	root := repoRoot(t)

	var violations []string
	for _, rel := range handlerDirs {
		dir := filepath.Join(root, filepath.FromSlash(rel))
		fset := token.NewFileSet()
		pkgs, err := parser.ParseDir(fset, dir, skipTests, parser.ParseComments)
		if err != nil {
			continue
		}
		for _, pkg := range pkgs {
			for fname, f := range pkg.Files {
				ast.Inspect(f, func(n ast.Node) bool {
					st, ok := n.(*ast.StructType)
					if !ok || st.Fields == nil {
						return true
					}
					for _, field := range st.Fields.List {
						typeText := exprText(field.Type)
						switch {
						case strings.Contains(typeText, "clientv3.Client"),
							strings.Contains(typeText, "sql.DB"):
							violations = append(violations,
								filepath.Base(fname)+": field type "+typeText)
						}
					}
					return true
				})
			}
		}
	}

	if len(violations) > 0 {
		t.Logf("handlers still holding raw persistence (informational, %d):", len(violations))
		for _, v := range violations {
			t.Logf("  - %s", v)
		}
	}
}

// TestConformance_HandlersUseTypedErrors flags handlers that use ad-hoc
// gin.H{"error": ...} instead of the standard internal/errors package.
func TestConformance_HandlersUseTypedErrors(t *testing.T) {
	root := repoRoot(t)

	var violations []string
	for _, rel := range handlerDirs {
		dir := filepath.Join(root, filepath.FromSlash(rel))
		fset := token.NewFileSet()
		pkgs, err := parser.ParseDir(fset, dir, skipTests, parser.ParseComments)
		if err != nil {
			continue
		}
		for _, pkg := range pkgs {
			for fname, f := range pkg.Files {
				ast.Inspect(f, func(n ast.Node) bool {
					call, ok := n.(*ast.CallExpr)
					if !ok {
						return true
					}
					if isGinHErrorCall(call) {
						pos := fset.Position(call.Pos())
						violations = append(violations,
							filepath.Base(fname)+":"+strconv.Itoa(pos.Line)+" uses gin.H{\"error\":...}")
					}
					return true
				})
			}
		}
	}
	if len(violations) > 0 {
		t.Logf("handlers using ad-hoc error format (informational, %d):", len(violations))
		for _, v := range violations {
			t.Logf("  - %s", v)
		}
	}
}

// isGinHErrorCall checks if a call expression is gin.H{"error": ...}.
func isGinHErrorCall(call *ast.CallExpr) bool {
	// This is a simplified check - looks for composite literals with "error" key.
	cl, ok := call.Fun.(*ast.CompositeLit)
	if !ok {
		return false
	}
	// Check if it's gin.H type.
	if ident, ok := cl.Type.(*ast.Ident); ok {
		if ident.Name != "H" {
			return false
		}
	}
	for _, elt := range cl.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if lit, ok := kv.Key.(*ast.BasicLit); ok && lit.Value == `"error"` {
			return true
		}
	}
	return false
}
