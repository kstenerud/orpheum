// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// callerCheck is who a check lets call: a capability's value and the schemes any one of which
// it takes.
type callerCheck struct {
	capability string
	schemes    []string
}

// String is the check as a declaration would say it.
func (check callerCheck) String() string {
	if len(check.schemes) == 0 {
		return check.capability
	}
	return check.capability + " by " + strings.Join(check.schemes, " or ")
}

// callerChecks are the functions that let a caller in, each by what it checks; a handler is held to
// the one it reaches. A check is found by its name wherever a handler, or a function of this
// package it reaches, names it, and what it calls in turn is its own.
var callerChecks = map[string]callerCheck{
	"requireSignedIn": {"signedIn", []string{schemeSession}},
	// A guest receiver too, held by its handler to its cast (D465 §2).
	"requireSignedInOrGuest":        {"signedInOrGuest", []string{schemeSession}},
	"requireSignedInReachingFile":   {"signedIn", []string{schemeSession}},
	"requireSignedInReachingItem":   {"signedIn", []string{schemeSession}},
	"requireAdmin":                  {"admin", []string{schemeSession}},
	"requireAccountManager":         {"accountSecurity", []string{schemeSession}},
	"requireMetadataEditor":         {"editMetadata", []string{schemeSession}},
	"requireMetadataEditorReaching": {"editMetadata", []string{schemeSession}},
	"requireTogetherHost":           {"hostTogether", []string{schemeSession}},
	"requireDeviceTests":            {"deviceTests", []string{schemeSession}},
	"deviceTestsCaller":             {"deviceTests", []string{schemeSession}},
	"requireSignedInForMedia":       {"media", []string{schemeSession, schemeMedia}},
	// A byte path's holder: the person signed in by the header or the media cookie, or a viewer by
	// their membership (D246 §3).
	"mediaHolder": {"media", []string{schemeSession, schemeMedia, schemeMembership}},
	// A playback's reporter: the person signed in, or a viewer by their membership.
	"reporterOf": {"member", []string{schemeSession, schemeMembership}},
	"MemberOf":   {"member", []string{schemeMembership}},
	"ViewerOf":   {"member", []string{schemeMembership}},
	// Who joins Watch Together signed in joins as themselves; the link is the grant (D117).
	"signedInBy": {"none", []string{schemeSession}},
}

// checksOfOtherPackages are the callerChecks another package declares; every other one is a
// function of this package, so a check removed or renamed cannot leave its capability unenforced.
var checksOfOtherPackages = map[string]bool{"MemberOf": true, "ViewerOf": true}

// secretsChecked are the operations anyone may call, each with the secret in the request that its
// handler checks itself, or why it has none to check. A new operation declaring none is listed
// here, saying which.
var secretsChecked = map[string]string{
	"getHealth":         "none: whether the server is up and ready",
	"getSetup":          "none: whether setup is complete",
	"getClientManifest": "none: the served client's signed manifest, public by design (D195)",
	"setup":             "the setup code from the server's log (Auth.CompleteSetup)",
	"signIn":            "the name and password (Auth.SignIn)",
	"register":          "the invitation's or registration link's secret (Auth.Register)",
	"getLinkKind":       "the invitation's or registration link's secret (Auth.KindOfLink)",
	"handOffSession":    "the name and password (Auth.HandOff)",
	"redeemSession":     "the hand-off's code (Auth.TakeHandOff)",
	"requestPairing": "none: it makes a pairing under the device's own secret, which grants nothing until a " +
		"person enters its code",
	"getPairingState":          "the pairing's secret (Auth.PairingStateOf)",
	"takePairedSession":        "the pairing's secret (Auth.TakePairedSession)",
	"tradeGuestGrant":          "the guest receiver's grant its sender handed it (Auth.TradeGuestGrant)",
	"getAirPlayMasterPlaylist": "the AirPlay grant in the path, from its address (Auth.AirPlayGrantOf)",
	"getAirPlayFile":           "the AirPlay grant in the path, from its address (Auth.AirPlayGrantOf)",
	"echoReachability":         "the nonce, answered only if this server made it (Reachability.Recognises)",
	"joinTogether":             "the Watch Together link's secret (Together.Join)",
	"checkTogetherLink":        "the Watch Together link's secret (Together.LinkAdmits)",
}

// sourceText is one Go source file: its name and its text.
type sourceText struct {
	name, text string
}

