// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"slices"
	"strings"
	"testing"
)

// documentedSchema is as much of a schema in the API document as these tests read.
type documentedSchema struct {
	Ref        string                      `json:"$ref"`
	Type       json.RawMessage             `json:"type"`
	Enum       []string                    `json:"enum"`
	Properties map[string]documentedSchema `json:"properties"`
	OneOf      []documentedSchema          `json:"oneOf"`
}

// declaredCodes are the Code constants errors.go declares, read from its source, so a code added
// there and left out of codes is caught.
func declaredCodes(t *testing.T) []Code {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), "errors.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var declared []Code
	for _, declaration := range parsed.Decls {
		general, isGeneral := declaration.(*ast.GenDecl)
		if !isGeneral || general.Tok != token.CONST {
			continue
		}
		for _, spec := range general.Specs {
			value, isValue := spec.(*ast.ValueSpec)
			if !isValue || value.Type == nil || types(value.Type) != "Code" {
				continue
			}
			for _, literal := range value.Values {
				if basic, isBasic := literal.(*ast.BasicLit); isBasic {
					declared = append(declared, Code(strings.Trim(basic.Value, `"`)))
				}
			}
		}
	}
	return declared
}

// types names an identifier's type expression, or "" for any other.
func types(expression ast.Expr) string {
	if ident, isIdent := expression.(*ast.Ident); isIdent {
		return ident.Name
	}
	return ""
}

func TestProblemBody_TheDocumentNamesEveryCodeOnceAndTypesItsDetail(t *testing.T) {
	t.Parallel()
	if declared := declaredCodes(t); !slices.Equal(declared, codes()) {
		t.Fatalf("errors.go declares the codes %v, and codes lists %v; one list, in step", declared, codes())
	}
	harness := newHarness(t)
	served := harness.do(t, http.MethodGet, "/openapi.json", "", nil)
	var document struct {
		Components struct {
			Schemas map[string]documentedSchema `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal([]byte(served.body), &document); err != nil {
		t.Fatalf("GET /openapi.json = %d: %v", served.status, err)
	}
	schemas := document.Components.Schemas
	if ref := schemas["Error"].Properties["error"].Ref; ref != schemaRefPrefix+problemBodySchemaName {
		t.Errorf("an error's inside is %q, want the ProblemBody schema", ref)
	}
	detailByCode := map[string]documentedSchema{}
	var named []Code
	for _, form := range schemas[problemBodySchemaName].OneOf {
		for _, code := range form.Properties["code"].Enum {
			named = append(named, Code(code))
			if detail, hasDetail := form.Properties["detail"]; hasDetail {
				detailByCode[code] = schemas[strings.TrimPrefix(detail.Ref, schemaRefPrefix)]
			}
		}
	}
	slices.Sort(named)
	every := codes()
	slices.Sort(every)
	if !slices.Equal(named, every) {
		t.Errorf("ProblemBody's forms name the codes %v, want each of %v once", named, every)
	}
	for _, want := range []struct {
		code        Code
		field, kind string
	}{
		{CodeAuthRequired, "reason", "string"},
		{CodeInvalidRequest, "problems", "array"},
		{CodeRateLimited, "retryAfterMs", "integer"},
		{CodeQuotaExhausted, "downloadsResetAt", "integer"},
		{CodeProviderError, "reason", "string"},
	} {
		code := want.code
		// A slice is documented as ["array","null"], as every slice in the document is.
		got := string(detailByCode[string(code)].Properties[want.field].Type)
		if !strings.Contains(got, `"`+want.kind+`"`) {
			t.Errorf("%s's detail.%s is documented as %q, want %q", code, want.field, got, want.kind)
		}
	}
	var reasons []ProviderReason
	for _, reason := range detailByCode[string(CodeProviderError)].Properties["reason"].Enum {
		reasons = append(reasons, ProviderReason(reason))
	}
	if !slices.Equal(reasons, providerReasons()) {
		t.Errorf("provider_error's reasons are documented as %v, want every ProviderReason, %v, so a "+
			"client can have words for each", reasons, providerReasons())
	}
	if len(detailByCode) != len(problemDetails()) {
		t.Errorf("%d codes document a detail, want the %d with one", len(detailByCode), len(problemDetails()))
	}
}

func TestInvalidRequest_NamesEachProblemByWhereItIsInTheDetail(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	refused := harness.do(t, http.MethodPost, "/libraries", admin, map[string]any{"name": 7})
	wantError(t, refused, http.StatusUnprocessableEntity, CodeInvalidRequest)
	var body struct {
		Error struct {
			Detail InvalidRequestDetailBody `json:"detail"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(refused.body), &body); err != nil {
		t.Fatal(err)
	}
	problems := body.Error.Detail.Problems
	if len(problems) == 0 || !slices.ContainsFunc(problems, func(problem RequestProblemBody) bool {
		return problem.Location == "body.name" && problem.Message != ""
	}) {
		t.Errorf("the refusal's detail is %s, want its problems, one at body.name", refused.body)
	}
}

// TestErrorAnswers_AreLoggedWithTheirOperationAndCode holds that every error answer, a handler's
// and huma's own, is in the debug log with its operation and code: a client shows only words
// chosen by the code, so the log is where a person finds what failed. The submitted value is not.
func TestErrorAnswers_AreLoggedWithTheirOperationAndCode(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	harness.createAdminAndSignIn(t)
	wantError(t, harness.do(t, http.MethodPost, "/session", "",
		map[string]any{"name": "owner", "password": "CANARY-wrong", "device": testDevice}),
		http.StatusUnauthorized, CodeAuthRequired)
	unnamed := harness.do(t, http.MethodPost, "/session", "",
		map[string]any{"name": "owner", "password": "CANARY-unnamed"})
	wantError(t, unnamed, unnamed.status, CodeInvalidRequest)

	type errorAnswer struct {
		Message   string `json:"msg"`
		Operation string `json:"operation"`
		Method    string `json:"method"`
		Path      string `json:"path"`
		Code      Code   `json:"code"`
	}
	var answers []errorAnswer
	for line := range strings.Lines(harness.logs.String()) {
		var record errorAnswer
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("a log line is not JSON: %s", line)
		}
		if record.Message == "answered with an error" {
			answers = append(answers, record)
		}
	}
	want := []errorAnswer{
		{"answered with an error", "signIn", http.MethodPost, "/api/v1/session", CodeAuthRequired},
		{"answered with an error", "signIn", http.MethodPost, "/api/v1/session", CodeInvalidRequest},
	}
	if !slices.Equal(answers, want) {
		t.Errorf("the error answers logged = %+v, want %+v", answers, want)
	}
	if strings.Contains(harness.logs.String(), "CANARY") {
		t.Errorf("the log shows a submitted password:\n%s", harness.logs.String())
	}
}
