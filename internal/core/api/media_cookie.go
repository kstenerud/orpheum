// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/playback"
)

// MediaCookieName names the cookie that carries the session to the byte paths, all under /bytes/
// (D216) — images now, media later — for what cannot send a header, such as an <img> (D154). It
// is HttpOnly and SameSite Strict; the API takes the header and never this cookie, so a forged
// request can only fetch what its user may already see. Over TLS it is named with hostOnlyPrefix.
const MediaCookieName = "orpheum_media"

// hostOnlyPrefix names a byte cookie over TLS. A browser keeps such a cookie only from its own host,
// Secure, at the path "/", and never for a parent domain, so no other core can set one in its
// place: every core is the same site, the core zone being on no public suffix list, and one
// core's page could otherwise toss its own cookie onto another's names (the D308 audit's F-6).
const hostOnlyPrefix = "__Host-"

// mediaCookieMaxAgeSeconds keeps the cookie as long as a browser allows (400 days): whether its
// session is still good is the server's to say, by the session's own expiry and revocation.
const mediaCookieMaxAgeSeconds = 400 * 24 * 60 * 60

// mediaCookies set the media cookie carrying a session; see byteCookies.
func mediaCookies(token string, isOverTLS bool) []http.Cookie {
	return byteCookies(MediaCookieName, token, isOverTLS)
}

// byteCookies set a byte cookie, and over TLS also clear the cookie of its plain name that a
// release before host-only names set there, so none is left over.
func byteCookies(name, value string, isOverTLS bool) []http.Cookie {
	cookies := []http.Cookie{byteCookie(name, value, isOverTLS)}
	if isOverTLS {
		leftOver := byteCookie(name, "", false) //nolint:gosec // only ever expired
		leftOver.MaxAge, leftOver.Secure = -1, true
		cookies = append(cookies, leftOver)
	}
	return cookies
}

// clearedCookies clear a byte cookie, under both its names.
func clearedCookies(name string, isOverTLS bool) []http.Cookie {
	cookies := byteCookies(name, "", isOverTLS)
	for index := range cookies {
		cookies[index].MaxAge = -1 //nolint:gosec // an expiry, made as byteCookie makes the cookie
	}
	return cookies
}

// byteCookie is a cookie for the byte paths. Over TLS it is Secure, at "/", and named with
// hostOnlyPrefix; over plain HTTP, as at home on the core's own address, a browser would never send
// a Secure cookie back (plan-deviations P20), so it is scoped to the byte paths by its path alone.
func byteCookie(name, value string, isOverTLS bool) http.Cookie {
	cookie := http.Cookie{ //nolint:gosec // Secure only over TLS (P20)
		Name: name, Value: value, Path: Prefix + "/bytes/", MaxAge: mediaCookieMaxAgeSeconds,
		HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: isOverTLS,
	}
	if isOverTLS {
		cookie.Name, cookie.Path = hostOnlyPrefix+name, "/"
	}
	return cookie
}

// ViewerCookieName names the cookie that carries a Watch Together viewer's membership to the byte
// paths (D246 §3): beside the media cookie, so a browser signed in as its person can also view a
// session without either replacing the other. It is made as the media cookie is.
const ViewerCookieName = "orpheum_viewer"

// viewerCookies set the viewer cookie carrying a membership credential; see byteCookies.
func viewerCookies(credential string, isOverTLS bool) []http.Cookie {
	return byteCookies(ViewerCookieName, credential, isOverTLS)
}

// MediaCredentials are a byte path's: the Authorization header, the media cookie, or a viewer's
// cookie. Read them by their methods, which take a cookie only as cookieValue allows.
type MediaCredentials struct {
	AuthorizationHeader
	PlainCookie       string `cookie:"orpheum_media"          doc:"The media cookie sign-in sets over plain HTTP, for an <img>"`             //nolint:lll // one struct tag
	HostOnlyCookie    string `cookie:"__Host-orpheum_media"   doc:"The media cookie sign-in sets over TLS, for an <img>"`                    //nolint:lll // one struct tag
	PlainViewerCookie string `cookie:"orpheum_viewer"         doc:"A Watch Together viewer's over plain HTTP, set as their playback starts"` //nolint:lll // one struct tag
	HostViewerCookie  string `cookie:"__Host-orpheum_viewer"  doc:"A Watch Together viewer's over TLS, set as their playback starts"`        //nolint:lll // one struct tag
	// FetchSite is where the browser says the request came from (Fetch Metadata): a cookie is taken
	// only from the core's own pages, never from another core's, which is the same site.
	FetchSite string `doc:"The browser's Sec-Fetch-Site" header:"Sec-Fetch-Site"`
	isOverTLS bool
	// ViewerHeader is the viewer's membership as a page on another origin sends it, which no cookie
	// reaches, beside the Authorization its own sign-in may take (discovery-core.md K10; staging,
	// 2026-10-14, where a signed-in guest's film was "gone").
	ViewerHeader string `doc:"A Watch Together viewer's membership, from a page on the discovery's origin" header:"Orpheum-Viewer"` //nolint:lll // one struct tag
}

