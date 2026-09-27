// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// fetched is a response, read and closed.
type fetched struct {
	StatusCode int
	Header     http.Header
}

// get asks for a path without following a redirect.
func (harness testHarness) get(t *testing.T, path, token string) (fetched, string) {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, harness.http.URL+Prefix+path, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	client := harness.http.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	answer, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closeErr := answer.Body.Close(); closeErr != nil && !errors.Is(closeErr, io.EOF) {
			t.Error(closeErr)
		}
	}()
	body, err := io.ReadAll(answer.Body)
	if err != nil {
		t.Fatal(err)
	}
	return fetched{StatusCode: answer.StatusCode, Header: answer.Header}, string(body)
}

func TestArt_ServesAStoredSizeAndRedirectsFromOneNotMade(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	token := harness.createAdminAndSignIn(t)
	hash := strings.Repeat("ab", 32)
	if err := harness.art.WriteArt(hash, "portrait-s.webp", []byte("RIFF....WEBP")); err != nil {
		t.Fatal(err)
	}
	answer, body := harness.get(t, "/bytes/art/"+hash+"/portrait-s.webp", token)
	if answer.StatusCode != http.StatusOK || body != "RIFF....WEBP" ||
		answer.Header.Get("Content-Type") != "image/webp" ||
		!strings.Contains(answer.Header.Get("Cache-Control"), "immutable") {
		t.Errorf("a stored size: %d %v %q", answer.StatusCode, answer.Header, body)
	}
	moved, _ := harness.get(t, "/bytes/art/"+hash+"/portrait-m.webp", token)
	if moved.StatusCode != http.StatusTemporaryRedirect ||
		moved.Header.Get("Location") != Prefix+"/bytes/art/"+hash+"/portrait-s.webp" ||
		moved.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("a size not made: %d %v", moved.StatusCode, moved.Header)
	}
	for _, path := range []string{
		"/bytes/art/" + hash + "/portrait-s.png", "/bytes/art/" + hash + "/poster",
		"/bytes/art/" + hash + "/landscape-s.webp",
		"/bytes/art/" + strings.Repeat("cd", 32) + "/portrait-s.webp",
	} {
		if missing, _ := harness.get(t, path, token); missing.StatusCode != http.StatusNotFound {
			t.Errorf("%s: %d, want 404", path, missing.StatusCode)
		}
	}
	if strange, _ := harness.get(t, "/bytes/art/..%2Fstate/portrait-s.webp", token); strange.StatusCode/100 != 4 {
		t.Errorf("a hash that is a path: %d", strange.StatusCode)
	}
	if anonymous, _ := harness.get(t, "/bytes/art/"+hash+"/portrait-s.webp", ""); anonymous.StatusCode !=
		http.StatusUnauthorized {
		t.Errorf("without signing in: %d, want 401", anonymous.StatusCode)
	}
}

func TestDoctor_AnAdminSeesTheArtworkCacheAgainstItsCeiling(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	found := harness.do(t, http.MethodGet, "/doctor", admin, nil)
	if found.status != http.StatusOK ||
		!strings.Contains(found.body, `{"check":"artwork cache","verdict":"ok","detail":"0.0 GiB of a 0 GiB `) {
		t.Errorf("doctor = %d %s", found.status, found.body)
	}
	if _, err := harness.service.CreateUser(t.Context(),
		auth.NewUser{Name: "partner", Password: "partner-password", Role: auth.RoleUser}); err != nil {
		t.Fatal(err)
	}
	wantError(t, harness.do(t, http.MethodGet, "/doctor", harness.signIn(t, "partner", "partner-password"), nil),
		http.StatusForbidden, CodeForbidden)
}

func TestDoctor_SaysANodeRunsFFmpegWithoutAMechanismAndWhatWouldPutItThere(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.connectNode(t, "/media/films")
	harness.nodes.confinement = []nodeproto.Confinement{
		{
			Mechanism: "landlock", Reason: "the kernel has Landlock, but it is not enabled at boot",
			Remedy: "add landlock to the lsm= list",
		},
		{Mechanism: "seccomp", InForce: true},
	}
	found := harness.do(t, http.MethodGet, "/doctor", admin, nil)
	want := `{"check":"FFmpeg confinement on node local","verdict":"warn",` +
		`"detail":"FFmpeg runs on this node without all its confinement: landlock: the kernel has Landlock, ` +
		`but it is not enabled at boot","action":"landlock: add landlock to the lsm= list"}`
	if found.status != http.StatusOK || !strings.Contains(found.body, want) {
		t.Errorf("doctor = %d %s, want the missing mechanism and its remedy (D112)", found.status, found.body)
	}
	harness.nodes.confinement[0].InForce = true
	if confined := harness.do(t, http.MethodGet, "/doctor", admin, nil); !strings.Contains(confined.body,
		`{"check":"FFmpeg confinement on node local","verdict":"ok"`) {
		t.Errorf("doctor with both in force = %s, want ok", confined.body)
	}
}

func TestDoctor_ShowsEachNodesPreparedCopiesAgainstTheirBudgetAndWarnsPastTheDisksLimit(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.connectNode(t, "/media/films")
	harness.nodes.prepared = &nodeproto.PreparedUsage{
		Bytes: 3 << 30, BudgetBytes: 200 << 30, DiskPercent: 40, DiskLimitPercent: 95,
	}
	within := harness.do(t, http.MethodGet, "/doctor", admin, nil)
	want := `{"check":"prepared copies on node local","verdict":"ok","detail":"3.0 GiB of a 200 GiB budget; ` +
		`the disk is 40% full, its limit 95%"}`
	if !strings.Contains(within.body, want) {
		t.Errorf("doctor = %s, want the node's prepared copies against their budget (D217 §5)", within.body)
	}
	harness.nodes.prepared.DiskPercent = 97
	if past := harness.do(t, http.MethodGet, "/doctor", admin, nil); !strings.Contains(past.body,
		`{"check":"prepared copies on node local","verdict":"warn"`) {
		t.Errorf("doctor with the disk past its limit = %s, want a warning", past.body)
	}
}
