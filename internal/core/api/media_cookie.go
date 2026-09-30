// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"context"
	"net/http"

	"github.com/kstenerud/orpheum/internal/core/auth"
)

// MediaCookieName names the cookie that carries the session to the byte paths, all under /bytes/
// (D216) — images now, media later — for what cannot send a header, such as an <img> (D154). It
// is HttpOnly, SameSite Strict and scoped to the byte paths; the API takes the header and never
// this cookie, so a forged cross-site request can only fetch what its user may already see.
const MediaCookieName = "orpheum_media"

// mediaCookieMaxAgeSeconds keeps the cookie as long as a browser allows (400 days): whether its
// session is still good is the server's to say, by the session's own expiry and revocation.
const mediaCookieMaxAgeSeconds = 400 * 24 * 60 * 60

// mediaCookie is the media cookie carrying a session. It is Secure when the request came over
// TLS; before the HTTPS name (M4) a browser on plain HTTP would never send a Secure cookie back
// (plan-deviations P20).
func mediaCookie(token string, isOverTLS bool) http.Cookie {
	return http.Cookie{ //nolint:gosec // Secure only over TLS until M4's HTTPS name (P20)
		Name: MediaCookieName, Value: token, Path: Prefix + "/bytes/", MaxAge: mediaCookieMaxAgeSeconds,
		HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: isOverTLS,
	}
}

// MediaCredentials are a byte path's: the Authorization header, or the media cookie.
type MediaCredentials struct {
	AuthorizationHeader
	Cookie string `cookie:"orpheum_media" doc:"The media cookie sign-in sets, for an <img>"`
}

// asAuthorizationHeader is the credential as an Authorization header: the header's, else the
// cookie's.
func (credentials MediaCredentials) asAuthorizationHeader() string {
	if credentials.Authorization == "" && credentials.Cookie != "" {
		return "Bearer " + credentials.Cookie
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

// The Cache-Control values the byte paths answer with, one per kind of bytes, so each kind is
// spelled once.
const (
	// An image size: its URL names its content, so a client keeps it for good (D212; artwork.md,
	// Content-addressed).
	cacheImmutable = "private, max-age=31536000, immutable"
	// Bytes that never change once made: a rendition's segment or finished playlist, and a
	// subtitle track.
	cacheForADay = "private, max-age=86400"
	// A playlist that grows while its rendition is made.
	cacheRevalidate = "no-cache"
	// A redirect to the nearest image size while the one asked for is not made.
	cacheNever = "no-store"
	// A direct playback's bytes, relayed from the file on its node.
	cacheNeverPrivately = "private, no-store"
)
