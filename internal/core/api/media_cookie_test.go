// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"bytes"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// doWithHeaders sends a request with the given headers and cookie and returns the response, closed.
func (harness testHarness) doWithHeaders(
	t *testing.T, method, path, body string, header http.Header,
) (closedResponse, string) {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), method, harness.httpServer.URL+Prefix+path,
		bytes.NewReader([]byte(body)))
	if err != nil {
		t.Fatal(err)
	}
	maps.Copy(request.Header, header)
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	answer, err := harness.httpServer.Client().Do(request)
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
	return closedResponse{StatusCode: answer.StatusCode, Header: answer.Header}, read.String()
}

func TestMediaCookie_SignInSetsItOnlyTheBytePathsTakeItAndSignOutClearsIt(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	harness.createAdminAndSignIn(t)
	hash := strings.Repeat("ab", 32)
	if err := harness.directories.WriteArt(hash, "portrait-s.webp", []byte("RIFF....WEBP")); err != nil {
		t.Fatal(err)
	}
	const credentials = `{"name":"owner","password":"owner-password",` +
		`"device":{"name":"Firefox on Linux","platform":"web"}}`
	signedIn, _ := harness.doWithHeaders(t, http.MethodPost, "/session", credentials, nil)
	cookies := (&http.Response{Header: signedIn.Header}).Cookies()
	if len(cookies) != 1 || cookies[0].Name != MediaCookieName || !cookies[0].HttpOnly ||
		cookies[0].SameSite != http.SameSiteStrictMode || cookies[0].Path != Prefix+"/bytes/" || cookies[0].Secure {
		t.Fatalf("sign-in set %+v; want one HttpOnly, SameSite=Strict media cookie scoped to /bytes/, not Secure "+
			"over plain HTTP", cookies)
	}
	withCookie := http.Header{"Cookie": {MediaCookieName + "=" + cookies[0].Value}}
	image, body := harness.doWithHeaders(t, http.MethodGet, "/bytes/art/"+hash+"/portrait-s.webp", "", withCookie)
	if image.StatusCode != http.StatusOK || body != "RIFF....WEBP" {
		t.Errorf("an image by the cookie alone: %d %q", image.StatusCode, body)
	}
	// The API takes the header and never the cookie: a forged request changes nothing (D154).
	me, _ := harness.doWithHeaders(t, http.MethodGet, "/me", "", withCookie)
	if me.StatusCode != http.StatusUnauthorized {
		t.Errorf("/me by the cookie alone: %d, want 401", me.StatusCode)
	}
	signOut := http.Header{"Authorization": {"Bearer " + cookies[0].Value}}
	signedOut, _ := harness.doWithHeaders(t, http.MethodDelete, "/session", "", signOut)
	cleared := (&http.Response{Header: signedOut.Header}).Cookies()
	if signedOut.StatusCode != http.StatusNoContent || len(cleared) != 1 || cleared[0].MaxAge >= 0 {
		t.Errorf("sign-out: %d, cookies %+v; want the media cookie cleared", signedOut.StatusCode, cleared)
	}
	after, _ := harness.doWithHeaders(t, http.MethodGet, "/bytes/art/"+hash+"/portrait-s.webp", "", withCookie)
	if after.StatusCode != http.StatusUnauthorized {
		t.Errorf("the cookie after sign-out: %d, want 401", image.StatusCode)
	}
}

func TestMediaCookie_AskingWhoIAmSetsItAgainForThisSession(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	token := harness.createAdminAndSignIn(t)
	// A browser whose cookie is missing or stale, from an older path or another sign-in, asks
	// who it is at start and is given this session's cookie before it fetches any bytes.
	me, _ := harness.doWithHeaders(t, http.MethodGet, "/me", "", http.Header{"Authorization": {"Bearer " + token}})
	cookies := (&http.Response{Header: me.Header}).Cookies()
	if me.StatusCode != http.StatusOK || len(cookies) != 1 || cookies[0].Name != MediaCookieName ||
		cookies[0].Value != token || cookies[0].Path != Prefix+"/bytes/" || !cookies[0].HttpOnly {
		t.Errorf("GET /me = %d with cookies %+v; want this session's media cookie on /bytes/", me.StatusCode, cookies)
	}
	refused, _ := harness.doWithHeaders(t, http.MethodGet, "/me", "", nil)
	if len(refused.Header.Values("Set-Cookie")) != 0 {
		t.Errorf("GET /me with no session set a cookie: %v", refused.Header.Values("Set-Cookie"))
	}
}