// ViewerHeaderName is the header a page on the discovery's origin sends its viewer's membership in.
const ViewerHeaderName = "Orpheum-Viewer"

// Resolve records whether the request came over TLS, which says which of a cookie's names counts.
func (credentials *MediaCredentials) Resolve(ctx huma.Context) []error {
	credentials.isOverTLS = ctx.TLS() != nil
	return nil
}

// cookieValue is a byte cookie as this request may use it. A cookie counts only on a request from
// the core's own pages, or one the browser says nothing of, never one another site's page made (a
// core's on the same site among them); and over TLS only by its host-only name, which no other host
// can have set.
func (credentials *MediaCredentials) cookieValue(plain, hostOnly string) string {
	switch credentials.FetchSite {
	case "", "same-origin", "none":
	default:
		return ""
	}
	if credentials.isOverTLS {
		return hostOnly
	}
	return plain
}

// mediaCookie is the media cookie's session as this request may use it.
func (credentials *MediaCredentials) mediaCookie() string {
	return credentials.cookieValue(credentials.PlainCookie, credentials.HostOnlyCookie)
}

// viewerCookie is the viewer cookie's membership as this request may use it.
func (credentials *MediaCredentials) viewerCookie() string {
	return credentials.cookieValue(credentials.PlainViewerCookie, credentials.HostViewerCookie)
}

// asAuthorizationHeader is the credential as an Authorization header: the header's, else the
// cookie's.
func (credentials *MediaCredentials) asAuthorizationHeader() string {
	if cookie := credentials.mediaCookie(); credentials.Authorization == "" && cookie != "" {
		return "Bearer " + cookie
	}
	return credentials.Authorization
}

// requireSignedInForMedia authenticates a byte path's request by its header, else its media
// cookie: the one way every byte path signs a request in.
func (server *Server) requireSignedInForMedia(
	ctx context.Context, credentials MediaCredentials,
) (*Dependencies, auth.Principal, error) {
	return server.requireSignedIn(ctx, credentials.asAuthorizationHeader())
}

// mediaHolder is who a byte path's request holds media tokens as: the person its header or media
// cookie signs in, and a Watch Together viewer its header or viewer cookie names, either or both
// (D246 §3). Neither is the sign-in's refusal.
func (server *Server) mediaHolder(
	ctx context.Context, credentials MediaCredentials,
) (*Dependencies, playback.Holder, error) {
	dependencies, err := server.readyDependencies()
	if err != nil {
		return nil, playback.Holder{}, err
	}
	_, principal, signInErr := server.requireSignedInForMedia(ctx, credentials)
	var viewer *playback.Holder
	for _, credential := range []string{
		bearerToken(credentials.Authorization), credentials.ViewerHeader, credentials.viewerCookie(),
	} {
		if held, isViewer := server.viewerHolder(ctx, dependencies, credential); isViewer {
			viewer = &held
			break
		}
	}
	switch {
	case signInErr == nil:
		holder := playback.HolderOf(principal)
		holder.Viewer = viewer
		return dependencies, holder, nil
	case viewer != nil:
		return dependencies, *viewer, nil
	}
	return nil, playback.Holder{}, signInErr
}

// viewerHolder is the holder a Watch Together viewer's membership credential is, while it lasts and
// the session plays something: the host's libraries, and only that film (D246 §3).
func (server *Server) viewerHolder(
	ctx context.Context, dependencies *Dependencies, credential string,
) (playback.Holder, bool) {
	if credential == "" {
		return playback.Holder{}, false
	}
	viewer, isViewer := dependencies.Together.ViewerOf(credential)
	if !isViewer || viewer.FileID == 0 {
		return playback.Holder{}, false
	}
	reach, err := dependencies.Auth.ReachOfUser(ctx, viewer.HostUserID)
	if err != nil {
		return playback.Holder{}, false
	}
	return playback.Holder{
		SessionKey: viewerKey(credential), Reach: reach, OnlyFileID: viewer.FileID, HostUserID: viewer.HostUserID,
	}, true
}

// viewerKey names a viewer's membership to the playbacks it starts without holding its credential:
// the hex of the credential's SHA-256, as a person's session is named.
func viewerKey(credential string) string {
	hash := sha256.Sum256([]byte(credential))
	return "viewer:" + hex.EncodeToString(hash[:])
}

// The Cache-Control values the byte paths answer with, one per kind of bytes, so each kind is
// spelled once.
const (
	// An image size: its URL names its content, so a client keeps it for good (D212; artwork.md,
	// Content-addressed).
	cacheImmutable = "private, max-age=31536000, immutable"
	// Bytes that never change once made: a chunk or a store's init, and a
	// subtitle track.
	cacheForADay = "private, max-age=86400"
	// A playlist that may change while it is in use.
	cacheRevalidate = "no-cache"
	// A redirect to the nearest image size while the one asked for is not made.
	cacheNever = "no-store"
	// A direct playback's bytes, relayed from the file on its node.
	cacheNeverPrivately = "private, no-store"
)
