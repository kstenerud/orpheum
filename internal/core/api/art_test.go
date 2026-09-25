// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
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
	answer, body := harness.get(t, "/art/"+hash+"/portrait-s.webp", token)
	if answer.StatusCode != http.StatusOK || body != "RIFF....WEBP" ||
		answer.Header.Get("Content-Type") != "image/webp" ||
		!strings.Contains(answer.Header.Get("Cache-Control"), "immutable") {
		t.Errorf("a stored size: %d %v %q", answer.StatusCode, answer.Header, body)
	}
	moved, _ := harness.get(t, "/art/"+hash+"/portrait-m.webp", token)
	if moved.StatusCode != http.StatusTemporaryRedirect ||
		moved.Header.Get("Location") != Prefix+"/art/"+hash+"/portrait-s.webp" ||
		moved.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("a size not made: %d %v", moved.StatusCode, moved.Header)
	}
	for _, path := range []string{
		"/art/" + hash + "/portrait-s.png", "/art/" + hash + "/poster", "/art/" + hash + "/landscape-s.webp",
		"/art/" + strings.Repeat("cd", 32) + "/portrait-s.webp",
	} {
		if missing, _ := harness.get(t, path, token); missing.StatusCode != http.StatusNotFound {
			t.Errorf("%s: %d, want 404", path, missing.StatusCode)
		}
	}
	if strange, _ := harness.get(t, "/art/..%2Fstate/portrait-s.webp", token); strange.StatusCode/100 != 4 {
		t.Errorf("a hash that is a path: %d", strange.StatusCode)
	}
	if anonymous, _ := harness.get(t, "/art/"+hash+"/portrait-s.webp", ""); anonymous.StatusCode !=
		http.StatusUnauthorized {
		t.Errorf("without signing in: %d, want 401", anonymous.StatusCode)
	}
}