// sendOverTLS sends a request to server, which serves over TLS, and returns the response, closed.
func sendOverTLS(
	t *testing.T, server *httptest.Server, method, path, body string, header http.Header,
) (closedResponse, string) {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), method, server.URL+Prefix+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	maps.Copy(request.Header, header)
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	answer, err := server.Client().Do(request)
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
	return closedResponse{StatusCode: answer.StatusCode, Header: answer.Header}, read.String()
}

// cookieNamed is the cookie of that name a response sets, or nil.
func cookieNamed(answer closedResponse, name string) *http.Cookie {
	for _, cookie := range (&http.Response{Header: answer.Header}).Cookies() {
		if cookie.Name == name {
			return cookie
		}
	}
	return nil
}

// Over TLS the media cookie is host-only, so no other core on the same site can set one in its
// place; the plain name a release before set is cleared, and no longer signs anything in (the D308
// audit's F-6).
func TestMediaCookie_OverTLSItIsHostOnlyAndThePlainNameNoLongerCounts(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	harness.createAdminAndSignIn(t)
	hash := strings.Repeat("cd", 32)
	if err := harness.directories.WriteArt(hash, "portrait-s.webp", []byte("RIFF....WEBP")); err != nil {
		t.Fatal(err)
	}
	overTLS := httptest.NewTLSServer(harness.server.Handler())
	t.Cleanup(overTLS.Close)
	signedIn, _ := sendOverTLS(t, overTLS, http.MethodPost, "/session", `{"name":"owner","password":"owner-password",`+
		`"device":{"name":"Firefox on Linux","platform":"web"}}`, nil)
	hostOnly := cookieNamed(signedIn, "__Host-"+MediaCookieName)
	if hostOnly == nil || !hostOnly.Secure || hostOnly.Path != "/" || hostOnly.Domain != "" || !hostOnly.HttpOnly {
		t.Fatalf("over TLS sign-in set %+v; want a host-only media cookie: Secure, at /, for no domain", hostOnly)
	}
	if plain := cookieNamed(signedIn, MediaCookieName); plain == nil || plain.MaxAge >= 0 {
		t.Errorf("over TLS sign-in left the plain media cookie as %+v; want it cleared", plain)
	}
	image := "/bytes/art/" + hash + "/portrait-s.webp"
	byHostOnly := http.Header{"Cookie": {hostOnly.Name + "=" + hostOnly.Value}}
	if answer, body := sendOverTLS(t, overTLS, http.MethodGet, image, "", byHostOnly); answer.StatusCode !=
		http.StatusOK || body != "RIFF....WEBP" {
		t.Errorf("an image by the host-only cookie: %d %q", answer.StatusCode, body)
	}
	byPlain := http.Header{"Cookie": {MediaCookieName + "=" + hostOnly.Value}}
	if answer, _ := sendOverTLS(t, overTLS, http.MethodGet, image, "", byPlain); answer.StatusCode !=
		http.StatusUnauthorized {
		t.Errorf("an image by the plain name over TLS, which another core could have set: %d, want 401",
			answer.StatusCode)
	}
}

// A byte path takes its cookie only from the core's own pages: a load another site's page made, a
// core's on the same site among them, is signed in by nothing but a header (the D308 audit's F-6).
func TestMediaCookie_ALoadFromAnotherSitesPageIsNotSignedInByIt(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	token := harness.createAdminAndSignIn(t)
	hash := strings.Repeat("ef", 32)
	if err := harness.directories.WriteArt(hash, "portrait-s.webp", []byte("RIFF....WEBP")); err != nil {
		t.Fatal(err)
	}
	image := "/bytes/art/" + hash + "/portrait-s.webp"
	for site, want := range map[string]int{
		"": http.StatusOK, "same-origin": http.StatusOK, "none": http.StatusOK,
		"same-site": http.StatusUnauthorized, "cross-site": http.StatusUnauthorized,
	} {
		header := http.Header{"Cookie": {MediaCookieName + "=" + token}}
		if site != "" {
			header.Set("Sec-Fetch-Site", site)
		}
		if answer, _ := harness.doWithHeaders(t, http.MethodGet, image, "", header); answer.StatusCode != want {
			t.Errorf("an image by the cookie, Sec-Fetch-Site %q: %d, want %d", site, answer.StatusCode, want)
		}
	}
	byHeader := http.Header{"Authorization": {"Bearer " + token}, "Sec-Fetch-Site": {"same-site"}}
	if answer, _ := harness.doWithHeaders(t, http.MethodGet, image, "", byHeader); answer.StatusCode != http.StatusOK {
		t.Errorf("an image by the header from another site's page: %d, want 200", answer.StatusCode)
	}
}
