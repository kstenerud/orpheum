// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/playback"
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

// ViewerCookieName names the cookie that carries a Watch Together viewer's membership to the byte
// paths (D246 §3): beside the media cookie, so a browser signed in as its person can also view a
// session without either replacing the other. It is HttpOnly, SameSite Strict and scoped as the
// media cookie is.
const ViewerCookieName = "orpheum_viewer"

// viewerCookie is the viewer cookie carrying a membership credential.
func viewerCookie(credential string, isOverTLS bool) http.Cookie {
	return http.Cookie{ //nolint:gosec // Secure only over TLS, as the media cookie (P20)
		Name: ViewerCookieName, Value: credential, Path: Prefix + "/bytes/", MaxAge: mediaCookieMaxAgeSeconds,
		HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: isOverTLS,
	}
}

// MediaCredentials are a byte path's: the Authorization header, the media cookie, or a viewer's
// cookie.
type MediaCredentials struct {
	AuthorizationHeader
	Cookie       string `cookie:"orpheum_media"  doc:"The media cookie sign-in sets, for an <img>"`
	ViewerCookie string `cookie:"orpheum_viewer" doc:"A Watch Together viewer's, set as their playback starts"`
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
	for _, credential := range []string{bearerToken(credentials.Authorization), credentials.ViewerCookie} {
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
