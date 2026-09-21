package main

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

type Sentinel struct {
	Name    string
	Message string
}

type ErrorDialect struct {
	New  []string
	Wrap []string
}

type ErrorDomain struct {
	Directory   string
	ImportPath  string
	PackageName string
	Dialect     ErrorDialect
	Sentinels   []Sentinel

	aliases        map[string]int
	functionKinds  map[string]map[string]int
	operationKinds map[string]map[string]int
	causeKinds     map[string]map[string]int
	providers      map[string]map[string]int
}

type parsedGoFile struct {
	path string
	fset *token.FileSet
	file *ast.File
}

type genericError struct {
	message   string
	cause     ast.Expr
	targets   []formatArgument
	operation string
	function  string
	domain    *ErrorDomain
}

func fixStructuredErrors(root string) error {
	moduleRoot, modulePath, err := findModule(root)
	if err != nil {
		return err
	}

	files, err := parseRepository(moduleRoot)
	if err != nil {
		return err
	}

	domains, err := discoverErrorDomains(moduleRoot, modulePath, files)
	if err != nil {
		return err
	}

	learnErrorUsages(files, domains)

	changed := make(map[string]*parsedGoFile)
	newSentinels := make(map[*ErrorDomain][]Sentinel)
	syntheticDomains := make(map[string]*ErrorDomain)

	for _, unit := range files {
		if !pathWithinRoot(root, unit.path) || !enforcesStructuredErrors(unit.path) {
			continue
		}

		domain := domainForFile(moduleRoot, modulePath, unit.path, domains)
		if domain == nil {
			continue
		}
		if _, err := os.Stat(filepath.Join(domain.Directory, "errors.go")); errors.Is(err, os.ErrNotExist) {
			if existing := syntheticDomains[domain.Directory]; existing != nil {
				domain = existing
			} else {
				syntheticDomains[domain.Directory] = domain
			}
		}

		if applyStructuredFixes(unit, domain, newSentinels) {
			changed[unit.path] = unit
		}
	}

	for domain, sentinels := range newSentinels {
		if len(sentinels) == 0 {
			continue
		}

		if err := updateErrorDomain(domain, sentinels); err != nil {
			return err
		}
	}

	paths := make([]string, 0, len(changed))
	for path := range changed {
		paths = append(paths, path)
	}
	slices.Sort(paths)

	for _, path := range paths {
		unit := changed[path]
		pruneUnusedStandardImports(unit.file)

		var output bytes.Buffer
		if err := format.Node(&output, unit.fset, unit.file); err != nil {
			return fmt.Errorf("format structured-error fix %q: %w", path, err)
		}

		// #nosec G306 -- fixed Go files use normal repository permissions.
		if err := os.WriteFile(path, output.Bytes(), 0o644); err != nil {
			return fmt.Errorf("write structured-error fix %q: %w", path, err)
		}
	}

	return nil
}

func findModule(root string) (moduleRoot, modulePath string, resultErr error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", "", fmt.Errorf("resolve root %q: %w", root, err)
	}
	if info, statErr := os.Stat(abs); statErr == nil && !info.IsDir() {
		abs = filepath.Dir(abs)
	}

	for dir := abs; ; dir = filepath.Dir(dir) {
		// #nosec G304 -- the operator explicitly selects the repository root.
		data, readErr := os.ReadFile(filepath.Join(dir, "go.mod"))
		if readErr == nil {
			for _, line := range strings.Split(string(data), "\n") {
				if value, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
					return dir, strings.TrimSpace(value), nil
				}
			}
			return "", "", fmt.Errorf("go.mod in %q has no module directive", dir)
		}
		if !errors.Is(readErr, os.ErrNotExist) {
			return "", "", fmt.Errorf("read go.mod in %q: %w", dir, readErr)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", "", fmt.Errorf("find go.mod above %q", root)
		}
	}
}

