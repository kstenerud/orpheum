// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/artwork"
	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/datadir"
	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/secrets"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

func TestMain(m *testing.M) {
	UseErrorShape()
	m.Run()
}

const canary = "CANARY-api-4e1f-0123456789WXYZ"

// memorySetupCodes keeps the setup code in memory.
type memorySetupCodes struct {
	code string
}

func (codes *memorySetupCodes) Read() (string, error) {
	if codes.code == "" {
		return "", auth.ErrNoSetupCode
	}
	return codes.code, nil
}

func (codes *memorySetupCodes) Write(code string) error {
	codes.code = code
	return nil
}

func (codes *memorySetupCodes) Remove() error {
	codes.code = ""
	return nil
}

type testHarness struct {
	server   *Server
	provider *fakeProvider
	cache    *store.Cache
	nodes    *fakeNodes
	http     *httptest.Server
	logs     *bytes.Buffer
	service  *auth.Service
	codes    *memorySetupCodes
	database string
	art      *datadir.Directories
}

// newHarness serves an API whose dependencies are ready, over a real migrated database.
func newHarness(t *testing.T) testHarness {
	t.Helper()
	database := storetest.Open(t)
	now := clocktest.NewFake(time.Date(2026, time.September, 24, 12, 0, 0, 0, time.UTC))
	seeded := settingstest.Seeded(t, database)
	service := auth.NewService(database, now, rand.Reader, seeded)
	key, err := secrets.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	logs := &bytes.Buffer{}
	server := NewServer(slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	codes := &memorySetupCodes{}
	provider, cache, nodes := newFakeProvider(), storetest.OpenCache(t), &fakeNodes{}
	providers := identify.Providers{"tmdb": provider}
	pass := identify.NewPass(database, now, slog.New(slog.DiscardHandler), providers, seeded)
	matcher := identify.NewMatcher(database, cache, pass, now, slog.New(slog.DiscardHandler))
	art, err := datadir.Prepare(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	server.Ready(Dependencies{
		Auth: service, Settings: seeded,
		Secrets: secrets.NewStore(database, key, now, rand.Reader, seeded), SetupCodes: codes,
		Identify: identify.NewChooser(matcher, pass), Libraries: library.NewAdmin(database, now, nodes),
		Refresh: identify.NewRefresher(database, jobs.NewQueue(cache, now, seeded, slog.New(slog.DiscardHandler)),
			seeded, now, slog.New(slog.DiscardHandler)),
		Browse: library.NewBrowser(database, seeded),
		Artwork: artwork.NewService(artwork.Parts{
			Database: database, Queue: jobs.NewQueue(cache, now, seeded, slog.New(slog.DiscardHandler)),
			Store: art, Cache: cache, Settings: seeded, Clock: now, Logger: slog.New(slog.DiscardHandler),
		}),
		ConfigFile: []ConfigFileKey{{
			Key: "server.shutdown_grace_s", Value: json.RawMessage("10"), Start: json.RawMessage("10"),
			IsFromFile: false, Describe: "seconds in-flight requests get to finish",
		}},
	})
	served := httptest.NewServer(server.Handler())
	t.Cleanup(served.Close)
	return testHarness{
		server: server, http: served, logs: logs, service: service, codes: codes, database: database.Path(),
		provider: provider, cache: cache, nodes: nodes, art: art,
	}
}

type response struct {
	status int
	body   string
}

func (harness testHarness) do(t *testing.T, method, path, token string, body any) response {
	t.Helper()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(t.Context(), method, harness.http.URL+Prefix+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	answer, err := harness.http.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closeErr := answer.Body.Close(); closeErr != nil {
			t.Error(closeErr)
		}
	}()
	text, err := io.ReadAll(answer.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response{status: answer.StatusCode, body: string(text)}
}

// wantError checks a response is an error in the one shape, with the code expected.
func wantError(t *testing.T, got response, status int, code Code) {
	t.Helper()
	if got.status != status {
		t.Errorf("status = %d, want %d; body %s", got.status, status, got.body)
	}
	var shape map[string]json.RawMessage
	if err := json.Unmarshal([]byte(got.body), &shape); err != nil || len(shape) != 1 || shape["error"] == nil {
		t.Fatalf("the error body is not {\"error\": {...}}: %s", got.body)
	}
	var detail ErrorDetail
	if err := json.Unmarshal(shape["error"], &detail); err != nil {
		t.Fatal(err)
	}
	if detail.Code != code || detail.Message == "" {
		t.Errorf("error = %+v, want code %s with a message", detail, code)
	}
}

// createAdminAndSignIn completes setup as owner and returns a session token.
func (harness testHarness) createAdminAndSignIn(t *testing.T) string {
	t.Helper()
	code, err := harness.service.EnsureSetupCode(t.Context(), harness.codes)
	if err != nil {
		t.Fatal(err)
	}
	created := harness.do(t, http.MethodPost, "/setup", "",
		map[string]string{"setupCode": code, "name": "owner", "password": "owner-password"})
	if created.status != http.StatusCreated {
		t.Fatalf("setup: %d %s", created.status, created.body)
	}
	return harness.signIn(t, "owner", "owner-password")
}

func (harness testHarness) signIn(t *testing.T, name, password string) string {
	t.Helper()
	signedIn := harness.do(t, http.MethodPost, "/session", "", map[string]string{"name": name, "password": password})
	var session struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal([]byte(signedIn.body), &session); err != nil || session.Token == "" {
		t.Fatalf("sign in as %s: %d %s", name, signedIn.status, signedIn.body)
	}
	return session.Token
}

func TestHealth_AnswersBeforeTheDatabaseIsReady(t *testing.T) {
	t.Parallel()
	server := NewServer(slog.New(slog.DiscardHandler))
	served := httptest.NewServer(server.Handler())
	t.Cleanup(served.Close)
	harness := testHarness{server: server, http: served}

	for _, tc := range []struct {
		status     Status
		httpStatus int
	}{
		{StatusStarting, http.StatusOK},
		{StatusMigrating, http.StatusOK},
		{StatusDegraded, http.StatusServiceUnavailable},
	} {
		server.SetStatus(tc.status, "step 1 of 1", "a reason")
		got := harness.do(t, http.MethodGet, "/health", "", nil)
		if got.status != tc.httpStatus || !strings.Contains(got.body, `"status":"`+string(tc.status)+`"`) {
			t.Errorf("health while %s = %d %s, want %d", tc.status, got.status, got.body, tc.httpStatus)
		}
	}
	server.SetStatus(StatusMigrating, "step 1 of 1")
	wantError(t, harness.do(t, http.MethodGet, "/me", "", nil), http.StatusServiceUnavailable, CodeMigrating)
}

func TestSetup_TheCodeCreatesTheFirstAdminAndThenSetupIsClosed(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	wantError(t, harness.do(t, http.MethodPost, "/session", "",
		map[string]string{"name": "owner", "password": "x"}), http.StatusConflict, CodeSetupRequired)
	code, err := harness.service.EnsureSetupCode(t.Context(), harness.codes)
	if err != nil {
		t.Fatal(err)
	}
	wantError(t, harness.do(t, http.MethodPost, "/setup", "",
		map[string]string{"setupCode": "WRONG-CODE", "name": "owner", "password": "x"}),
		http.StatusForbidden, CodeForbidden)
	created := harness.do(t, http.MethodPost, "/setup", "",
		map[string]string{"setupCode": code, "name": "owner", "password": "owner-password"})
	if created.status != http.StatusCreated || !strings.Contains(created.body, `"isAdmin":true`) {
		t.Fatalf("setup with the right code: %d %s", created.status, created.body)
	}
	wantError(t, harness.do(t, http.MethodPost, "/setup", "",
		map[string]string{"setupCode": code, "name": "intruder", "password": "x"}), http.StatusConflict, CodeConflict)
}

func TestSession_TheTokenWorksInTheHeaderOnlyUntilSignOut(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	token := harness.createAdminAndSignIn(t)
	me := harness.do(t, http.MethodGet, "/me", token, nil)
	if me.status != http.StatusOK || !strings.Contains(me.body, `"name":"owner"`) {
		t.Errorf("GET /me = %d %s", me.status, me.body)
	}
	wantError(t, harness.do(t, http.MethodGet, "/me", "", nil), http.StatusUnauthorized, CodeAuthRequired)
	wantError(t, harness.do(t, http.MethodGet, "/me?token="+token, "", nil), http.StatusUnauthorized, CodeAuthRequired)
	wantError(t, harness.do(t, http.MethodPost, "/session", "",
		map[string]string{"name": "owner", "password": "wrong"}), http.StatusUnauthorized, CodeAuthRequired)
	if out := harness.do(t, http.MethodDelete, "/session", token, nil); out.status != http.StatusNoContent {
		t.Errorf("DELETE /session = %d %s", out.status, out.body)
	}
	wantError(t, harness.do(t, http.MethodGet, "/me", token, nil), http.StatusUnauthorized, CodeAuthRequired)
}

func TestSettings_OnlyAnAdminChangesTheServersAndABadValueChangesNothing(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	if _, err := harness.service.CreateUser(t.Context(),
		auth.NewUser{Name: "partner", Password: "partner-password", Role: auth.RoleUser}); err != nil {
		t.Fatal(err)
	}
	user := harness.signIn(t, "partner", "partner-password")
	wantError(t, harness.do(t, http.MethodGet, "/settings", user, nil), http.StatusForbidden, CodeForbidden)

	wantError(t, harness.do(t, http.MethodPut, "/settings", admin,
		map[string]any{"values": map[string]any{"update_check": "yes"}}),
		http.StatusUnprocessableEntity, CodeInvalidRequest)
	wantError(t, harness.do(t, http.MethodPut, "/settings", admin,
		map[string]any{"values": map[string]any{"signin_failure_limit": 3}}),
		http.StatusUnprocessableEntity, CodeInvalidRequest)
	page := harness.do(t, http.MethodGet, "/settings", admin, nil)
	if strings.Contains(page.body, "signin_failure_limit") || !strings.Contains(page.body, "update_check") {
		t.Errorf("GET /settings shows a setting it should not, or misses one it should: %s", page.body)
	}
	changed := harness.do(t, http.MethodPut, "/settings", admin,
		map[string]any{"values": map[string]any{"update_check": false}})
	const wantChanged = `"key":"update_check","kind":"bool","value":false,"start":null,"isModified":true`
	if changed.status != http.StatusOK || !strings.Contains(changed.body, wantChanged) {
		t.Errorf("PUT /settings = %d %s", changed.status, changed.body)
	}
	mine := harness.do(t, http.MethodGet, "/me/settings", user, nil)
	if mine.status != http.StatusOK || !strings.Contains(mine.body, `"settings":[]`) {
		t.Errorf("GET /me/settings = %d %s, want an empty list, not null", mine.status, mine.body)
	}
}

func TestSecrets_AreWriteOnly(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	set := harness.do(t, http.MethodPut, "/secrets/tmdb_api_key", admin, map[string]string{"value": canary})
	if set.status != http.StatusNoContent {
		t.Fatalf("PUT /secrets/tmdb_api_key = %d %s", set.status, set.body)
	}
	listed := harness.do(t, http.MethodGet, "/secrets", admin, nil)
	if listed.status != http.StatusOK || !strings.Contains(listed.body, `"isSet":true,"hint":"WXYZ"`) {
		t.Errorf("GET /secrets = %d %s, want it set with its hint", listed.status, listed.body)
	}
	mine := harness.do(t, http.MethodPut, "/me/secrets/opensubtitles_password", admin,
		map[string]string{"value": canary})
	if mine.status != http.StatusNoContent {
		t.Errorf("PUT /me/secrets/opensubtitles_password = %d %s", mine.status, mine.body)
	}
	wantError(t, harness.do(t, http.MethodPut, "/secrets/plex_token", admin, map[string]string{"value": canary}),
		http.StatusNotFound, CodeNotFound)
	malformed := harness.do(t, http.MethodPut, "/secrets/tmdb_api_key", admin,
		map[string]any{"value": []string{canary}})
	wantError(t, malformed, http.StatusUnprocessableEntity, CodeInvalidRequest)
	if strings.Contains(malformed.body, "CANARY") || !strings.Contains(malformed.body, "body.value") {
		t.Errorf("a malformed secret's error must name where, not echo the value: %s", malformed.body)
	}
	cleared := harness.do(t, http.MethodDelete, "/secrets/tmdb_api_key", admin, nil)
	if cleared.status != http.StatusNoContent {
		t.Errorf("DELETE /secrets/tmdb_api_key = %d %s", cleared.status, cleared.body)
	}
	for _, path := range []string{"/secrets", "/me/secrets", "/settings", "/me"} {
		if got := harness.do(t, http.MethodGet, path, admin, nil); strings.Contains(got.body, "CANARY") {
			t.Errorf("GET %s shows the secret: %s", path, got.body)
		}
	}
	if strings.Contains(harness.logs.String(), "CANARY") {
		t.Errorf("the log shows the secret:\n%s", harness.logs.String())
	}
}

func TestConfig_ShowsEverySettingAndTheFileAndResets(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	if _, err := harness.service.CreateUser(t.Context(),
		auth.NewUser{Name: "partner", Password: "partner-password", Role: auth.RoleUser}); err != nil {
		t.Fatal(err)
	}
	user := harness.signIn(t, "partner", "partner-password")
	for _, request := range [][2]string{
		{http.MethodGet, "/config"},
		{http.MethodGet, "/config/unused"},
		{http.MethodPost, "/config/signin_failure_limit/reset"},
		{http.MethodDelete, "/config/unused/x"},
	} {
		wantError(t, harness.do(t, request[0], request[1], user, nil), http.StatusForbidden, CodeForbidden)
	}

	listed := harness.do(t, http.MethodGet, "/config", admin, nil)
	const limit = `"key":"signin_failure_limit","kind":"integer","value":5,"start":5,"isModified":false,` +
		`"minimum":1,"maximum":1000`
	const file = `"file":[{"key":"server.shutdown_grace_s","value":10,"start":10,"isFromFile":false`
	if listed.status != http.StatusOK || !strings.Contains(listed.body, limit) || !strings.Contains(listed.body, file) {
		t.Errorf("GET /config = %d %s", listed.status, listed.body)
	}
	changed := harness.do(t, http.MethodPut, "/config", admin,
		map[string]any{"values": map[string]any{"signin_failure_limit": 7}})
	if !strings.Contains(changed.body, `"key":"signin_failure_limit","kind":"integer","value":7,"start":5,`+
		`"isModified":true`) {
		t.Errorf("PUT /config = %d %s", changed.status, changed.body)
	}
	wantError(t, harness.do(t, http.MethodPut, "/config", admin,
		map[string]any{"values": map[string]any{"signin_failure_limit": 0}}),
		http.StatusUnprocessableEntity, CodeInvalidRequest)
	reset := harness.do(t, http.MethodPost, "/config/signin_failure_limit/reset", admin, nil)
	if reset.status != http.StatusOK || !strings.Contains(reset.body, `"value":5,"start":5,"isModified":false`) {
		t.Errorf("POST /config/signin_failure_limit/reset = %d %s", reset.status, reset.body)
	}
	wantError(t, harness.do(t, http.MethodPost, "/config/no_such_setting/reset", admin, nil),
		http.StatusNotFound, CodeNotFound)

	storetest.ExecRaw(t, harness.database, "DELETE FROM setting WHERE key = 'secret_hint_length'")
	healed := harness.do(t, http.MethodGet, "/config", admin, nil)
	const hint = `"key":"secret_hint_length","kind":"integer","value":4`
	if healed.status != http.StatusOK || !strings.Contains(healed.body, hint) {
		t.Errorf("GET /config with a removed row = %d %s, want it healed with its starting value", healed.status,
			healed.body)
	}
}

func TestConfig_ListsAndDeletesOnlyUnusedRows(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	storetest.ExecRaw(t, harness.database,
		"INSERT INTO setting (scope, key, value, set_at) VALUES ('server', 'signin_falure_limit', '3', 1)")
	unused := harness.do(t, http.MethodGet, "/config/unused", admin, nil)
	if unused.status != http.StatusOK ||
		!strings.Contains(unused.body, `"unused":[{"key":"signin_falure_limit","scope":"server","value":3}]`) {
		t.Errorf("GET /config/unused = %d %s", unused.status, unused.body)
	}
	wantError(t, harness.do(t, http.MethodDelete, "/config/unused/signin_failure_limit", admin, nil),
		http.StatusConflict, CodeConflict)
	if deleted := harness.do(t, http.MethodDelete, "/config/unused/signin_falure_limit", admin, nil); deleted.status !=
		http.StatusNoContent {
		t.Errorf("DELETE /config/unused/signin_falure_limit = %d %s", deleted.status, deleted.body)
	}
	after := harness.do(t, http.MethodGet, "/config/unused", admin, nil)
	if !strings.Contains(after.body, `"unused":[]`) {
		t.Errorf("after the delete, GET /config/unused = %s", after.body)
	}
}

func TestConfig_FiltersToWhatDiffersFromTheStartingValue(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.do(t, http.MethodPut, "/config", admin, map[string]any{"values": map[string]any{"signin_failure_limit": 7}})
	all := harness.do(t, http.MethodGet, "/config", admin, nil)
	changed := harness.do(t, http.MethodGet, "/config?modified=true", admin, nil)
	var listed struct {
		Settings []SettingBody `json:"settings"`
	}
	if err := json.Unmarshal([]byte(changed.body), &listed); err != nil || changed.status != http.StatusOK {
		t.Fatalf("GET /config?modified=true = %d %s", changed.status, changed.body)
	}
	found := false
	for _, setting := range listed.Settings {
		if !setting.IsModified {
			t.Errorf("the filtered list holds %s, which is at its starting value", setting.Key)
		}
		found = found || setting.Key == "signin_failure_limit"
	}
	if !found || strings.Contains(changed.body, `"key":"update_check"`) ||
		!strings.Contains(all.body, `"key":"update_check"`) {
		t.Errorf("the filter kept an unchanged setting or dropped the changed one:\n%s", changed.body)
	}
}
