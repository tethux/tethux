// Command repolint enforces repository-specific Go source policies.
//
// Rules:
//
//   - expensive-assertion:
//     Expensive invariant checks must be guarded by assert.Enabled.
//     This rule supports automatic fixing with -fix.
//
//   - structured-error:
//     Production code under storage/ and bridge/ must use structured error
//     constructors instead of fmt.Errorf or errors.New.
//
// Suppression:
//
//	//repolint:ignore expensive-assertion -- startup-only validation
//	s.assertValidLocked()
//
// Run:
//
//	go run ./tools/repolint .
//	go run ./tools/repolint -fix .
package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	ruleExpensiveAssertion = "expensive-assertion"
	ruleStructuredError    = "structured-error"
)

type diagnostic struct {
	Rule       string
	Position   token.Position
	Message    string
	Suggestion string
}

type reporter func(diagnostic)

type options struct {
	fix bool
}

func main() {
	var opts options

	flag.BoolVar(
		&opts.fix,
		"fix",
		false,
		"apply safe automatic fixes",
	)

	flag.Usage = func() {
		// #nosec G705 -- this writes plain text to the CLI's terminal output.
		_, writeErr := fmt.Fprintf(
			flag.CommandLine.Output(),
			"Usage: %s [options] [path ...]\n\n"+
				"Enforces repository-specific Go source policies.\n\n"+
				"If no paths are supplied, the current directory is checked.\n\n"+
				"Suppression:\n"+
				"  //repolint:ignore RULE -- reason\n\n",
			filepath.Base(os.Args[0]),
		)
		if writeErr != nil {
			os.Exit(1)
		}

		flag.PrintDefaults()
	}

	flag.Parse()

	roots := flag.Args()
	if len(roots) == 0 {
		roots = []string{"."}
	}

	failed := false

	report := func(d diagnostic) {
		failed = true

		_, writeErr := fmt.Fprintf(
			os.Stderr,
			"%s: [%s] %s\n",
			d.Position,
			d.Rule,
			d.Message,
		)
		if writeErr != nil {
			return
		}

		if d.Suggestion != "" {
			_, writeErr = fmt.Fprintf(os.Stderr, "  fix: %s\n", d.Suggestion)
			if writeErr != nil {
				return
			}
		}
	}

	for _, root := range roots {
		lintErr := lintRoot(root, opts, report)
		if lintErr != nil {
			_, writeErr := fmt.Fprintf(os.Stderr, "repolint: %v\n", lintErr)
			if writeErr != nil {
				failed = true
			}
			failed = true
		}
	}

	if failed && !opts.fix {
		os.Exit(1)
	}
}

func lintRoot(root string, opts options, report reporter) error {
	return filepath.WalkDir(
		root,
		func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return fmt.Errorf("walk %q: %w", path, walkErr)
			}

			if entry.IsDir() {
				if path != root && shouldSkipDir(entry.Name()) {
					return filepath.SkipDir
				}

				return nil
			}

			if filepath.Ext(path) != ".go" {
				return nil
			}

			return lintFile(path, opts, report)
		},
	)
}

func lintFile(path string, opts options, report reporter) error {
	// #nosec G304 -- the repository root is explicitly selected by the operator.
	src, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %q: %w", path, err)
	}

	fset := token.NewFileSet()

	file, err := parser.ParseFile(
		fset,
		path,
		src,
		parser.ParseComments|parser.SkipObjectResolution,
	)
	if err != nil {
		return fmt.Errorf("parse %q: %w", path, err)
	}

	if opts.fix {
		changed := fixAssertions(file, fset)

		if changed {
			var out bytes.Buffer

			formatErr := format.Node(&out, fset, file)
			if formatErr != nil {
				return fmt.Errorf("format fixed %q: %w", path, formatErr)
			}

			// #nosec G306 -- preserve normal source-file permissions when fixing.
			writeErr := os.WriteFile(path, out.Bytes(), 0o644)
			if writeErr != nil {
				return fmt.Errorf("write fixed %q: %w", path, writeErr)
			}

			// Parse the updated file again so diagnostics refer to the actual
			// resulting source.
			src = out.Bytes()
			fset = token.NewFileSet()

			file, err = parser.ParseFile(
				fset,
				path,
				src,
				parser.ParseComments|parser.SkipObjectResolution,
			)
			if err != nil {
				return fmt.Errorf("parse fixed %q: %w", path, err)
			}
		}
	}

	lintAssertions(fset, file, false, report)

	if enforcesStructuredErrors(path) {
		lintErrorConstructors(fset, file, report)
	}

	return nil
}