// accessIndex is what accessViolations reads of a package's source: its functions by name, its
// string constants by name, and its huma.Register calls.
type accessIndex struct {
	functions     map[string]*ast.FuncDecl
	constants     map[string]string
	registrations []*ast.CallExpr
	fileSet       *token.FileSet
}

// indexAccess parses sources, or says why it could not.
func indexAccess(sources []sourceText) (accessIndex, []string) {
	index := accessIndex{
		functions: map[string]*ast.FuncDecl{}, constants: map[string]string{}, fileSet: token.NewFileSet(),
	}
	var problems []string
	for _, source := range sources {
		parsed, err := parser.ParseFile(index.fileSet, source.name, source.text, parser.SkipObjectResolution)
		if err != nil {
			problems = append(problems, source.name+" does not parse: "+err.Error())
			continue
		}
		for _, declaration := range parsed.Decls {
			index.add(declaration)
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			if call, isCall := node.(*ast.CallExpr); isCall && isHumaRegister(call) {
				index.registrations = append(index.registrations, call)
			}
			return true
		})
	}
	return index, problems
}

// add indexes one top-level declaration: a function or a block of string constants.
func (index accessIndex) add(declaration ast.Decl) {
	switch declared := declaration.(type) {
	case *ast.FuncDecl:
		index.functions[declared.Name.Name] = declared
	case *ast.GenDecl:
		if declared.Tok != token.CONST {
			return
		}
		for _, spec := range declared.Specs {
			values, isValues := spec.(*ast.ValueSpec)
			if !isValues {
				continue
			}
			for position, name := range values.Names {
				if position >= len(values.Values) {
					continue
				}
				if text, isString := stringLiteral(values.Values[position]); isString {
					index.constants[name.Name] = text
				}
			}
		}
	}
}

// isHumaRegister reports whether call is huma.Register(api, operation, handler).
func isHumaRegister(call *ast.CallExpr) bool {
	selector, isSelector := call.Fun.(*ast.SelectorExpr)
	if !isSelector || selector.Sel.Name != "Register" {
		return false
	}
	packageName, isIdent := selector.X.(*ast.Ident)
	return isIdent && packageName.Name == "huma" && len(call.Args) == 3
}

// stringLiteral is expression's text when it is a string literal.
func stringLiteral(expression ast.Expr) (string, bool) {
	literal, isLiteral := expression.(*ast.BasicLit)
	if !isLiteral || literal.Kind != token.STRING {
		return "", false
	}
	text, err := strconv.Unquote(literal.Value)
	return text, err == nil
}

// accessViolations says, for each registration in sources, where its declaration of who may call it
// (callableBy) and its handler's check of the caller disagree; an operation declaring none must be
// in secretsChecked, and secretsChecked must name no other. A registration this rule cannot read is
// a violation too, so it cannot pass by not looking.
func accessViolations(sources []sourceText, secretsChecked map[string]string) []string {
	index, violations := indexAccess(sources)
	declaredNone := map[string]bool{}
	for _, call := range index.registrations {
		where := index.fileSet.Position(call.Pos()).String()
		operationID, declared, problem := index.declaration(call)
		if problem != "" {
			violations = append(violations, where+": "+problem)
			continue
		}
		reached := index.checksReachedFrom(operationID)
		if len(reached) > 1 {
			violations = append(violations, where+": "+operationID+
				"'s handler checks its caller in more than one way, "+strings.Join(sortedChecks(reached), "; ")+
				": make it one check, named in callerChecks")
			continue
		}
		found := callerCheck{capability: string(capabilityNone)}
		for _, check := range reached {
			found = check
		}
		if found.String() != declared.String() {
			violations = append(violations, where+": "+operationID+" declares "+declared.String()+
				", but its handler checks "+found.String())
		}
		if declared.capability == string(capabilityNone) {
			declaredNone[operationID] = true
			if secretsChecked[operationID] == "" {
				violations = append(violations, where+": "+operationID+" declares none: name the secret its handler "+
					"checks itself, or why it has none, in secretsChecked")
			}
		}
	}
	for operationID := range secretsChecked {
		if !declaredNone[operationID] {
			violations = append(violations, "secretsChecked names "+operationID+", which does not declare none")
		}
	}
	slices.Sort(violations)
	return violations
}

// sortedChecks are checks as declarations would say them, in order.
func sortedChecks(checks map[string]callerCheck) []string {
	return slices.Sorted(maps.Keys(checks))
}

