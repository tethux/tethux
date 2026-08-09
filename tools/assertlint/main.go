// Command assertlint rejects expensive invariant scans that are not guarded by
// the compile-time assert.Enabled constant.
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	roots := os.Args[1:]
	if len(roots) == 0 {
		roots = []string{"."}
	}

	failed := false
	for _, root := range roots {
		if err := lintRoot(root, func(pos token.Position, message string) {
			failed = true
			fmt.Fprintf(os.Stderr, "%s: %s\n", pos, message)
		}); err != nil {
			fmt.Fprintln(os.Stderr, err)
			failed = true
		}
	}
	if failed {
		os.Exit(1)
	}
}

func lintRoot(root string, report func(token.Position, string)) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != root && (entry.Name() == ".git" || entry.Name() == "vendor" || entry.Name() == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		lintNode(fset, file, false, report)
		if enforcesStructuredErrors(path) {
			lintErrorConstructors(fset, file, report)
		}
		return nil
	})
}

func lintNode(fset *token.FileSet, node ast.Node, guarded bool, report func(token.Position, string)) {
	if node == nil {
		return
	}

	if ifStmt, ok := node.(*ast.IfStmt); ok {
		lintNode(fset, ifStmt.Init, guarded, report)
		lintNode(fset, ifStmt.Cond, guarded, report)
		lintNode(fset, ifStmt.Body, guarded || isAssertEnabled(ifStmt.Cond), report)
		lintNode(fset, ifStmt.Else, guarded, report)
		return
	}

	if call, ok := node.(*ast.CallExpr); ok && isExpensiveAssertion(call.Fun) && !guarded {
		report(fset.Position(call.Pos()), fmt.Sprintf(
			"expensive assertion %s must be guarded by if assert.Enabled",
			selectorName(call.Fun),
		))
	}

	ast.Inspect(node, func(child ast.Node) bool {
		if child == nil || child == node {
			return true
		}
		lintNode(fset, child, guarded, report)
		return false
	})
}

func enforcesStructuredErrors(path string) bool {
	path = strings.TrimPrefix(filepath.ToSlash(path), "./")
	return !strings.HasSuffix(path, "_test.go") &&
		!strings.Contains(path, "/errs/") &&
		(strings.HasPrefix(path, "bridge/") ||
			strings.HasPrefix(path, "virt/"))
}

func lintErrorConstructors(fset *token.FileSet, node ast.Node, report func(token.Position, string)) {
	ast.Inspect(node, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkg, ok := selector.X.(*ast.Ident)
		if !ok {
			return true
		}

		forbidden := pkg.Name == "fmt" && selector.Sel.Name == "Errorf" ||
			pkg.Name == "errors" && selector.Sel.Name == "New"
		if forbidden {
			report(fset.Position(call.Pos()), fmt.Sprintf(
				"use the package's structured errs.New or errs.Wrap instead of %s.%s",
				pkg.Name, selector.Sel.Name,
			))
		}
		return true
	})
}

func isAssertEnabled(expr ast.Expr) bool {
	switch value := expr.(type) {
	case *ast.ParenExpr:
		return isAssertEnabled(value.X)
	case *ast.SelectorExpr:
		ident, ok := value.X.(*ast.Ident)
		return ok && ident.Name == "assert" && value.Sel.Name == "Enabled"
	case *ast.BinaryExpr:
		return value.Op == token.LAND &&
			(isAssertEnabled(value.X) || isAssertEnabled(value.Y))
	default:
		return false
	}
}

func isExpensiveAssertion(expr ast.Expr) bool {
	name := selectorName(expr)
	return strings.HasPrefix(name, "assertValid") || strings.HasPrefix(name, "assertInvariants")
}

func selectorName(expr ast.Expr) string {
	selector, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	return selector.Sel.Name
}