func parseRepository(root string) ([]*parsedGoFile, error) {
	var files []*parsedGoFile
	repository, err := os.OpenRoot(root)
	if err != nil {
		return nil, fmt.Errorf("open repository root %q: %w", root, err)
	}

	walkErr := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != root && shouldSkipDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		relativePath, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return fmt.Errorf("resolve repository path %q: %w", path, relErr)
		}
		src, readErr := repository.ReadFile(relativePath)
		if readErr != nil {
			return fmt.Errorf("read %q: %w", path, readErr)
		}
		fset := token.NewFileSet()
		file, parseErr := parser.ParseFile(fset, path, src, parser.ParseComments|parser.SkipObjectResolution)
		if parseErr != nil {
			return fmt.Errorf("parse %q: %w", path, parseErr)
		}
		files = append(files, &parsedGoFile{path: path, fset: fset, file: file})
		return nil
	})
	closeErr := repository.Close()
	if walkErr != nil {
		return nil, walkErr
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close repository root %q: %w", root, closeErr)
	}
	return files, nil
}

func discoverErrorDomains(root, modulePath string, files []*parsedGoFile) ([]*ErrorDomain, error) {
	var domains []*ErrorDomain
	for _, unit := range files {
		if filepath.Base(unit.path) != "errors.go" || filepath.Base(filepath.Dir(unit.path)) != "errs" {
			continue
		}
		rel, err := filepath.Rel(root, filepath.Dir(unit.path))
		if err != nil {
			return nil, fmt.Errorf("resolve error domain %q: %w", unit.path, err)
		}
		domain := &ErrorDomain{
			Directory: filepath.Dir(unit.path), ImportPath: modulePath + "/" + filepath.ToSlash(rel),
			PackageName: unit.file.Name.Name, aliases: map[string]int{},
			functionKinds: map[string]map[string]int{}, operationKinds: map[string]map[string]int{},
			causeKinds: map[string]map[string]int{}, providers: map[string]map[string]int{},
		}
		domain.Dialect.New = constructorRoles(unit.file, "New")
		domain.Dialect.Wrap = constructorRoles(unit.file, "Wrap")
		domain.Sentinels = parseSentinels(unit.file)
		domains = append(domains, domain)
	}
	slices.SortFunc(domains, func(a, b *ErrorDomain) int { return cmp.Compare(a.Directory, b.Directory) })
	return domains, nil
}

func constructorRoles(file *ast.File, name string) []string {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != name || fn.Type.Params == nil {
			continue
		}
		var roles []string
		for _, field := range fn.Type.Params.List {
			for _, param := range field.Names {
				roles = append(roles, strings.ToLower(param.Name))
			}
		}
		return roles
	}
	return nil
}

func parseSentinels(file *ast.File) []Sentinel {
	var result []Sentinel
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}
		for _, spec := range gen.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range value.Names {
				if !strings.HasPrefix(name.Name, "Err") || i >= len(value.Values) {
					continue
				}
				call, ok := value.Values[i].(*ast.CallExpr)
				if !ok || len(call.Args) != 1 {
					continue
				}
				selector, ok := call.Fun.(*ast.SelectorExpr)
				lit, literal := call.Args[0].(*ast.BasicLit)
				if !ok || selector.Sel.Name != "New" || !literal {
					continue
				}
				message, err := strconv.Unquote(lit.Value)
				if err == nil {
					result = append(result, Sentinel{Name: name.Name, Message: message})
				}
			}
		}
	}
	return result
}

