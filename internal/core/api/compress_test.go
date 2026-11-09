// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"bytes"
	"compress/gzip"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/api"
)

// A list an operation of compressedOperations answers with is sent gzipped to a client that accepts
// it, varying by Accept-Encoding, and is the same JSON once unzipped; to one that does not, it is
// sent as it is (D342).
func TestCompress_AListIsGzippedToAClientThatAcceptsIt(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	token := harness.createAdminAndSignIn(t)
	plain := harness.doWithHeader(t, http.MethodGet, "/config", token,
		http.Header{"Accept-Encoding": {"identity"}}, nil)
	if plain.status != http.StatusOK || plain.header.Get("Content-Encoding") != "" ||
		!slices.Contains(plain.header.Values("Vary"), "Accept-Encoding") {
		t.Fatalf("/config to a client taking no gzip: %d, %v; want it as it is, varying by Accept-Encoding",
			plain.status, plain.header)
	}
	if len(plain.body) < api.SmallestCompressedAnswer {
		t.Fatalf("/config is %d bytes, too few for this test to see it gzipped", len(plain.body))
	}
	zipped := harness.doWithHeader(t, http.MethodGet, "/config", token,
		http.Header{"Accept-Encoding": {"gzip"}}, nil)
	if zipped.header.Get("Content-Encoding") != "gzip" ||
		!slices.Contains(zipped.header.Values("Vary"), "Accept-Encoding") {
		t.Fatalf("/config to a client taking gzip: %v; want it gzipped, varying by Accept-Encoding", zipped.header)
	}
	if len(zipped.body) >= len(plain.body) {
		t.Errorf("gzipped /config is %d bytes, not fewer than its %d", len(zipped.body), len(plain.body))
	}
	if unzipped := gunzipped(t, zipped.body); unzipped != plain.body {
		t.Errorf("gzipped /config unzips to\n%s\nwant\n%s", unzipped, plain.body)
	}
}

// An answer that carries a credential is never compressed, whatever the client accepts (D342):
// a sign-in's token leaves as it is.
func TestCompress_ASignInIsNeverGzipped(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	harness.createAdminAndSignIn(t)
	answer := harness.doWithHeader(t, http.MethodPost, "/session", "",
		http.Header{"Accept-Encoding": {"gzip"}},
		map[string]any{"name": "owner", "password": "owner-password", "device": testDevice})
	if answer.status != http.StatusCreated || answer.header.Get("Content-Encoding") != "" {
		t.Errorf("a sign-in to a client taking gzip: %d, %v; want it as it is", answer.status, answer.header)
	}
}

// credentialField names a field of an answer that is, or holds, a credential: a token, a secret, a
// password, a session's, a link's or a pairing's. An answer with one beside text a request chose
// is what BREACH reads by its compressed size.
var credentialField = regexp.MustCompile(`(?i)token|secret|password|credential|cookie|nonce|signature|` +
	`locator|session|link|key$|keys$|code$|codes$|url$`)

// notCredentials are fields of compressed answers whose names credentialField matches but which hold
// no credential, by operation, each with what it holds.
var notCredentials = map[string]map[string]string{
	"getSystem": { //nolint:gosec // G101: fields' names, and no credential
		"code":        "a scan's error code for what it could not read: permissionDenied, say",
		"causeCode":   "why a node's confinement is off: disabled_at_boot, say",
		"credentials": "whether a provider's credentials are accepted: unknown, accepted, missing or refused",
		"key":         "a setting's name",
	},
	"getConfig": {"key": "a setting's name", "configFileKeys": "the config file's keys and values"},
	"listUsers": {"hasPassword": "whether a person has a password, never the password"},
}

// No operation compressedOperations names answers with a field that is or holds a credential, found
// by walking each success answer's schema through every reference, list and map (D342). The walk
// is held to finding the credentials of the answers that carry one, so it cannot pass by seeing
// nothing.
func TestArch_NoCompressedOperationAnswersWithACredential(t *testing.T) {
	t.Parallel()
	document := api.NewServer(slog.New(slog.DiscardHandler)).OpenAPI()
	operations := map[string]*huma.Operation{}
	for _, item := range document.Paths {
		for _, operation := range []*huma.Operation{item.Get, item.Post, item.Put, item.Delete, item.Head, item.Patch} {
			if operation != nil {
				operations[operation.OperationID] = operation
			}
		}
	}
	for _, id := range api.CompressedOperations {
		operation, isRegistered := operations[id]
		if !isRegistered {
			t.Errorf("compressedOperations names %s, which the API does not register", id)
			continue
		}
		for _, field := range credentialFieldsOf(document, operation) {
			if _, isNot := notCredentials[id][field]; !isNot {
				t.Errorf("%s is compressed, but answers with %s, a credential's: BREACH (D342)", id, field)
			}
		}
	}
	for _, id := range []string{"signIn", "redeemSession", "takePairedSession", "getOwnLink"} {
		if len(credentialFieldsOf(document, operations[id])) == 0 {
			t.Errorf("the walk finds no credential in %s's answer, which carries one", id)
		}
	}
}

// credentialFieldsOf is the fields of operation's success answers named as a credential's.
func credentialFieldsOf(document *huma.OpenAPI, operation *huma.Operation) []string {
	if operation == nil {
		return nil
	}
	found := map[string]bool{}
	seen := map[*huma.Schema]bool{}
	for status, answer := range operation.Responses {
		if !strings.HasPrefix(status, "2") {
			continue
		}
		for _, media := range answer.Content {
			walkFields(document.Components.Schemas, media.Schema, seen, func(name string) {
				if credentialField.MatchString(name) {
					found[name] = true
				}
			})
		}
		for name := range answer.Headers {
			if credentialField.MatchString(name) || strings.EqualFold(name, "Set-Cookie") {
				found[name] = true
			}
		}
	}
	names := make([]string, 0, len(found))
	for name := range found {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// walkFields calls each with the name of every field schema holds, through references, lists, maps
// and alternatives.
func walkFields(registry huma.Registry, schema *huma.Schema, seen map[*huma.Schema]bool, each func(string)) {
	if schema == nil || seen[schema] {
		return
	}
	seen[schema] = true
	if schema.Ref != "" {
		walkFields(registry, registry.SchemaFromRef(schema.Ref), seen, each)
	}
	for name, field := range schema.Properties {
		each(name)
		walkFields(registry, field, seen, each)
	}
	walkFields(registry, schema.Items, seen, each)
	if values, isSchema := schema.AdditionalProperties.(*huma.Schema); isSchema {
		walkFields(registry, values, seen, each)
	}
	for _, alternative := range slices.Concat(schema.OneOf, schema.AnyOf, schema.AllOf) {
		walkFields(registry, alternative, seen, each)
	}
}

// gunzipped is body unzipped.
func gunzipped(t *testing.T, body string) string {
	t.Helper()
	reader, err := gzip.NewReader(strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	var unzipped bytes.Buffer
	if _, err = io.Copy(&unzipped, reader); err != nil { //nolint:gosec // G110: a test's own small answer
		t.Fatal(err)
	}
	return unzipped.String()
}