// declaration is a registration's OperationID and what its callableBy declares, with each scheme
// its capability takes by default spelled out, or what makes it unreadable.
func (index accessIndex) declaration(call *ast.CallExpr) (string, callerCheck, string) {
	operation, isLiteral := call.Args[1].(*ast.CompositeLit)
	if !isLiteral {
		return "", callerCheck{}, "registers an operation this rule cannot read: not a huma.Operation literal"
	}
	operationID, declaration := "", ast.Expr(nil)
	for _, element := range operation.Elts {
		field, isField := element.(*ast.KeyValueExpr)
		if !isField {
			continue
		}
		key, isIdent := field.Key.(*ast.Ident)
		switch {
		case !isIdent:
		case key.Name == "OperationID":
			operationID, _ = stringLiteral(field.Value)
		case key.Name == "Extensions":
			declaration = field.Value
		}
	}
	handler, isSelector := call.Args[2].(*ast.SelectorExpr)
	if operationID == "" || !isSelector || handler.Sel.Name != operationID {
		return "", callerCheck{}, "registers an operation whose handler is not the method its OperationID names"
	}
	callable, isCall := declaration.(*ast.CallExpr)
	if function, isIdent := callableFunction(callable, isCall); !isIdent || function != "callableBy" {
		return "", callerCheck{}, operationID + " declares no capability: set Extensions to callableBy(...)"
	}
	declared, problem := index.callableByArguments(callable)
	if problem != "" {
		return "", callerCheck{}, operationID + "'s callableBy " + problem
	}
	return operationID, declared, ""
}

// callableByArguments is what a callableBy call declares, with each scheme its capability takes by
// default spelled out, or what makes it unreadable.
func (index accessIndex) callableByArguments(callable *ast.CallExpr) (callerCheck, string) {
	var names []string
	for _, argument := range callable.Args {
		ident, isIdent := argument.(*ast.Ident)
		value, isConstant := "", false
		if isIdent {
			value, isConstant = index.constants[ident.Name]
		}
		if !isConstant {
			return callerCheck{}, "names something other than a constant"
		}
		names = append(names, value)
	}
	if len(names) == 0 {
		return callerCheck{}, "names no capability"
	}
	declared := callerCheck{capability: names[0], schemes: names[1:]}
	if len(declared.schemes) == 0 {
		declared.schemes = defaultSchemes(capability(declared.capability))
	}
	return declared, ""
}

// callableFunction is the name call calls, when it is a plain function's.
func callableFunction(call *ast.CallExpr, isCall bool) (string, bool) {
	if !isCall {
		return "", false
	}
	ident, isIdent := call.Fun.(*ast.Ident)
	if !isIdent {
		return "", false
	}
	return ident.Name, true
}

// checksReachedFrom is every check in callerChecks that the function named, or any function of
// the package it names in turn, names, by how a declaration says it.
func (index accessIndex) checksReachedFrom(function string) map[string]callerCheck {
	reached := map[string]bool{}
	seen := map[string]bool{}
	var walk func(name string)
	walk = func(name string) {
		declaration := index.functions[name]
		if seen[name] || declaration == nil || declaration.Body == nil {
			return
		}
		seen[name] = true
		ast.Inspect(declaration.Body, func(node ast.Node) bool {
			var named string
			switch reference := node.(type) {
			case *ast.SelectorExpr:
				named = reference.Sel.Name
			case *ast.Ident:
				named = reference.Name
			default:
				return true
			}
			if _, isCheck := callerChecks[named]; isCheck {
				reached[named] = true
			} else {
				walk(named)
			}
			return true
		})
	}
	walk(function)
	checks := map[string]callerCheck{}
	for name := range reached {
		checks[callerChecks[name].String()] = callerChecks[name] // two forms of one check are one
	}
	return checks
}

// Each operation declares who may call it, its capability and its security (D425 §6), and its
// handler checks the caller as declared: the one check it reaches in callerChecks is the
// declaration's, an operation reaching none declares none and names the secret it checks itself in
// secretsChecked. A handler that reaches two checks fails, so a check cannot hide behind another.
func TestArch_EveryHandlerChecksTheCallersItsOperationDeclares(t *testing.T) {
	t.Parallel()
	names, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var sources []sourceText
	for _, name := range names {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		text, readErr := os.ReadFile(name)
		if readErr != nil {
			t.Fatal(readErr)
		}
		sources = append(sources, sourceText{name: name, text: string(text)})
	}
	index, _ := indexAccess(sources)
	if len(index.registrations) < 100 {
		t.Fatalf("found %d registrations; the rule would pass by not looking", len(index.registrations))
	}
	for _, violation := range accessViolations(sources, secretsChecked) {
		t.Error(violation)
	}
	for _, name := range slices.Sorted(maps.Keys(callerChecks)) {
		if index.functions[name] == nil && !checksOfOtherPackages[name] {
			t.Errorf("callerChecks names %s, which this package does not declare", name)
		}
	}
}