func learnErrorUsages(files []*parsedGoFile, domains []*ErrorDomain) {
	for _, unit := range files {
		imports := importAliases(unit.file)
		ast.Inspect(unit.file, func(node ast.Node) bool {
			fn, ok := node.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				return true
			}
			ast.Inspect(fn.Body, func(child ast.Node) bool {
				call, ok := child.(*ast.CallExpr)
				if !ok {
					return true
				}
				selector, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || (selector.Sel.Name != "New" && selector.Sel.Name != "Wrap") {
					return true
				}
				pkg, pkgOK := selector.X.(*ast.Ident)
				if !pkgOK {
					return true
				}
				domain := domainByImport(domains, imports[pkg.Name])
				if domain == nil {
					return true
				}
				domain.aliases[pkg.Name]++
				roles := domain.Dialect.New
				if selector.Sel.Name == "Wrap" {
					roles = domain.Dialect.Wrap
				}
				kind := argumentSentinel(call.Args, roles)
				if kind == "" {
					return true
				}
				incrementNested(domain.functionKinds, fn.Name.Name, kind)
				if expr := argumentForRole(call.Args, roles, "operation"); expr != nil {
					if text, ok := stringLiteral(expr); ok {
						incrementNested(domain.operationKinds, text, kind)
					}
				}
				if expr := argumentForRole(call.Args, roles, "provider"); expr != nil {
					incrementNested(domain.providers, fn.Name.Name, renderExpr(unit.fset, expr))
				}
				if expr := argumentForRole(call.Args, roles, "cause"); expr != nil {
					if cause := causeKey(expr); cause != "" {
						incrementNested(domain.causeKinds, cause, kind)
					}
				}
				return true
			})
			return false
		})
	}
}

func applyStructuredFixes(unit *parsedGoFile, domain *ErrorDomain, additions map[*ErrorDomain][]Sentinel) bool {
	imports := importAliases(unit.file)
	alias := existingDomainAlias(imports, domain.ImportPath)
	changed := false

	ast.Inspect(unit.file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		generic, ok := analyzeGenericError(
			imports,
			call,
			functionAt(unit.file, call.Pos()),
		)
		if !ok || ignored(unit.file, unit.fset, call.Pos(), ruleStructuredError) {
			return true
		}
		generic.domain = domain
		if alias == "" {
			alias = chooseDomainAlias(unit.file, domain)
			addImport(unit.file, alias, domain.ImportPath)
			imports[alias] = domain.ImportPath
		}
		sentinel := matchSentinel(domain, &generic)
		if sentinel.Name == "" {
			sentinel = newSentinel(domain, generic.message)
			domain.Sentinels = append(domain.Sentinels, sentinel)
			additions[domain] = append(additions[domain], sentinel)
		}
		roles := domain.Dialect.New
		constructor := "New"
		if generic.cause != nil {
			roles = domain.Dialect.Wrap
			constructor = "Wrap"
		}
		if len(roles) == 0 {
			return true
		}
		call.Fun = &ast.SelectorExpr{X: ast.NewIdent(alias), Sel: ast.NewIdent(constructor)}
		call.Args = structuredArguments(unit, &generic, sentinel, roles)
		call.Ellipsis = token.NoPos
		changed = true
		return true
	})
	return changed
}

func analyzeGenericError(imports map[string]string, call *ast.CallExpr, function string) (genericError, bool) {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return genericError{}, false
	}
	pkg, pkgOK := selector.X.(*ast.Ident)
	if !pkgOK {
		return genericError{}, false
	}
	importPath := imports[pkg.Name]
	if importPath == "errors" && selector.Sel.Name == "New" && len(call.Args) == 1 {
		message, literal := stringLiteral(call.Args[0])
		if !literal || normalizeMessage(message) == "" {
			return genericError{}, false
		}
		return genericError{message: message, operation: operationText(message), function: function}, literal
	}
	if importPath != "fmt" || selector.Sel.Name != "Errorf" || len(call.Args) == 0 {
		return genericError{}, false
	}
	formatString, ok := stringLiteral(call.Args[0])
	if !ok {
		return genericError{}, false
	}
	analysis := analyzeFormat(formatString, call.Args[1:])
	if normalizeMessage(analysis.static) == "" {
		return genericError{}, false
	}
	return genericError{message: analysis.static, cause: analysis.cause, targets: analysis.targets, operation: operationText(analysis.static), function: function}, true
}

func functionAt(file *ast.File, pos token.Pos) string {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Pos() <= pos && pos <= fn.End() {
			return fn.Name.Name
		}
	}
	return ""
}