func shouldSkipDir(name string) bool {
	switch name {
	case ".git", "vendor", "node_modules":
		return true
	default:
		return false
	}
}

// fixAssertions wraps eligible expensive assertion expression statements in:
//
//	if assert.Enabled {
//		...
//	}
//
// Only direct expression statements are rewritten. More complicated
// expressions are left diagnostic-only.
func fixAssertions(file *ast.File, _ *token.FileSet) bool {
	changed := false

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}

		if fixStatementList(&fn.Body.List, false) {
			changed = true
		}
	}

	return changed
}

func fixStatementList(stmts *[]ast.Stmt, guarded bool) bool {
	changed := false
	list := *stmts

	for i, stmt := range list {
		switch value := stmt.(type) {
		case *ast.ExprStmt:
			if guarded {
				continue
			}

			call, ok := value.X.(*ast.CallExpr)
			if !ok || !isExpensiveAssertion(call.Fun) {
				continue
			}

			list[i] = &ast.IfStmt{
				Cond: &ast.SelectorExpr{
					X:   ast.NewIdent("assert"),
					Sel: ast.NewIdent("Enabled"),
				},
				Body: &ast.BlockStmt{
					List: []ast.Stmt{value},
				},
			}

			changed = true

		case *ast.IfStmt:
			bodyGuarded := guarded || isAssertEnabled(value.Cond)

			if fixStatementList(&value.Body.List, bodyGuarded) {
				changed = true
			}

			switch elseNode := value.Else.(type) {
			case *ast.BlockStmt:
				if fixStatementList(&elseNode.List, guarded) {
					changed = true
				}

			case *ast.IfStmt:
				if fixIfStatement(elseNode, guarded) {
					changed = true
				}
			}

		case *ast.ForStmt:
			if fixStatementList(&value.Body.List, guarded) {
				changed = true
			}

		case *ast.RangeStmt:
			if fixStatementList(&value.Body.List, guarded) {
				changed = true
			}

		case *ast.SwitchStmt:
			for _, clauseNode := range value.Body.List {
				clause, ok := clauseNode.(*ast.CaseClause)
				if !ok {
					continue
				}

				if fixStatementList(&clause.Body, guarded) {
					changed = true
				}
			}

		case *ast.TypeSwitchStmt:
			for _, clauseNode := range value.Body.List {
				clause, ok := clauseNode.(*ast.CaseClause)
				if !ok {
					continue
				}

				if fixStatementList(&clause.Body, guarded) {
					changed = true
				}
			}

		case *ast.SelectStmt:
			for _, clauseNode := range value.Body.List {
				clause, ok := clauseNode.(*ast.CommClause)
				if !ok {
					continue
				}

				if fixStatementList(&clause.Body, guarded) {
					changed = true
				}
			}

		case *ast.LabeledStmt:
			if fixStatement(&value.Stmt, guarded) {
				changed = true
			}
		}
	}

	*stmts = list
	return changed
}

func fixIfStatement(stmt *ast.IfStmt, guarded bool) bool {
	changed := false

	bodyGuarded := guarded || isAssertEnabled(stmt.Cond)

	if fixStatementList(&stmt.Body.List, bodyGuarded) {
		changed = true
	}

	switch elseNode := stmt.Else.(type) {
	case *ast.BlockStmt:
		if fixStatementList(&elseNode.List, guarded) {
			changed = true
		}

	case *ast.IfStmt:
		if fixIfStatement(elseNode, guarded) {
			changed = true
		}
	}

	return changed
}

