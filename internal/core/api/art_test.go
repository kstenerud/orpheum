// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// closedResponse is a response, read and closed.
type closedResponse struct {
	StatusCode int
	Header     http.Header
}

// getWithoutRedirect asks for a path without following a redirect.
func (harness testHarness) getWithoutRedirect(t *testing.T, path, token string) (closedResponse, string) {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet,
		harness.httpServer.URL+api.Prefix+path, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	client := harness.httpServer.Client()
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
	return closedResponse{StatusCode: answer.StatusCode, Header: answer.Header}, string(body)
}

func TestArt_ServesAStoredSizeAndRedirectsFromOneNotMade(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	token := harness.createAdminAndSignIn(t)
	sourceSha256Hex := strings.Repeat("ab", 32)
	if err := harness.directories.WriteArt(sourceSha256Hex, "portrait-s.webp", []byte("RIFF....WEBP")); err != nil {
		t.Fatal(err)
	}
	answer, body := harness.getWithoutRedirect(t, "/bytes/art/"+sourceSha256Hex+"/portrait-s.webp", token)
	if answer.StatusCode != http.StatusOK || body != "RIFF....WEBP" ||
		answer.Header.Get("Content-Type") != "image/webp" ||
		!strings.Contains(answer.Header.Get("Cache-Control"), "immutable") {
		t.Errorf("a stored size: %d %v %q", answer.StatusCode, answer.Header, body)
	}
	moved, _ := harness.getWithoutRedirect(t, "/bytes/art/"+sourceSha256Hex+"/portrait-m.webp", token)
	if moved.StatusCode != http.StatusTemporaryRedirect ||
		moved.Header.Get("Location") != api.Prefix+"/bytes/art/"+sourceSha256Hex+"/portrait-s.webp" ||
		moved.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("a size not made: %d %v", moved.StatusCode, moved.Header)
	}
	for _, path := range []string{
		"/bytes/art/" + sourceSha256Hex + "/portrait-s.png", "/bytes/art/" + sourceSha256Hex + "/poster",
		"/bytes/art/" + sourceSha256Hex + "/landscape-s.webp",
		"/bytes/art/" + strings.Repeat("cd", 32) + "/portrait-s.webp",
	} {
		if missing, _ := harness.getWithoutRedirect(t, path, token); missing.StatusCode != http.StatusNotFound {
			t.Errorf("%s: %d, want 404", path, missing.StatusCode)
		}
	}
	strange, _ := harness.getWithoutRedirect(t, "/bytes/art/..%2Fstate/portrait-s.webp", token)
	if strange.StatusCode/100 != 4 {
		t.Errorf("a hash that is a path: %d", strange.StatusCode)
	}
	anonymous, _ := harness.getWithoutRedirect(t, "/bytes/art/"+sourceSha256Hex+"/portrait-s.webp", "")
	if anonymous.StatusCode != http.StatusUnauthorized {
		t.Errorf("without signing in: %d, want 401", anonymous.StatusCode)
	}
}

// The art path's last part is a variant of the image, a size and a format, and the API document
// says so (D227).
func TestArt_TheDocumentNamesThePathsVariant(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	served := harness.do(t, http.MethodGet, "/openapi.json", "", nil)
	var document struct {
		Paths map[string]struct {
			Get struct {
				Parameters []struct {
					Name string `json:"name"`
					In   string `json:"in"`
				} `json:"parameters"`
			} `json:"get"`
		} `json:"paths"`
	}
	if err := json.Unmarshal([]byte(served.body), &document); err != nil {
		t.Fatalf("GET /openapi.json = %d: %v", served.status, err)
	}
	var named []string
	for _, parameter := range document.Paths["/bytes/art/{sourceSha256}/{variant}"].Get.Parameters {
		if parameter.In == "path" {
			named = append(named, parameter.Name)
		}
	}
	if !slices.Equal(named, []string{"sourceSha256", "variant"}) {
		t.Errorf("the art path's parameters are %v, want [sourceSha256 variant]", named)
	}
}