type formatParts struct {
	static  string
	cause   ast.Expr
	targets []formatArgument
}

type formatArgument struct {
	expr ast.Expr
	verb byte
}

func analyzeFormat(value string, args []ast.Expr) formatParts {
	var text strings.Builder
	argIndex := 0
	parts := formatParts{}
	for i := 0; i < len(value); i++ {
		if value[i] != '%' || i+1 >= len(value) {
			text.WriteByte(value[i])
			continue
		}
		if value[i+1] == '%' {
			text.WriteByte('%')
			i++
			continue
		}
		j := i + 1
		for j < len(value) && !unicode.IsLetter(rune(value[j])) {
			j++
		}
		if j >= len(value) {
			text.WriteByte(value[i])
			continue
		}
		verb := value[j]
		if argIndex < len(args) {
			if verb == 'w' {
				parts.cause = args[argIndex]
			} else {
				parts.targets = append(parts.targets, formatArgument{expr: args[argIndex], verb: verb})
			}
		}
		argIndex++
		i = j
		text.WriteByte(' ')
	}
	parts.static = normalizeDisplayMessage(text.String())
	return parts
}

func structuredArguments(unit *parsedGoFile, generic *genericError, sentinel Sentinel, roles []string) []ast.Expr {
	args := make([]ast.Expr, 0, len(roles))
	for _, role := range roles {
		switch role {
		case "provider":
			provider := mostCommon(generic.domain.providers[generic.function])
			if provider != "" {
				if expr, err := parser.ParseExpr(provider); err == nil {
					args = append(args, expr)
					continue
				}
			}
			args = append(args, stringExpr(domainOwner(generic.domain)))
		case "operation":
			operation := mostCommonKindKey(generic.domain.operationKinds, sentinel.Name)
			if operation == "" {
				operation = generic.operation
			}
			args = append(args, stringExpr(operation))
		case "kind":
			args = append(args, &ast.SelectorExpr{X: ast.NewIdent(existingDomainAlias(importAliases(unit.file), generic.domain.ImportPath)), Sel: ast.NewIdent(sentinel.Name)})
		case "target":
			args = append(args, targetExpression(generic))
		case "cause":
			args = append(args, generic.cause)
		default:
			args = append(args, stringExpr(""))
		}
	}
	return args
}

func targetExpression(generic *genericError) ast.Expr {
	if len(generic.targets) == 0 {
		return stringExpr("")
	}
	if len(generic.targets) > 1 {
		args := make([]ast.Expr, 0, len(generic.targets)*2-1)
		for index, target := range generic.targets {
			if index > 0 {
				args = append(args, stringExpr(" "))
			}
			args = append(args, target.expr)
		}
		return &ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: ast.NewIdent("fmt"), Sel: ast.NewIdent("Sprint")},
			Args: args,
		}
	}
	switch generic.targets[0].verb {
	case 'd', 'x', 'X', 'f', 'e', 'E', 'g', 'G', 'o', 'O', 'c', 'p', 'U':
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent("fmt"), Sel: ast.NewIdent("Sprint")}, Args: []ast.Expr{generic.targets[0].expr}}
	default:
		return generic.targets[0].expr
	}
}

