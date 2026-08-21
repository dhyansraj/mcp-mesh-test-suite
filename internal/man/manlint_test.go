package man

// Regression guard against man-page drift.
//
// Every ```yaml fence in every embedded man page is parsed and validated
// against the real schema the binary implements:
//
//   - the fence is decoded into the matching struct from internal/config with
//     yaml KnownFields(true), so a key the struct does not declare is an error
//   - every step names a handler registered in handlers.NewRegistry(), and
//     carries only options that handler actually reads
//   - every ${...} reference uses a form internal/interpolate actually resolves
//
// The three "sources of truth" (handler names, the step options each handler
// reads, interpolation prefixes) are derived from the packages that implement
// them rather than copied into a literal list here, so the guard follows the
// binary as it changes instead of becoming a second thing that can drift.
//
// A fence that is deliberately not a tsuite document (a GitHub Actions
// snippet, say) must opt out explicitly with an HTML comment on the line
// before it:
//
//	<!-- manlint:skip GitHub Actions workflow, not a tsuite file -->
//
// or by using a ```yaml-fragment info string. Skips are counted and logged so
// coverage cannot quietly disappear.

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/config"
	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/handlers"
	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/interpolate"
)

// ---------------------------------------------------------------------------
// fence extraction
// ---------------------------------------------------------------------------

type yamlFence struct {
	index      int    // 1-based position among yaml fences in the page
	line       int    // 1-based line of the opening ``` in the page
	heading    string // nearest preceding markdown heading
	skipReason string // non-empty => opted out of validation
	body       string
}

var skipMarker = regexp.MustCompile(`^<!--\s*manlint:skip\s*(.*?)\s*-->\s*$`)

// extractYAMLFences pulls every yaml fence out of a markdown page, remembering
// where it came from so failures can point at it.
func extractYAMLFences(page string) []yamlFence {
	var (
		fences  []yamlFence
		heading string
		pending string
		inFence bool
		info    string
		start   int
		buf     []string
	)

	for i, line := range strings.Split(page, "\n") {
		lineNo := i + 1

		if strings.HasPrefix(line, "```") {
			if inFence {
				if isYAMLInfo(info) {
					fences = append(fences, yamlFence{
						index:      len(fences) + 1,
						line:       start,
						heading:    heading,
						skipReason: fenceSkipReason(info, pending),
						body:       strings.Join(buf, "\n"),
					})
				}
				inFence, info, buf, pending = false, "", nil, ""
				continue
			}
			inFence = true
			info = strings.TrimSpace(strings.TrimPrefix(line, "```"))
			start = lineNo
			continue
		}

		if inFence {
			buf = append(buf, line)
			continue
		}

		switch {
		case strings.HasPrefix(line, "#"):
			heading = strings.TrimSpace(line)
			pending = ""
		case skipMarker.MatchString(line):
			pending = strings.TrimSpace(skipMarker.FindStringSubmatch(line)[1])
			if pending == "" {
				pending = "(no reason given)"
			}
		case strings.TrimSpace(line) != "":
			pending = ""
		}
	}

	return fences
}