func fixStatement(stmt *ast.Stmt, guarded bool) bool { //nolint:gocritic // the interface value must be replaced in place
	switch value := (*stmt).(type) {
	case *ast.ExprStmt:
		if guarded {
			return false
		}

		call, ok := value.X.(*ast.CallExpr)
		if !ok || !isExpensiveAssertion(call.Fun) {
			return false
		}

		*stmt = &ast.IfStmt{
			Cond: &ast.SelectorExpr{
				X:   ast.NewIdent("assert"),
				Sel: ast.NewIdent("Enabled"),
			},
			Body: &ast.BlockStmt{
				List: []ast.Stmt{value},
			},
		}

		return true

	case *ast.BlockStmt:
		return fixStatementList(&value.List, guarded)

	case *ast.IfStmt:
		return fixIfStatement(value, guarded)

	case *ast.ForStmt:
		return fixStatementList(&value.Body.List, guarded)

	case *ast.RangeStmt:
		return fixStatementList(&value.Body.List, guarded)

	default:
		return false
	}
}

func lintAssertions(
	fset *token.FileSet,
	node ast.Node,
	guarded bool,
	report reporter,
) {
	if node == nil {
		return
	}

	if ifStmt, ok := node.(*ast.IfStmt); ok {
		lintAssertions(fset, ifStmt.Init, guarded, report)
		lintAssertions(fset, ifStmt.Cond, guarded, report)

		bodyGuarded := guarded || isAssertEnabled(ifStmt.Cond)
		lintAssertions(fset, ifStmt.Body, bodyGuarded, report)

		lintAssertions(fset, ifStmt.Else, guarded, report)

		return
	}

	if call, ok := node.(*ast.CallExpr); ok &&
		isExpensiveAssertion(call.Fun) &&
		!guarded {

		name := selectorName(call.Fun)

		report(diagnostic{
			Rule:     ruleExpensiveAssertion,
			Position: fset.Position(call.Pos()),
			Message: fmt.Sprintf(
				"expensive assertion %q runs without an assert.Enabled guard",
				name,
			),
			Suggestion: fmt.Sprintf(
				"run repolint with -fix, or wrap the call in `if assert.Enabled { %s(...) }`",
				name,
			),
		})
	}

	ast.Inspect(node, func(child ast.Node) bool {
		if child == nil || child == node {
			return true
		}

		lintAssertions(fset, child, guarded, report)
		return false
	})
}

func enforcesStructuredErrors(path string) bool {
	path = normalizedRepoPath(path)

	if strings.HasSuffix(path, "_test.go") {
		return false
	}

	if isErrsPackage(path) {
		return false
	}

	return hasPathPrefix(path, "storage") ||
		hasPathPrefix(path, "bridge")
}

func normalizedRepoPath(path string) string {
	clean := filepath.Clean(path)

	if abs, err := filepath.Abs(clean); err == nil {
		if cwd, cwdErr := os.Getwd(); cwdErr == nil {
			if rel, relErr := filepath.Rel(cwd, abs); relErr == nil {
				clean = rel
			}
		}
	}

	clean = filepath.ToSlash(clean)

	return strings.TrimPrefix(clean, "./")
}

func hasPathPrefix(path, prefix string) bool {
	return path == prefix ||
		strings.HasPrefix(path, prefix+"/")
}

func isErrsPackage(path string) bool {
	path = "/" + strings.Trim(filepath.ToSlash(path), "/") + "/"

	return strings.Contains(path, "/errs/")
}

func lintErrorConstructors(
	fset *token.FileSet,
	file *ast.File,
	report reporter,
) {
	imports := importAliases(file)

	ast.Inspect(file, func(node ast.Node) bool {
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

		importPath, ok := imports[pkg.Name]
		if !ok {
			return true
		}

		if ignored(file, fset, call.Pos(), ruleStructuredError) {
			return true
		}

		switch {
		case importPath == "fmt" &&
			selector.Sel.Name == "Errorf":

			suggestion := "use the package's structured errs.New constructor"

			if errorfWrapsCause(call) {
				suggestion = "format contains %w; use the package's structured errs.Wrap constructor"
			}

			report(diagnostic{
				Rule:       ruleStructuredError,
				Position:   fset.Position(call.Pos()),
				Message:    "fmt.Errorf is forbidden in this package",
				Suggestion: suggestion,
			})

		case importPath == "errors" &&
			selector.Sel.Name == "New":

			report(diagnostic{
				Rule:     ruleStructuredError,
				Position: fset.Position(call.Pos()),
				Message:  "errors.New is forbidden in this package",
				Suggestion: "use the package's structured " +
					"errs.New constructor",
			})
		}

		return true
	})
}

