// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"context"
	"net/http"
)

// MediaCookie is the cookie that carries the session to the byte paths — images now, media
// later — for what cannot send a header, such as an <img> (D154). It is HttpOnly, SameSite
// Strict and scoped to the byte paths; the API takes the header and never this cookie, so a
// forged cross-site request can only fetch what its user may already see.
const MediaCookie = "orpheum_media"

// mediaCookieMaxAge keeps the cookie as long as a browser allows (400 days): whether its session
// is still good is the server's to say, by the session's own expiry and revocation.
const mediaCookieMaxAge = 400 * 24 * 60 * 60

// mediaCookie is the media cookie carrying a session. It is Secure when the request came over
// TLS; before the HTTPS name (M4) a browser on plain HTTP would never send a Secure cookie back
// (plan-deviations P20).
func mediaCookie(token string, overTLS bool) http.Cookie {
	return http.Cookie{ //nolint:gosec // Secure only over TLS until M4's HTTPS name (P20)
		Name: MediaCookie, Value: token, Path: Prefix + "/art/", MaxAge: mediaCookieMaxAge,
		HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: overTLS,
	}
}

// MediaCredentials are a byte path's: the Authorization header, or the media cookie.
type MediaCredentials struct {
	AuthorizationHeader
	Cookie string `cookie:"orpheum_media" doc:"The media cookie sign-in sets, for an <img>"`
}

// signedInForMedia authenticates a byte path's request by its header, else its media cookie.
func (server *Server) signedInForMedia(ctx context.Context, credentials MediaCredentials) (*Dependencies, error) {
	authorization := credentials.Authorization
	if authorization == "" && credentials.Cookie != "" {
		authorization = "Bearer " + credentials.Cookie
	}
	dependencies, _, err := server.signedIn(ctx, authorization)
	return dependencies, err
}