func firstWord(s string) string {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func isYAMLInfo(info string) bool {
	switch strings.ToLower(firstWord(info)) {
	case "yaml", "yml", "yaml-fragment":
		return true
	}
	return false
}

func fenceSkipReason(info, pending string) string {
	if strings.EqualFold(firstWord(info), "yaml-fragment") {
		if pending != "" {
			return pending
		}
		return "yaml-fragment info string"
	}
	return pending
}

// ---------------------------------------------------------------------------
// classification
// ---------------------------------------------------------------------------

type fenceKind string

const (
	kindSuiteConfig fenceKind = "config.yaml"
	kindTestConfig  fenceKind = "test.yaml"
	kindRoutines    fenceKind = "routines.yaml"
	kindSteps       fenceKind = "step list"
	kindAssertions  fenceKind = "assertion list"
)

// classify decides what a fence should be validated against from its own
// shape: a sequence is either assertions or steps, and a mapping is matched
// against the top-level key sets of the config structs. The nearest heading
// and any leading `# config.yaml` comment are used only to break ties, so
// adding a section to a man page cannot silently change what gets checked.
func classify(root *yaml.Node, hint string) (fenceKind, error) {
	switch root.Kind {
	case yaml.SequenceNode:
		if len(root.Content) > 0 && allItemsHaveKey(root.Content, "expr") {
			return kindAssertions, nil
		}
		return kindSteps, nil

	case yaml.MappingNode:
		keys := mappingKeys(root)
		if contains(keys, "routines") {
			return kindRoutines, nil
		}

		suiteHits := intersect(keys, structYAMLKeys(config.SuiteConfig{}))
		testHits := intersect(keys, structYAMLKeys(config.TestConfig{}))

		switch {
		case len(suiteHits) > 0 && len(testHits) > 0:
			if k, ok := kindFromHint(hint); ok {
				return k, nil
			}
			return "", fmt.Errorf("ambiguous document: keys %v look like config.yaml and %v look like test.yaml", suiteHits, testHits)
		case len(suiteHits) > 0:
			return kindSuiteConfig, nil
		case len(testHits) > 0:
			return kindTestConfig, nil
		}

		if k, ok := kindFromHint(hint); ok {
			return k, nil
		}
		return "", fmt.Errorf("no top-level key matches config.yaml, test.yaml or routines.yaml (keys: %v)", keys)
	}

	return "", fmt.Errorf("expected a mapping or a sequence at the document root, got %s", nodeKind(root))
}

func kindFromHint(hint string) (fenceKind, bool) {
	switch {
	case strings.Contains(hint, "routines.yaml"):
		return kindRoutines, true
	case strings.Contains(hint, "config.yaml"):
		return kindSuiteConfig, true
	case strings.Contains(hint, "test.yaml"):
		return kindTestConfig, true
	}
	return "", false
}

// ---------------------------------------------------------------------------
// the test
// ---------------------------------------------------------------------------

func TestManPageYAMLMatchesConfigSchema(t *testing.T) {
	var (
		registry  = handlers.NewRegistry()
		stepKeys  = newStepKeyPolicy(t, registry)
		variables = knownVariableForms(t)
		topics    = embeddedTopics(t)
	)

	totalValidated, totalSkipped := 0, 0

	for _, topic := range topics {
		t.Run(topic, func(t *testing.T) {
			page, err := (&ManPage{Name: topic}).GetContent()
			if err != nil {
				t.Fatalf("reading embedded content for %q: %v", topic, err)
			}

			fences := extractYAMLFences(page)
			validated, skipped := 0, 0

			for _, f := range fences {
				if f.skipReason != "" {
					skipped++
					t.Logf("%s.md:%d yaml fence #%d skipped: %s", topic, f.line, f.index, f.skipReason)
					continue
				}
				validated++
				lint := &fenceLinter{
					t:         t,
					topic:     topic,
					fence:     f,
					registry:  registry,
					stepKeys:  stepKeys,
					variables: variables,
				}
				lint.run()
			}

			totalValidated += validated
			totalSkipped += skipped
			t.Logf("%s.md: %d yaml fences validated, %d skipped", topic, validated, skipped)
			if len(fences) == 0 {
				t.Logf("%s.md: no yaml examples on this page", topic)
			}
		})
	}

	t.Logf("total: %d yaml fences validated, %d skipped across %d topics", totalValidated, totalSkipped, len(topics))
	if totalValidated == 0 {
		t.Fatal("no yaml fences were validated; the extractor or the embedded content is broken")
	}
}

type fenceLinter struct {
	t         *testing.T
	topic     string
	fence     yamlFence
	registry  *handlers.Registry
	stepKeys  stepKeyPolicy
	variables variableForms
}

// errorf reports a problem with enough context to find and fix it: topic,
// position in the page, nearest heading and the snippet itself.
func (l *fenceLinter) errorf(format string, args ...any) {
	l.t.Helper()
	l.t.Errorf("%s.md:%d (yaml fence #%d, under %q): %s\n--- snippet ---\n%s---------------",
		l.topic, l.fence.line, l.fence.index, l.fence.heading,
		fmt.Sprintf(format, args...), indent(l.fence.body))
}

func (l *fenceLinter) run() {
	l.t.Helper()

	l.checkVariables()

	var root yaml.Node
	if err := yaml.Unmarshal([]byte(l.fence.body), &root); err != nil {
		l.errorf("not valid YAML: %v", err)
		return
	}
	if len(root.Content) == 0 {
		l.errorf("fence is empty")
		return
	}
	doc := root.Content[0]

	hint := l.fence.heading + "\n" + leadingComments(l.fence.body)
	kind, err := classify(doc, hint)
	if err != nil {
		l.errorf("cannot tell what this example is: %v\n"+
			"add `<!-- manlint:skip <reason> -->` on the line before the fence if it is not a tsuite document", err)
		return
	}

	switch kind {
	case kindSuiteConfig:
		l.checkSuiteConfig()
	case kindTestConfig:
		l.checkTestConfig()
	case kindRoutines:
		l.checkRoutines()
	case kindSteps:
		var steps []config.Step
		if !l.decodeStrict(&steps, string(kind)) {
			return
		}
		l.checkSteps("", steps)
	case kindAssertions:
		var assertions []config.Assertion
		if !l.decodeStrict(&assertions, string(kind)) {
			return
		}
		l.checkAssertions("", assertions)
	}
}

// decodeStrict decodes the fence with KnownFields(true) so any key the target
// struct does not declare is a failure.
func (l *fenceLinter) decodeStrict(out any, as string) bool {
	l.t.Helper()
	dec := yaml.NewDecoder(bytes.NewReader([]byte(l.fence.body)))
	dec.KnownFields(true)
	if err := dec.Decode(out); err != nil {
		l.errorf("does not decode as %s: %v", as, err)
		return false
	}
	return true
}

func (l *fenceLinter) checkSuiteConfig() {
	l.t.Helper()

	var suite config.SuiteConfig
	l.decodeStrict(&suite, string(kindSuiteConfig))
}

func (l *fenceLinter) checkTestConfig() {
	l.t.Helper()

	var test config.TestConfig
	if !l.decodeStrict(&test, string(kindTestConfig)) {
		return
	}
	l.checkSteps("pre_run", test.PreRun)
	l.checkSteps("test", test.Test)
	l.checkSteps("post_run", test.PostRun)
	l.checkAssertions("assertions", test.Assertions)
}

func (l *fenceLinter) checkRoutines() {
	l.t.Helper()

	var routines config.GlobalRoutinesConfig
	if !l.decodeStrict(&routines, string(kindRoutines)) {
		return
	}
	for _, name := range sortedKeys(routines.Routines) {
		def := routines.Routines[name]
		if len(def.Steps) == 0 {
			l.errorf("routine %q has no steps: (a routine is a `steps:` list)", name)
			continue
		}
		l.checkSteps("routines."+name+".steps", def.Steps)
	}
}

func (l *fenceLinter) checkSteps(where string, steps []config.Step) {
	l.t.Helper()

	for i, step := range steps {
		at := fmt.Sprintf("%s[%d]", where, i)
		if where == "" {
			at = fmt.Sprintf("step[%d]", i)
		}
		if step.Name != "" {
			at += " " + fmt.Sprintf("%q", step.Name)
		}

		switch {
		case step.Routine != "":
			if step.Handler != "" {
				l.errorf("%s sets both routine: and handler:", at)
			}
		case step.Handler == "":
			l.errorf("%s has neither handler: nor routine:", at)
		default:
			if _, ok := l.registry.Get(step.Handler); !ok {
				l.errorf("%s uses handler %q, which is not registered (registered: %s)",
					at, step.Handler, strings.Join(l.stepKeys.handlers(), ", "))
			}
		}

		// config.Step has an inline catch-all (Extra), so KnownFields cannot
		// reject unknown step keys - they land in Extra instead. Check them
		// against the keys this step's own handler actually reads.
		for _, key := range sortedKeys(step.Extra) {
			if l.stepKeys.allows(step.Handler, key) {
				continue
			}
			switch {
			case step.Routine != "":
				l.errorf("%s sets %q, which is not a step field; a routine step passes values through params:", at, key)
			case l.stepKeys.knows(step.Handler):
				l.errorf("%s sets %q, which is not a step field and is not read by the %q handler (its options: %s)",
					at, key, step.Handler, strings.Join(sortedKeys(l.stepKeys.byHandler[step.Handler]), ", "))
			default:
				l.errorf("%s sets %q, which is not a step field and is not read by any handler", at, key)
			}
		}
	}
}

func (l *fenceLinter) checkAssertions(where string, assertions []config.Assertion) {
	l.t.Helper()

	for i, a := range assertions {
		if strings.TrimSpace(a.Expr) == "" {
			at := fmt.Sprintf("%s[%d]", where, i)
			if where == "" {
				at = fmt.Sprintf("assertion[%d]", i)
			}
			l.errorf("%s has an empty expr:", at)
		}
	}
}

var varRef = regexp.MustCompile(`\$\{([^}]+)\}`)

func (l *fenceLinter) checkVariables() {
	l.t.Helper()

	seen := map[string]bool{}
	for _, m := range varRef.FindAllStringSubmatch(l.fence.body, -1) {
		ref := m[1]
		if seen[ref] {
			continue
		}
		seen[ref] = true
		if form, ok := l.variables.recognize(ref); !ok {
			l.errorf("${%s} is not a form internal/interpolate resolves (%s)\nrecognized: %s",
				ref, form, l.variables.describe())
		}
	}
}

// ---------------------------------------------------------------------------
// sources of truth, derived rather than duplicated
// ---------------------------------------------------------------------------

// variableForms is the set of ${...} shapes ResolveVariable understands,
// extracted from its source so a new prefix is picked up automatically.
type variableForms struct {
	prefixes map[string]bool // "config.", "captured.", ...
	schemes  map[string]bool // "env:", "jq:", ...
	bare     map[string]bool // "stdout", "suite_path", "uc_name", ...
}

func (v variableForms) recognize(ref string) (string, bool) {
	if i := strings.Index(ref, ":"); i >= 0 && (!strings.Contains(ref[:i], ".") || i < strings.Index(ref, ".")) {
		if v.schemes[ref[:i+1]] {
			return "", true
		}
		return "unknown prefix " + ref[:i+1], false
	}
	if i := strings.Index(ref, "."); i >= 0 {
		if v.prefixes[ref[:i+1]] {
			return "", true
		}
		return "unknown prefix " + ref[:i+1], false
	}
	if v.bare[ref] {
		return "", true
	}
	return "unknown bare name (names are case-sensitive; an unresolvable reference is left verbatim at runtime)", false
}

func (v variableForms) describe() string {
	all := append(sortedKeys(v.prefixes), sortedKeys(v.schemes)...)
	for _, b := range sortedKeys(v.bare) {
		all = append(all, b)
	}
	return strings.Join(all, " ")
}

// knownVariableForms reads the prefixes out of interpolate.ResolveVariable and
// the extra top-level names the runner seeds into Context.Extra.
func knownVariableForms(t *testing.T) variableForms {
	t.Helper()

	forms := variableForms{
		prefixes: map[string]bool{},
		schemes:  map[string]bool{},
		bare:     map[string]bool{},
	}

	for _, lit := range varNameLiterals(t, siblingPkg("interpolate"), "ResolveVariable") {
		switch {
		case strings.HasSuffix(lit, "."):
			forms.prefixes[lit] = true
		case strings.HasSuffix(lit, ":"):
			forms.schemes[lit] = true
		default:
			forms.bare[lit] = true
		}
	}
	for _, key := range contextExtraKeys(t, siblingPkg("runner")) {
		forms.bare[key] = true
	}

	// If the extraction ever stops matching the source, fail loudly here
	// rather than silently accepting every variable in the docs.
	for _, anchor := range []string{"config.", "captured.", "last.", "params.", "state.", "steps."} {
		if !forms.prefixes[anchor] {
			t.Fatalf("could not derive prefix %q from interpolate.ResolveVariable; the extractor needs updating", anchor)
		}
	}
	for _, anchor := range []string{"env:", "file:", "fixture:", "jq:", "json:", "jsonfile:"} {
		if !forms.schemes[anchor] {
			t.Fatalf("could not derive scheme %q from interpolate.ResolveVariable; the extractor needs updating", anchor)
		}
	}
	for _, anchor := range []string{"stdout", "exit_code", "suite_path", "artifacts", "uc_name"} {
		if !forms.bare[anchor] {
			t.Fatalf("could not derive bare name %q; the extractor needs updating", anchor)
		}
	}

	return forms
}

// TestDerivedVariablePrefixesResolve cross-checks the AST extraction against
// runtime behaviour: every derived dotted prefix must actually route to a map
// on the Context, and a made-up prefix must not.
func TestDerivedVariablePrefixesResolve(t *testing.T) {
	const sentinel = "manlint-sentinel"

	forms := knownVariableForms(t)

	newCtx := func() *interpolate.Context {
		ctx := interpolate.NewContext()
		for _, m := range []map[string]any{ctx.Config, ctx.State, ctx.Captured, ctx.Last, ctx.Steps, ctx.Params} {
			m["probe"] = sentinel
		}
		return ctx
	}

	for _, prefix := range sortedKeys(forms.prefixes) {
		t.Run(prefix, func(t *testing.T) {
			got, err := interpolate.ResolveVariable(prefix+"probe", newCtx())
			if err != nil {
				t.Fatalf("ResolveVariable(%q) error = %v", prefix+"probe", err)
			}
			if got != sentinel {
				t.Errorf("ResolveVariable(%q) = %#v, want %q: derived prefix does not route to a Context map", prefix+"probe", got, sentinel)
			}
		})
	}

	t.Run("unknown", func(t *testing.T) {
		if forms.prefixes["nope."] {
			t.Fatal("test bug: nope. should not be a derived prefix")
		}
		got, _ := interpolate.ResolveVariable("nope.probe", newCtx())
		if got == sentinel {
			t.Errorf("ResolveVariable(%q) = %q, want unresolved", "nope.probe", sentinel)
		}
	})
}

// stepKeyPolicy is what a step may carry, per handler. universal holds the
// keys every step may use: the yaml tags on config.Step plus the keys shared
// code in internal/handlers reads. byHandler holds, for each registered
// handler name, the extra options only that handler reads out of its step map
// (config.Step.Extra is an inline catch-all, so those never appear as struct
// fields) - so `operation:` is a file option, not a shell one.
type stepKeyPolicy struct {
	universal map[string]bool
	byHandler map[string]map[string]bool
}

func (p stepKeyPolicy) allows(handler, key string) bool {
	return p.universal[key] || p.byHandler[handler][key]
}

func (p stepKeyPolicy) knows(handler string) bool {
	_, ok := p.byHandler[handler]
	return ok
}

func (p stepKeyPolicy) handlers() []string { return sortedKeys(p.byHandler) }

// newStepKeyPolicy derives the policy from internal/handlers: which types
// NewRegistry registers, what name each of those types returns from Name(),
// and which literal step keys each one reads. Nothing is listed here, so a new
// handler is covered the moment it is registered.
func newStepKeyPolicy(t *testing.T, r *handlers.Registry) stepKeyPolicy {
	t.Helper()

	policy := stepKeyPolicy{
		universal: map[string]bool{},
		byHandler: map[string]map[string]bool{},
	}
	for _, k := range structYAMLKeys(config.Step{}) {
		policy.universal[k] = true
	}

	files := parseGoDir(t, siblingPkg("handlers"))
	nameOfType := handlerNameByType(files)

	for _, typ := range registeredHandlerTypes(files) {
		name, ok := nameOfType[typ]
		if !ok {
			t.Fatalf("handlers.NewRegistry registers &%s{} but no Name() method on %s returns a string literal; the extractor needs updating", typ, typ)
		}
		if _, ok := r.Get(name); !ok {
			t.Errorf("handlers.NewRegistry registers &%s{} as %q but the registry has no such handler", typ, name)
			continue
		}
		policy.byHandler[name] = map[string]bool{}
	}
	if len(policy.byHandler) == 0 {
		t.Fatal("derived no handler names from internal/handlers; the extractor needs updating")
	}
	for typ, name := range nameOfType {
		if !policy.knows(name) {
			t.Errorf("handler %q (%s) declares Name() but is not registered by handlers.NewRegistry()", name, typ)
		}
	}

	read := 0
	for owner, keys := range stepMapKeysByOwner(files, nameOfType) {
		for _, k := range keys {
			read++
			if owner == "" {
				policy.universal[k] = true
				continue
			}
			policy.byHandler[owner][k] = true
		}
	}
	if read == 0 {
		t.Fatal("derived no step option keys from internal/handlers; the extractor needs updating")
	}

	// If the attribution ever stops matching the source, fail loudly here
	// rather than silently accepting (or rejecting) every option in the docs.
	for _, anchor := range []string{"handler", "command", "workdir"} {
		if !policy.universal[anchor] {
			t.Fatalf("expected %q to be allowed on every step; the extractor needs updating", anchor)
		}
	}
	for handler, key := range map[string]string{
		"file":           "operation",
		"wait":           "type",
		"pip-install":    "packages",
		"npm-install":    "replace_file_deps",
		"maven-install":  "m2_repo",
		"gradle-install": "strip_file_repos",
	} {
		if !policy.allows(handler, key) {
			t.Fatalf("expected the %q handler to read %q; the extractor needs updating", handler, key)
		}
	}
	if policy.allows("shell", "operation") {
		t.Fatal("shell must not accept the file handler's operation:; step keys are no longer attributed per handler")
	}

	return policy
}

// handlerNameByType maps a handler implementation type to the name its Name()
// method returns.
func handlerNameByType(files []*ast.File) map[string]string {
	out := map[string]string{}
	for _, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name.Name != "Name" || fn.Body == nil {
				continue
			}
			typ := receiverType(fn)
			if typ == "" {
				continue
			}
			for _, stmt := range fn.Body.List {
				ret, ok := stmt.(*ast.ReturnStmt)
				if !ok || len(ret.Results) != 1 {
					continue
				}
				if s, ok := stringLit(ret.Results[0]); ok {
					out[typ] = s
				}
			}
		}
	}
	return out
}