// accessFixture is a package of one operation, getThing, declared as declared, whose handler checks
// its caller by body.
func accessFixture(declared, body string) []sourceText {
	return []sourceText{{name: "fixture.go", text: `package api

const (
	capabilitySignedIn capability = "signedIn"
	capabilityAdmin    capability = "admin"
	capabilityNone     capability = "none"
	schemeSession = "session"
)

func (server *Server) registerThing() {
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getThing", Method: http.MethodGet, Path: "/thing",
		Extensions: ` + declared + `,
	}, server.getThing)
}

func (server *Server) getThing(ctx context.Context, input *AuthorizationHeader) (*struct{}, error) {
	` + body + `
	return nil, nil
}

func (server *Server) checkThroughAHelper(ctx context.Context, authorization string) error {
	_, _, err := server.requireAdmin(ctx, authorization)
	return err
}
`}}
}

// The rule fails a handler whose check and declaration disagree, either way and through a helper;
// one declaring none that no list says the secret of; and two checks in one handler. It passes one
// that agrees.
func TestAccessViolations_FailAHandlerWhoseCheckDisagreesWithItsDeclaration(t *testing.T) {
	t.Parallel()
	signedIn := "_, _, err := server.requireSignedIn(ctx, input.Authorization); _ = err"
	admin := "_, _, err := server.requireAdmin(ctx, input.Authorization); _ = err"
	viaHelper := "_ = server.checkThroughAHelper(ctx, input.Authorization)"
	cases := []struct {
		name, declared, body string
		secrets              map[string]string
		wantViolation        string
	}{
		{
			"an admin's operation checking only a sign-in", "callableBy(capabilityAdmin)", signedIn, nil,
			"getThing declares admin by session, but its handler checks signedIn by session",
		},
		{
			"a signed-in operation checking an admin", "callableBy(capabilitySignedIn)", admin, nil,
			"getThing declares signedIn by session, but its handler checks admin by session",
		},
		{
			"a signed-in operation checking an admin through a helper", "callableBy(capabilitySignedIn)", viaHelper,
			nil, "but its handler checks admin by session",
		},
		{
			"an operation for anyone checking a sign-in", "callableBy(capabilityNone)", signedIn,
			map[string]string{"getThing": "a secret"},
			"getThing declares none, but its handler checks signedIn",
		},
		{
			"an operation for anyone naming no secret", "callableBy(capabilityNone)", "", nil,
			"name the secret its handler checks itself",
		},
		{
			"a list naming a secret for an operation that checks a sign-in", "callableBy(capabilitySignedIn)", signedIn,
			map[string]string{"getThing": "a secret"},
			"secretsChecked names getThing, which does not declare none",
		},
		{"two checks", "callableBy(capabilityAdmin)", signedIn + "\n\t" + admin, nil, "in more than one way"},
		{"no declaration", "nil", signedIn, nil, "declares no capability"},
		{
			"other schemes than the check's", "callableBy(capabilitySignedIn, schemeSession, schemeSession)", signedIn,
			nil, "declares signedIn by session or session, but its handler checks signedIn by session",
		},
	}
	for _, test := range cases {
		violations := accessViolations(accessFixture(test.declared, test.body), test.secrets)
		if !slices.ContainsFunc(violations, func(violation string) bool {
			return strings.Contains(violation, test.wantViolation)
		}) {
			t.Errorf("%s: violations %q, want one saying %q", test.name, violations, test.wantViolation)
		}
	}
	for _, agreeing := range []struct{ declared, body string }{
		{"callableBy(capabilityAdmin)", admin},
		{"callableBy(capabilityAdmin)", viaHelper},
		{"callableBy(capabilitySignedIn)", signedIn},
	} {
		if violations := accessViolations(accessFixture(agreeing.declared, agreeing.body), nil); len(violations) > 0 {
			t.Errorf("%s with %s: violations %q, want none", agreeing.declared, agreeing.body, violations)
		}
	}
	if violations := accessViolations(accessFixture("callableBy(capabilityNone)", ""),
		map[string]string{"getThing": "a secret"}); len(violations) > 0 {
		t.Errorf("none naming its secret: violations %q, want none", violations)
	}
}
