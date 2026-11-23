// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"net/http"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// A file's key frame is its node's, asked on its byte socket for the file's picture stream at the
// place, relayed as the WebP it is to a person reaching the file, by the media cookie an <img> sends,
// cached privately and kept nowhere; a person reaching no library is answered as for a file that does
// not exist, and a place outside the film is refused (D545).
func TestFrame_IsRelayedForAPersonReachingTheFileAndNoOneElse(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	asked := nodeproto.FrameRequest{
		Video:       nodeproto.BytesFile{RootPath: "/media", Path: []byte("Film (2002).mkv"), PayloadHash: "0c0d"},
		StreamIndex: 0, AtMs: 60_000,
	}
	harness.nodeBytes.mutex.Lock()
	harness.nodeBytes.files[asked.Target()] = "a webp"
	harness.nodeBytes.mutex.Unlock()
	path := api.Prefix + "/bytes/files/2/frame?atMs="
	got := harness.getBytePath(t, path+"60000", admin, true, "")
	if got.status != http.StatusOK || got.body != "a webp" {
		t.Fatalf("GET by the media cookie = %d %q, want the node's key frame", got.status, got.body)
	}
	if contentType, cache := got.header.Get("Content-Type"), got.header.Get("Cache-Control"); contentType !=
		"image/webp" || cache != "private, max-age=3600" {
		t.Errorf("Content-Type %q, Cache-Control %q; want image/webp, private, max-age=3600", contentType, cache)
	}
	for place, want := range map[string]int{
		"-1": http.StatusUnprocessableEntity, "1200001": http.StatusUnprocessableEntity,
		"soon": http.StatusUnprocessableEntity, "": http.StatusUnprocessableEntity,
	} {
		if refused := harness.getBytePath(t, path+place, admin, false, ""); refused.status != want {
			t.Errorf("atMs=%q = %d, want %d", place, refused.status, want)
		}
	}
	made := harness.do(t, http.MethodPost, "/roles", admin, map[string]any{
		"name": "no films", "capabilities": []string{}, "libraries": []any{},
	})
	if made.status != http.StatusCreated {
		t.Fatalf("POST /roles = %d %s", made.status, made.body)
	}
	if _, err := harness.authService.CreateUser(t.Context(),
		auth.NewUser{Name: "stranger", Password: "stranger-password", Role: "no films"}); err != nil {
		t.Fatal(err)
	}
	stranger := harness.signIn(t, "stranger", "stranger-password")
	if refused := harness.getBytePath(t, path+"60000", stranger, true, ""); refused.status != http.StatusNotFound {
		t.Errorf("a person reaching no library = %d, want 404, as a file that does not exist", refused.status)
	}
	if refused := harness.getBytePath(t, path+"60000", "", true, ""); refused.status != http.StatusUnauthorized {
		t.Errorf("no one signed in = %d, want 401", refused.status)
	}
}