// registeredHandlerTypes returns the types NewRegistry passes to Register,
// e.g. r.Register(&ShellHandler{}) -> "ShellHandler".
func registeredHandlerTypes(files []*ast.File) []string {
	var out []string
	for _, file := range files {
		ast.Inspect(file, func(n ast.Node) bool {
			decl, ok := n.(*ast.FuncDecl)
			if !ok || decl.Name.Name != "NewRegistry" || decl.Body == nil {
				return true
			}
			ast.Inspect(decl.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok || len(call.Args) != 1 {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "Register" {
					return true
				}
				arg := call.Args[0]
				if unary, ok := arg.(*ast.UnaryExpr); ok && unary.Op == token.AND {
					arg = unary.X
				}
				lit, ok := arg.(*ast.CompositeLit)
				if !ok {
					return true
				}
				if id, ok := lit.Type.(*ast.Ident); ok {
					out = append(out, id.Name)
				}
				return true
			})
			return false
		})
	}
	return out
}

// varNameLiterals returns the string literals compared against `varName`
// inside fn: strings.HasPrefix(varName, "x"), varName == "x", and the cases of
// a `switch varName`.
func varNameLiterals(t *testing.T, dir, fn string) []string {
	t.Helper()

	var out []string
	for _, file := range parseGoDir(t, dir) {
		ast.Inspect(file, func(n ast.Node) bool {
			decl, ok := n.(*ast.FuncDecl)
			if !ok || decl.Name.Name != fn {
				return true
			}
			ast.Inspect(decl.Body, func(n ast.Node) bool {
				switch node := n.(type) {
				case *ast.CallExpr:
					if isSelector(node.Fun, "strings", "HasPrefix") && len(node.Args) == 2 && isIdent(node.Args[0], "varName") {
						if s, ok := stringLit(node.Args[1]); ok {
							out = append(out, s)
						}
					}
				case *ast.BinaryExpr:
					if node.Op == token.EQL {
						if isIdent(node.X, "varName") {
							if s, ok := stringLit(node.Y); ok {
								out = append(out, s)
							}
						}
					}
				case *ast.SwitchStmt:
					if !isIdent(node.Tag, "varName") {
						return true
					}
					for _, stmt := range node.Body.List {
						clause, ok := stmt.(*ast.CaseClause)
						if !ok {
							continue
						}
						for _, expr := range clause.List {
							if s, ok := stringLit(expr); ok {
								out = append(out, s)
							}
						}
					}
				}
				return true
			})
			return false
		})
	}
	return out
}

