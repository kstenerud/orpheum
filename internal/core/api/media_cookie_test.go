// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"bytes"
	"maps"
	"net/http"
	"strings"
	"testing"
)

// raw sends a request with the given headers and cookie and returns the response, closed.
func (harness testHarness) raw(t *testing.T, method, path, body string, header http.Header) (fetched, string) {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), method, harness.http.URL+Prefix+path,
		bytes.NewReader([]byte(body)))
	if err != nil {
		t.Fatal(err)
	}
	maps.Copy(request.Header, header)
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
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
	var read bytes.Buffer
	if _, err := read.ReadFrom(answer.Body); err != nil {
		t.Fatal(err)
	}
	return fetched{StatusCode: answer.StatusCode, Header: answer.Header}, read.String()
}

func TestMediaCookie_SignInSetsItOnlyTheBytePathsTakeItAndSignOutClearsIt(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	harness.createAdminAndSignIn(t)
	hash := strings.Repeat("ab", 32)
	if err := harness.art.WriteArt(hash, "portrait-s.webp", []byte("RIFF....WEBP")); err != nil {
		t.Fatal(err)
	}
	signedIn, _ := harness.raw(t, http.MethodPost, "/session", `{"name":"owner","password":"owner-password"}`, nil)
	cookies := (&http.Response{Header: signedIn.Header}).Cookies()
	if len(cookies) != 1 || cookies[0].Name != MediaCookie || !cookies[0].HttpOnly ||
		cookies[0].SameSite != http.SameSiteStrictMode || cookies[0].Path != Prefix+"/bytes/" || cookies[0].Secure {
		t.Fatalf("sign-in set %+v; want one HttpOnly, SameSite=Strict media cookie scoped to /bytes/, not Secure "+
			"over plain HTTP", cookies)
	}
	withCookie := http.Header{"Cookie": {MediaCookie + "=" + cookies[0].Value}}
	image, body := harness.raw(t, http.MethodGet, "/bytes/art/"+hash+"/portrait-s.webp", "", withCookie)
	if image.StatusCode != http.StatusOK || body != "RIFF....WEBP" {
		t.Errorf("an image by the cookie alone: %d %q", image.StatusCode, body)
	}
	// The API takes the header and never the cookie: a forged request changes nothing (D154).
	if me, _ := harness.raw(t, http.MethodGet, "/me", "", withCookie); me.StatusCode != http.StatusUnauthorized {
		t.Errorf("/me by the cookie alone: %d, want 401", me.StatusCode)
	}
	signOut := http.Header{"Authorization": {"Bearer " + cookies[0].Value}}
	signedOut, _ := harness.raw(t, http.MethodDelete, "/session", "", signOut)
	cleared := (&http.Response{Header: signedOut.Header}).Cookies()
	if signedOut.StatusCode != http.StatusNoContent || len(cleared) != 1 || cleared[0].MaxAge >= 0 {
		t.Errorf("sign-out: %d, cookies %+v; want the media cookie cleared", signedOut.StatusCode, cleared)
	}
	after, _ := harness.raw(t, http.MethodGet, "/bytes/art/"+hash+"/portrait-s.webp", "", withCookie)
	if after.StatusCode != http.StatusUnauthorized {
		t.Errorf("the cookie after sign-out: %d, want 401", image.StatusCode)
	}
}