func matchSentinel(domain *ErrorDomain, generic *genericError) Sentinel {
	type candidate struct {
		sentinel Sentinel
		score    int
	}
	candidates := make([]candidate, 0, len(domain.Sentinels))
	messageTokens := tokenSet(generic.message)
	action := actionToken(generic.message)
	for _, sentinel := range domain.Sentinels {
		score := 0
		if normalizeMessage(sentinel.Message) == normalizeMessage(generic.message) {
			score += 100
		}
		sentinelTokens := tokenSet(sentinel.Message)
		common := 0
		for token := range messageTokens {
			if sentinelTokens[token] {
				common++
			}
		}
		if len(messageTokens)+len(sentinelTokens) > 0 {
			score += 50 * common * 2 / (len(messageTokens) + len(sentinelTokens))
		}
		if action != "" && action == actionToken(sentinel.Message) {
			score += 45
		}
		name := strings.ToLower(strings.TrimPrefix(sentinel.Name, "Err"))
		if action != "" && strings.Contains(name, action) {
			score += 25
		}
		if domain.functionKinds[generic.function][sentinel.Name] > 0 {
			score += 30
		}
		if domain.causeKinds[causeKey(generic.cause)][sentinel.Name] > 0 {
			score += 35
		}
		operationScore := 0
		for operation, kinds := range domain.operationKinds {
			if kinds[sentinel.Name] == 0 {
				continue
			}
			switch {
			case normalizeMessage(operation) == normalizeMessage(generic.operation):
				operationScore = max(operationScore, 60)
			case actionToken(operation) == actionToken(generic.operation):
				operationScore = max(operationScore, 20)
			}
		}
		score += operationScore
		candidates = append(candidates, candidate{sentinel: sentinel, score: score})
	}
	slices.SortFunc(candidates, func(a, b candidate) int {
		if byScore := cmp.Compare(b.score, a.score); byScore != 0 {
			return byScore
		}
		return cmp.Compare(a.sentinel.Name, b.sentinel.Name)
	})
	if len(candidates) == 0 || candidates[0].score < 45 {
		return Sentinel{}
	}
	if len(candidates) > 1 && candidates[0].score == candidates[1].score {
		return Sentinel{}
	}
	return candidates[0].sentinel
}

func newSentinel(domain *ErrorDomain, message string) Sentinel {
	base := sentinelIdentifier(message)
	name := base
	used := map[string]bool{}
	for _, sentinel := range domain.Sentinels {
		used[sentinel.Name] = true
	}
	for suffix := 2; used[name]; suffix++ {
		name = base + strconv.Itoa(suffix)
	}
	return Sentinel{Name: name, Message: normalizeDisplayMessage(message)}
}

func sentinelIdentifier(message string) string {
	words := strings.Fields(normalizeMessage(message))
	if len(words) >= 2 && words[0] == "failed" && words[1] == "to" {
		words = words[2:]
	}
	ignored := map[string]bool{"storage": true, "object": true, "objects": true, "libvirt": true, "domain": true, "container": true}
	var kept []string
	for _, word := range words {
		if ignored[word] {
			continue
		}
		if word == "configuration" {
			word = "config"
		}
		kept = append(kept, word)
	}
	if len(kept) == 0 {
		kept = []string{"Unknown"}
	}
	var name strings.Builder
	name.WriteString("Err")
	for _, word := range kept {
		runes := []rune(word)
		if len(runes) == 0 {
			continue
		}
		name.WriteRune(unicode.ToUpper(runes[0]))
		name.WriteString(string(runes[1:]))
	}
	return name.String()
}

func updateErrorDomain(domain *ErrorDomain, sentinels []Sentinel) error {
	path := filepath.Join(domain.Directory, "errors.go")
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return createErrorDomain(domain, sentinels)
	}
	// #nosec G304 -- the path belongs to a discovered repository error domain.
	src, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read error domain %q: %w", path, err)
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return fmt.Errorf("parse error domain %q: %w", path, err)
	}
	var target *ast.GenDecl
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if ok && gen.Tok == token.VAR {
			target = gen
			break
		}
	}
	if target == nil {
		target = &ast.GenDecl{Tok: token.VAR, Lparen: 1}
		file.Decls = append([]ast.Decl{target}, file.Decls...)
	}
	for _, sentinel := range sentinels {
		target.Specs = append(target.Specs, &ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(sentinel.Name)}, Values: []ast.Expr{&ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent("errors"), Sel: ast.NewIdent("New")}, Args: []ast.Expr{stringExpr(sentinel.Message)}}}})
	}
	addImport(file, "", "errors")
	var output bytes.Buffer
	if err := format.Node(&output, fset, file); err != nil {
		return fmt.Errorf("format error domain %q: %w", path, err)
	}
	// #nosec G306 -- generated Go files use normal repository permissions.
	if err := os.WriteFile(path, output.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write error domain %q: %w", path, err)
	}
	return nil
}