// contextExtraKeys returns the literal keys assigned into an interpolate
// Context's Extra map, e.g. ctx.Extra["uc_name"] = ucName.
func contextExtraKeys(t *testing.T, dir string) []string {
	t.Helper()

	var out []string
	for _, file := range parseGoDir(t, dir) {
		ast.Inspect(file, func(n ast.Node) bool {
			assign, ok := n.(*ast.AssignStmt)
			if !ok {
				return true
			}
			for _, lhs := range assign.Lhs {
				idx, ok := lhs.(*ast.IndexExpr)
				if !ok {
					continue
				}
				sel, ok := idx.X.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "Extra" {
					continue
				}
				if s, ok := stringLit(idx.Index); ok {
					out = append(out, s)
				}
			}
			return true
		})
	}
	return out
}

// stepMapKeysByOwner returns the literal keys handlers pull out of the step
// map - covering both step["x"] and helper calls such as stringField(step,
// "x") - attributed to the handler that reads them.
//
// Handlers share this package (and its accessor helpers), so ownership is
// decided per declaration: a method on a registered handler type belongs to
// that handler; a plain function belongs to the handler its file implements,
// since every handler lives in its own file. Anything else - handler.go's
// shared helpers, or a file implementing several handlers - is universal and
// keyed by "".
func stepMapKeysByOwner(files []*ast.File, nameOfType map[string]string) map[string][]string {
	out := map[string][]string{}
	for _, file := range files {
		var fileOwners []string
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			if name, ok := nameOfType[receiverType(fn)]; ok && !contains(fileOwners, name) {
				fileOwners = append(fileOwners, name)
			}
		}

		for _, decl := range file.Decls {
			owner := ""
			if len(fileOwners) == 1 {
				owner = fileOwners[0]
			}
			if fn, ok := decl.(*ast.FuncDecl); ok {
				if name, ok := nameOfType[receiverType(fn)]; ok {
					owner = name
				}
			}
			out[owner] = append(out[owner], stepMapKeysIn(decl)...)
		}
	}
	return out
}