func errorfWrapsCause(call *ast.CallExpr) bool {
	if len(call.Args) == 0 {
		return false
	}

	lit, ok := call.Args[0].(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return false
	}

	formatString, err := strconv.Unquote(lit.Value)
	if err != nil {
		return false
	}

	return containsErrorWrapVerb(formatString)
}

// containsErrorWrapVerb detects an unescaped %w.
//
// %%w is literal text and must not count as an error-wrapping directive.
func containsErrorWrapVerb(formatString string) bool {
	for i := 0; i < len(formatString); i++ {
		if formatString[i] != '%' {
			continue
		}

		if i+1 >= len(formatString) {
			continue
		}

		if formatString[i+1] == '%' {
			i++
			continue
		}

		for j := i + 1; j < len(formatString); j++ {
			switch formatString[j] {
			case 'w':
				return true

			case 'v', 's', 'q', 'd', 'x', 'X', 'f', 'e', 'E',
				'g', 'G', 'o', 'O', 'c', 'p', 'T', 't', 'b',
				'U':
				i = j
				goto next
			}
		}

	next:
	}

	return false
}

func importAliases(file *ast.File) map[string]string {
	imports := make(map[string]string, len(file.Imports))

	for _, spec := range file.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}

		name := filepath.Base(importPath)

		if spec.Name != nil {
			name = spec.Name.Name
		}

		if name == "." || name == "_" {
			continue
		}

		imports[name] = importPath
	}

	return imports
}

// ignored reports whether the nearest preceding comment contains:
//
//	//repolint:ignore RULE -- optional reason
//
// A suppression applies only to the immediately following source line.
func ignored(
	file *ast.File,
	fset *token.FileSet,
	pos token.Pos,
	rule string,
) bool {
	line := fset.Position(pos).Line

	for _, group := range file.Comments {
		endLine := fset.Position(group.End()).Line

		if endLine != line-1 {
			continue
		}

		for _, comment := range group.List {
			ignoredRule, ok := parseIgnoreDirective(comment.Text)
			if ok && (ignoredRule == rule || ignoredRule == "all") {
				return true
			}
		}
	}

	return false
}

func parseIgnoreDirective(comment string) (string, bool) {
	comment = strings.TrimSpace(comment)

	comment = strings.TrimPrefix(comment, "//")
	comment = strings.TrimSpace(comment)

	const prefix = "repolint:ignore"

	if !strings.HasPrefix(comment, prefix) {
		return "", false
	}

	rest := strings.TrimSpace(
		strings.TrimPrefix(comment, prefix),
	)

	if rest == "" {
		return "", false
	}

	rule, _, _ := strings.Cut(rest, " ")
	rule, _, _ = strings.Cut(rule, "--")

	rule = strings.TrimSpace(rule)

	if rule == "" {
		return "", false
	}

	return rule, true
}

func isAssertEnabled(expr ast.Expr) bool {
	switch value := expr.(type) {
	case *ast.ParenExpr:
		return isAssertEnabled(value.X)

	case *ast.SelectorExpr:
		ident, ok := value.X.(*ast.Ident)

		return ok &&
			ident.Name == "assert" &&
			value.Sel.Name == "Enabled"

	case *ast.BinaryExpr:
		return value.Op == token.LAND &&
			(isAssertEnabled(value.X) ||
				isAssertEnabled(value.Y))

	default:
		return false
	}
}

func isExpensiveAssertion(expr ast.Expr) bool {
	name := selectorName(expr)

	return strings.HasPrefix(name, "assertValid") ||
		strings.HasPrefix(name, "assertInvariants")
}

func selectorName(expr ast.Expr) string {
	selector, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return ""
	}

	return selector.Sel.Name
}