func createErrorDomain(domain *ErrorDomain, sentinels []Sentinel) error {
	if err := os.MkdirAll(domain.Directory, 0o750); err != nil {
		return fmt.Errorf("create error domain %q: %w", domain.Directory, err)
	}
	provider := domainOwner(domain)
	var fields, identity, newArgs, newCall, wrapArgs, literal string
	switch {
	case slices.Contains(domain.Dialect.New, "operation"):
		fields = "Operation string\n\tKind error\n\tTarget string\n\tCause error"
		identity = "e.Operation"
		newArgs = "operation string, kind error, target string"
		newCall = "operation, kind, target"
		wrapArgs = "operation string, kind error, target string, cause error"
		literal = "Operation: operation, Kind: kind, Target: target, Cause: cause"
	case slices.Contains(domain.Dialect.New, "provider"):
		fields = "Provider string\n\tKind error\n\tTarget string\n\tCause error"
		identity = "e.Provider"
		newArgs = "provider string, kind error, target string"
		newCall = "provider, kind, target"
		wrapArgs = "provider string, kind error, target string, cause error"
		literal = "Provider: provider, Kind: kind, Target: target, Cause: cause"
	default:
		fields = "Provider string\n\tKind error\n\tTarget string\n\tCause error"
		identity = "e.Provider"
		newArgs = "kind error, target string"
		newCall = "kind, target"
		wrapArgs = "kind error, target string, cause error"
		literal = fmt.Sprintf("Provider: %q, Kind: kind, Target: target, Cause: cause", provider)
	}
	var vars strings.Builder
	for _, sentinel := range sentinels {
		fmt.Fprintf(&vars, "\t%s = errors.New(%q)\n", sentinel.Name, sentinel.Message)
	}
	template := fmt.Sprintf(`// Package errs defines stable error categories for %s.
package errs

import (
	"errors"
	"strings"
)

var (
%s)

type OpError struct {
	%s
}

func (e *OpError) Error() string {
	parts := make([]string, 0, 4)
	for _, value := range []string{%s, errorString(e.Kind), e.Target, errorString(e.Cause)} {
		if value != "" { parts = append(parts, value) }
	}
	return strings.Join(parts, ": ")
}

func (e *OpError) Unwrap() []error {
	result := make([]error, 0, 2)
	if e.Kind != nil { result = append(result, e.Kind) }
	if e.Cause != nil { result = append(result, e.Cause) }
	return result
}

func errorString(err error) string {
	if err == nil { return "" }
	return err.Error()
}

func New(%s) error { return Wrap(%s, nil) }

func Wrap(%s) error {
	return &OpError{%s}
}
`, provider, vars.String(), fields, identity, newArgs, newCall, wrapArgs, literal)
	formatted, err := format.Source([]byte(template))
	if err != nil {
		return fmt.Errorf("format new error domain %q: %w", domain.Directory, err)
	}
	path := filepath.Join(domain.Directory, "errors.go")
	// #nosec G306 -- generated Go files use normal repository permissions.
	if err := os.WriteFile(path, formatted, 0o644); err != nil {
		return fmt.Errorf("write new error domain %q: %w", path, err)
	}
	return nil
}