// stepMapKeysIn collects the literal step-map keys read anywhere under n.
func stepMapKeysIn(n ast.Node) []string {
	var out []string
	ast.Inspect(n, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.IndexExpr:
			if isIdent(node.X, "step") {
				if s, ok := stringLit(node.Index); ok {
					out = append(out, s)
				}
			}
		case *ast.CallExpr:
			for i, arg := range node.Args {
				if !isIdent(arg, "step") || i+1 >= len(node.Args) {
					continue
				}
				if s, ok := stringLit(node.Args[i+1]); ok {
					out = append(out, s)
				}
			}
		}
		return true
	})
	return out
}

// receiverType returns the (pointer-dereferenced) receiver type name of a
// method, or "" for a plain function.
func receiverType(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return ""
	}
	expr := fn.Recv.List[0].Type
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	if id, ok := expr.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}

// siblingPkg locates a package next to internal/man from this file's own path,
// so the AST extraction does not depend on the working directory.
func siblingPkg(name string) string {
	_, thisFile, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(thisFile), "..", name)
}

func parseGoDir(t *testing.T, dir string) []*ast.File {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}

	fset := token.NewFileSet()
	var files []*ast.File
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		files = append(files, f)
	}
	if len(files) == 0 {
		t.Fatalf("no Go files parsed from %s", dir)
	}
	return files
}