func TestDoctor_AnAdminSeesTheArtworkCacheAgainstItsCeiling(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	found := harness.do(t, http.MethodGet, "/system", admin, nil)
	if found.status != http.StatusOK ||
		!strings.Contains(found.body, `{"check":"artwork cache","verdict":"ok","detail":"0.0 GiB of a 0 GiB `) {
		t.Errorf("doctor = %d %s", found.status, found.body)
	}
	if _, err := harness.authService.CreateUser(t.Context(),
		auth.NewUser{Name: "partner", Password: "partner-password", Role: auth.RoleUser}); err != nil {
		t.Fatal(err)
	}
	wantError(t, harness.do(t, http.MethodGet, "/system", harness.signIn(t, "partner", "partner-password"), nil),
		http.StatusForbidden, api.CodeForbidden)
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
		{Mechanism: "seccomp", IsInForce: true},
	}
	found := harness.do(t, http.MethodGet, "/system", admin, nil)
	want := `{"check":"FFmpeg confinement on node local","verdict":"warn",` +
		`"detail":"FFmpeg runs without all its confinement: landlock: the kernel has Landlock, ` +
		`but it is not enabled at boot","action":"landlock: add landlock to the lsm= list"}`
	if found.status != http.StatusOK || !strings.Contains(found.body, want) {
		t.Errorf("doctor = %d %s, want the missing mechanism and its remedy (D112)", found.status, found.body)
	}
	harness.nodes.confinement[0].IsInForce = true
	if confined := harness.do(t, http.MethodGet, "/system", admin, nil); !strings.Contains(confined.body,
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
		UsedBytes: 3 << 30, BudgetBytes: 200 << 30, DiskUsedPercent: 40, DiskUsedLimitPercent: 95,
		DiskAvailableBytes: 500 << 20, DiskMinFreeBytes: 100 << 20,
	}
	within := harness.do(t, http.MethodGet, "/system", admin, nil)
	want := `{"check":"prepared copies on node local","verdict":"ok","detail":"3.0 GiB of a 200 GiB budget; ` +
		`the disk is 40% full, its limit 95%, with 500 MiB free, its floor 100 MiB"}`
	if !strings.Contains(within.body, want) {
		t.Errorf("doctor = %s, want the node's prepared copies against their budget (D217 §5)", within.body)
	}
	harness.nodes.prepared.DiskUsedPercent = 97
	if past := harness.do(t, http.MethodGet, "/system", admin, nil); !strings.Contains(past.body,
		`{"check":"prepared copies on node local","verdict":"warn"`) {
		t.Errorf("doctor with the disk past its limit = %s, want a warning", past.body)
	}
	harness.nodes.prepared.DiskUsedPercent, harness.nodes.prepared.DiskAvailableBytes = 40, 50<<20
	if below := harness.do(t, http.MethodGet, "/system", admin, nil); !strings.Contains(below.body,
		`{"check":"prepared copies on node local","verdict":"warn"`) {
		t.Errorf("doctor with the disk below its floor of free space = %s, want a warning", below.body)
	}
}

// D363 §8, §9: the doctor names each file found at more than one place, at every place, and warns of
// an item whose places' NFO files disagree.
func TestDoctor_NamesAFileAtTwoPlacesAndWarnsWhenTheirNFOFilesDisagree(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelve(t)
	copyID := librarytest.File(t, harness.database, librarytest.FileSpec{
		LibraryID: 1, NodeID: 1, Path: "/m/Copy/Metropolis (1927).mkv", Filing: "copy", CopyOf: 1, FirstSeenAt: 2,
	})
	for index, besideID := range []int64{1, copyID} {
		nfoID := librarytest.File(t, harness.database, librarytest.FileSpec{
			NodeID: 1, Path: fmt.Sprintf("/m/%d/Metropolis (1927).nfo", index), SidecarKind: "nfo",
			AttachedToFile: besideID, FirstSeenAt: 2,
		})
		storetest.ExecRaw(t, harness.databasePath, fmt.Sprintf(
			"INSERT INTO file_nfo (file_id, body) VALUES (%d, CAST('<movie><title>%d</title></movie>' AS BLOB))",
			nfoID, index))
	}
	found := harness.do(t, http.MethodGet, "/system", admin, nil)
	for _, want := range []string{
		`{"check":"files found at more than one place","verdict":"ok","detail":"1, each one file, byte for byte ` +
			`the same, at every place: file 1 at /m/Metropolis (1927).mkv and /m/Copy/Metropolis (1927).mkv"}`,
		`{"check":"NFO files beside the places of one file","verdict":"warn","detail":"1 with NFO files ` +
			`that disagree, the one beside the place found first speaking for each: \"Metropolis\" (item 10) at ` +
			`/m/Metropolis (1927).mkv and /m/Copy/Metropolis (1927).mkv"`,
	} {
		if !strings.Contains(found.body, want) {
			t.Errorf("doctor = %d %s; want %s", found.status, found.body, want)
		}
	}
}