func domainForFile(root, modulePath, path string, domains []*ErrorDomain) *ErrorDomain {
	dir := filepath.Dir(path)
	var best *ErrorDomain
	for _, domain := range domains {
		owner := filepath.Dir(domain.Directory)
		if isPathWithin(owner, dir) && (best == nil || len(owner) > len(filepath.Dir(best.Directory))) {
			best = domain
		}
	}
	if best != nil {
		return best
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return nil
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	var owner string
	var roles []string
	switch {
	case len(parts) > 0 && parts[0] == "storage":
		owner = filepath.Join(root, "storage")
		roles = []string{"provider", "kind", "target"}
	case len(parts) > 0 && parts[0] == "bridge":
		owner = filepath.Join(root, "bridge")
		roles = []string{"operation", "kind", "target"}
	case len(parts) > 2 && parts[0] == "virt" && parts[1] == "hypervisor":
		owner = filepath.Join(root, "virt", "hypervisor", parts[2])
		roles = []string{"kind", "target"}
	default:
		return nil
	}
	dir = filepath.Join(owner, "errs")
	relImport, _ := filepath.Rel(root, dir)
	domain := &ErrorDomain{Directory: dir, ImportPath: modulePath + "/" + filepath.ToSlash(relImport), PackageName: "errs", Dialect: ErrorDialect{New: roles, Wrap: append(slices.Clone(roles), "cause")}, aliases: map[string]int{}, functionKinds: map[string]map[string]int{}, operationKinds: map[string]map[string]int{}, causeKinds: map[string]map[string]int{}, providers: map[string]map[string]int{}}
	return domain
}

func chooseDomainAlias(file *ast.File, domain *ErrorDomain) string {
	preferred := mostCommon(domain.aliases)
	if preferred == "" {
		if strings.Contains(domain.ImportPath, "/storage/errs") {
			preferred = "storageerrs"
		} else {
			preferred = "errs"
		}
	}
	used := importAliases(file)
	if _, exists := used[preferred]; !exists && !identifierExists(file, preferred) {
		return preferred
	}
	for i := 2; ; i++ {
		candidate := preferred + strconv.Itoa(i)
		if _, exists := used[candidate]; !exists && !identifierExists(file, candidate) {
			return candidate
		}
	}
}

func identifierExists(file *ast.File, name string) bool {
	found := false
	ast.Inspect(file, func(node ast.Node) bool {
		ident, ok := node.(*ast.Ident)
		if ok && ident.Name == name {
			found = true
			return false
		}
		return !found
	})
	return found
}

func addImport(file *ast.File, alias, path string) {
	for _, spec := range file.Imports {
		if value, _ := strconv.Unquote(spec.Path.Value); value == path {
			return
		}
	}
	spec := &ast.ImportSpec{Path: &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(path)}}
	if alias != "" && alias != filepath.Base(path) {
		spec.Name = ast.NewIdent(alias)
	}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if ok && gen.Tok == token.IMPORT {
			gen.Specs = append(gen.Specs, spec)
			file.Imports = append(file.Imports, spec)
			return
		}
	}
	decl := &ast.GenDecl{Tok: token.IMPORT, Specs: []ast.Spec{spec}}
	file.Decls = append([]ast.Decl{decl}, file.Decls...)
	file.Imports = append(file.Imports, spec)
}

func pruneUnusedStandardImports(file *ast.File) {
	for _, path := range []string{"fmt", "errors"} {
		alias := ""
		for _, spec := range file.Imports {
			value, _ := strconv.Unquote(spec.Path.Value)
			if value == path {
				alias = filepath.Base(path)
				if spec.Name != nil {
					alias = spec.Name.Name
				}
			}
		}
		if alias == "" || identifierUsed(file, alias) {
			continue
		}
		removeImport(file, path)
	}
}

func identifierUsed(file *ast.File, name string) bool {
	used := false
	ast.Inspect(file, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		ident, identOK := func() (*ast.Ident, bool) {
			if !ok {
				return nil, false
			}
			value, yes := selector.X.(*ast.Ident)
			return value, yes
		}()
		if identOK && ident.Name == name {
			used = true
			return false
		}
		return !used
	})
	return used
}

func removeImport(file *ast.File, path string) {
	var imports []*ast.ImportSpec
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.IMPORT {
			continue
		}
		var specs []ast.Spec
		for _, raw := range gen.Specs {
			spec, ok := raw.(*ast.ImportSpec)
			if !ok {
				specs = append(specs, raw)
				continue
			}
			value, _ := strconv.Unquote(spec.Path.Value)
			if value != path {
				specs = append(specs, spec)
				imports = append(imports, spec)
			}
		}
		gen.Specs = specs
	}
	var decls []ast.Decl
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if ok && gen.Tok == token.IMPORT && len(gen.Specs) == 0 {
			continue
		}
		decls = append(decls, decl)
	}
	file.Decls = decls
	file.Imports = imports
}