// ---------------------------------------------------------------------------
// embedded content
// ---------------------------------------------------------------------------

// embeddedTopics lists the topics from the //go:embed FS the binary serves, so
// the test covers exactly what ships and a page that fails to embed fails here.
func embeddedTopics(t *testing.T) []string {
	t.Helper()

	entries, err := fs.ReadDir(contentFS, "content")
	if err != nil {
		t.Fatalf("reading embedded content: %v", err)
	}

	var topics []string
	for _, e := range entries {
		if name, ok := strings.CutSuffix(e.Name(), ".md"); ok {
			topics = append(topics, name)
		}
	}
	sort.Strings(topics)
	if len(topics) == 0 {
		t.Fatal("no man pages embedded")
	}
	return topics
}

// TestManRegistryMatchesEmbeddedContent keeps Pages, ListPages and the
// embedded files in step: an unreachable page is as bad as a missing one.
func TestManRegistryMatchesEmbeddedContent(t *testing.T) {
	topics := embeddedTopics(t)

	listed := map[string]bool{}
	for _, p := range ListPages() {
		listed[p.Name] = true
	}

	for _, topic := range topics {
		t.Run(topic, func(t *testing.T) {
			page, ok := Pages[topic]
			if !ok {
				t.Fatalf("content/%s.md is embedded but has no entry in man.Pages", topic)
			}
			if !listed[topic] {
				t.Errorf("%q is in man.Pages but missing from ListPages()", topic)
			}
			if GetPage(topic) != page {
				t.Errorf("GetPage(%q) did not return the registered page", topic)
			}
			if _, err := page.GetContent(); err != nil {
				t.Errorf("GetContent() error = %v", err)
			}
		})
	}

	for name := range Pages {
		if !contains(topics, name) {
			t.Errorf("man.Pages has %q but content/%s.md is not embedded", name, name)
		}
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// structYAMLKeys returns the yaml key names declared by a struct, ignoring
// skipped ("-") and inline fields.
func structYAMLKeys(v any) []string {
	rt := reflect.TypeOf(v)
	keys := make([]string, 0, rt.NumField())
	for i := 0; i < rt.NumField(); i++ {
		tag := rt.Field(i).Tag.Get("yaml")
		name, _, _ := strings.Cut(tag, ",")
		if name == "" || name == "-" {
			continue
		}
		keys = append(keys, name)
	}
	return keys
}

func mappingKeys(node *yaml.Node) []string {
	var keys []string
	for i := 0; i+1 < len(node.Content); i += 2 {
		keys = append(keys, node.Content[i].Value)
	}
	return keys
}

func allItemsHaveKey(items []*yaml.Node, key string) bool {
	for _, item := range items {
		if item.Kind != yaml.MappingNode || !contains(mappingKeys(item), key) {
			return false
		}
	}
	return true
}

// leadingComments returns the comment lines at the top of a fence, which is
// where the pages say `# test.yaml` or `# global/routines.yaml`.
func leadingComments(body string) string {
	var out []string
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if !strings.HasPrefix(trimmed, "#") {
			break
		}
		out = append(out, trimmed)
	}
	return strings.Join(out, "\n")
}

func indent(s string) string {
	var b strings.Builder
	for _, line := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		b.WriteString("  " + line + "\n")
	}
	return b.String()
}

func nodeKind(n *yaml.Node) string {
	switch n.Kind {
	case yaml.MappingNode:
		return "mapping"
	case yaml.SequenceNode:
		return "sequence"
	case yaml.ScalarNode:
		return "scalar"
	case yaml.AliasNode:
		return "alias"
	}
	return "unknown"
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

func intersect(keys, want []string) []string {
	var out []string
	for _, k := range keys {
		if contains(want, k) {
			out = append(out, k)
		}
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func isIdent(e ast.Expr, name string) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == name
}

func isSelector(e ast.Expr, pkg, name string) bool {
	sel, ok := e.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == name && isIdent(sel.X, pkg)
}

func stringLit(e ast.Expr) (string, bool) {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}
	return s, true
}