func normalizeDisplayMessage(value string) string {
	return strings.Trim(strings.Join(strings.Fields(value), " "), " :,-")
}

func normalizeMessage(value string) string {
	value = strings.ToLower(value)
	value = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return ' '
	}, value)
	return strings.Join(strings.Fields(value), " ")
}

func tokenSet(value string) map[string]bool {
	result := map[string]bool{}
	for _, word := range strings.Fields(normalizeMessage(value)) {
		if !isNoiseWord(word) {
			result[word] = true
		}
	}
	return result
}

func isNoiseWord(word string) bool {
	return word == "failed" || word == "to" || word == "the" || word == "a" || word == "an"
}

func actionToken(value string) string {
	for _, word := range strings.Fields(normalizeMessage(value)) {
		if !isNoiseWord(word) && word != "invalid" && word != "unsupported" {
			return word
		}
	}
	return ""
}

func operationText(value string) string {
	words := strings.Fields(normalizeDisplayMessage(value))
	if len(words) >= 2 && strings.EqualFold(words[0], "failed") && strings.EqualFold(words[1], "to") {
		words = words[2:]
	}
	return strings.Join(words, " ")
}

func stringExpr(value string) ast.Expr {
	return &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(value)}
}

func stringLiteral(expr ast.Expr) (string, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(lit.Value)
	return value, err == nil
}

func existingDomainAlias(imports map[string]string, path string) string {
	for alias, value := range imports {
		if value == path {
			return alias
		}
	}
	return ""
}

func domainByImport(domains []*ErrorDomain, path string) *ErrorDomain {
	for _, domain := range domains {
		if domain.ImportPath == path {
			return domain
		}
	}
	return nil
}

func argumentForRole(args []ast.Expr, roles []string, role string) ast.Expr {
	for i, value := range roles {
		if value == role && i < len(args) {
			return args[i]
		}
	}
	return nil
}

func argumentSentinel(args []ast.Expr, roles []string) string {
	expr := argumentForRole(args, roles, "kind")
	selector, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	return selector.Sel.Name
}

func causeKey(expr ast.Expr) string {
	ident, ok := expr.(*ast.Ident)
	if !ok {
		return ""
	}
	value := strings.ToLower(ident.Name)
	for _, suffix := range []string{"error", "err", "cause"} {
		if trimmed, found := strings.CutSuffix(value, suffix); found {
			return trimmed
		}
	}
	return value
}

func incrementNested(values map[string]map[string]int, outer, inner string) {
	if values[outer] == nil {
		values[outer] = map[string]int{}
	}
	values[outer][inner]++
}

func mostCommon(values map[string]int) string {
	best := ""
	count := 0
	for value, n := range values {
		if n > count || (n == count && value < best) {
			best, count = value, n
		}
	}
	return best
}

func mostCommonKindKey(values map[string]map[string]int, kind string) string {
	counts := map[string]int{}
	for key, kinds := range values {
		counts[key] += kinds[kind]
	}
	return mostCommon(counts)
}

func domainOwner(domain *ErrorDomain) string { return filepath.Base(filepath.Dir(domain.Directory)) }

func renderExpr(fset *token.FileSet, expr ast.Expr) string {
	var output bytes.Buffer
	if format.Node(&output, fset, expr) != nil {
		return ""
	}
	return output.String()
}

func pathWithinRoot(root, path string) bool {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	if info, statErr := os.Stat(absRoot); statErr == nil && !info.IsDir() {
		return filepath.Clean(absRoot) == filepath.Clean(path)
	}
	return isPathWithin(absRoot, path)
}

func isPathWithin(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
