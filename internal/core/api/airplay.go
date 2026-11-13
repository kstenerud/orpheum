// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/playback"
	"github.com/kstenerud/orpheum/internal/platform/slogx"
)

// airPlayPathPrefix is where AirPlay grants are carried, in the path of the media they serve (D473
// §1, the one exception to D154 §6): an AirPlay receiver is Apple's player, which sends no cookie and no
// header of ours, so the grant can only be in the URL it is handed, and every playlist's relative
// URIs carry it on to the next.
const airPlayPathPrefix = "/bytes/media/g/"

// airPlayHold is how long a request under an AirPlay grant is held for a chunk not made yet, or a
// master playlist for its film's grid: AVPlayer does not honour the 425 and Retry-After hls.js is
// told to ask again by, so the core holds it until the chunk is made (airplay.md, the 2026-11-12
// findings). It is chunkHoldMs's longest.
const airPlayHold = 30 * time.Second

// AirPlayGrantInput asks for an AirPlay grant for one playback the caller holds.
type AirPlayGrantInput struct {
	AuthorizationHeader
	Body struct {
		MediaToken string `doc:"The playback's media token, as POST /playback answered it" json:"mediaToken" maxLength:"200" minLength:"1"` //nolint:lll // one struct tag
	}
}

// AirPlayGrantOutput is the grant made, and where its master playlist is.
type AirPlayGrantOutput struct {
	Body struct {
		MasterPath string    `doc:"The master playlist's path under the grant, on the server's own origin, the grant in it" json:"masterPath"` //nolint:lll // one struct tag
		ExpiresAt  time.Time `doc:"When the grant ends, unless Stop revokes it sooner"                                     json:"expiresAt"`   //nolint:lll // one struct tag
	}
}

// AirPlayRevokeOutput is how many of the session's AirPlay grants Stop revoked.
type AirPlayRevokeOutput struct {
	Body struct {
		Revoked int `doc:"How many of the session's AirPlay grants were still good and are revoked now" json:"revoked"`
	}
}

// AirPlayMasterInput asks for the master playlist of an AirPlay grant's playback.
type AirPlayMasterInput struct {
	AirPlayReceiver

	Grant string `doc:"The AirPlay grant" path:"grant"`
}

// AirPlayFileInput asks for one file of one stream of an AirPlay grant's playback.
type AirPlayFileInput struct {
	AirPlayReceiver

	Grant      string `doc:"The AirPlay grant"                                       path:"grant"`
	StreamName string `doc:"A quality such as q1200k480p, or a sound such as sound0-96k" path:"streamName"`
	FileName   string `doc:"stream.m3u8, init.mp4 or a chunk, NNNNN.m4s"                path:"fileName"`
}

// AirPlayReceiver is the address of the receiver presenting an AirPlay grant: the connection's own,
// never a header's, which a client writes as it likes (D473 §3). Invalid when the connection's
// address cannot be read, which no grant admits.
type AirPlayReceiver struct {
	address netip.Addr
}

// Resolve records the connection's address.
func (receiver *AirPlayReceiver) Resolve(ctx huma.Context) []error {
	if addressPort, err := netip.ParseAddrPort(ctx.RemoteAddr()); err == nil {
		receiver.address = addressPort.Addr()
	}
	return nil
}

func (server *Server) registerAirPlay() {
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "makeAirPlayGrant", Method: http.MethodPost, Path: "/airplay/grants",
		Summary: "A grant in the media's path for one playback the caller holds, for an AirPlay receiver " +
			"to fetch its film with, for a day unless revoked (D473)",
		Tags:          []string{tagPlayback},
		DefaultStatus: http.StatusOK,
		Extensions:    callableBy(capabilitySignedIn),
	}, server.makeAirPlayGrant)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "revokeAirPlayGrants", Method: http.MethodDelete, Path: "/airplay/grants",
		Summary:       "Revokes every AirPlay grant the caller's session made, as Stop and the end of AirPlay do",
		Tags:          []string{tagPlayback},
		DefaultStatus: http.StatusOK,
		Extensions:    callableBy(capabilitySignedIn),
	}, server.revokeAirPlayGrants)
	responses := map[string]*huma.Response{
		"200": {Description: "A playlist, an init or a chunk", Content: map[string]*huma.MediaType{
			"application/vnd.apple.mpegurl": {}, "video/iso.segment": {}, "video/mp4": {},
		}},
		"404": {
			Description: "No such grant, revoked or expired, presented from another address, or a stream not offered",
		},
		"425": {Description: "Not made yet, after the request was held for it"},
	}
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getAirPlayMasterPlaylist", Method: http.MethodGet,
		Path:    airPlayPathPrefix + "{grant}/master.m3u8",
		Summary: "The master playlist of an AirPlay grant's playback, for the receiver to fetch itself",
		Tags:    []string{tagPlayback}, Responses: responses,
		Extensions: callableBy(capabilityNone),
	}, server.getAirPlayMasterPlaylist)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getAirPlayFile", Method: http.MethodGet,
		Path:    airPlayPathPrefix + "{grant}/{streamName}/{fileName}",
		Summary: "One stream's playlist, init or chunk of an AirPlay grant's playback",
		Tags:    []string{tagPlayback}, Responses: responses,
		Extensions: callableBy(capabilityNone),
	}, server.getAirPlayFile)
}

// makeAirPlayGrant answers POST /airplay/grants: the caller's playback, as its session holds
// it, gets a grant for its film alone, good for a day unless Stop revokes it.
func (server *Server) makeAirPlayGrant(
	ctx context.Context, input *AirPlayGrantInput,
) (*AirPlayGrantOutput, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	chunked, err := dependencies.Playback.ChunksOf(ctx, input.Body.MediaToken, playback.HolderOf(principal))
	if err != nil {
		return nil, server.playbackError(ctx, "find the playback to AirPlay", err)
	}
	grant, err := dependencies.Auth.IssueAirPlayGrant(principal, input.Body.MediaToken, chunked.FileID)
	if err != nil {
		return nil, server.logInternalError(ctx, "make an AirPlay grant", err)
	}
	output := &AirPlayGrantOutput{}
	output.Body.MasterPath = Prefix + airPlayPathPrefix + grant + "/master.m3u8"
	output.Body.ExpiresAt = dependencies.Clock.Now().Add(auth.AirPlayGrantLife).UTC()
	return output, nil
}

// revokeAirPlayGrants answers DELETE /airplay/grants: the caller's session's AirPlay grants end, and
// a receiver presenting one is refused from then on.
func (server *Server) revokeAirPlayGrants(
	ctx context.Context, input *AuthorizationHeader,
) (*AirPlayRevokeOutput, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	output := &AirPlayRevokeOutput{}
	output.Body.Revoked = dependencies.Auth.RevokeAirPlayGrants(principal)
	return output, nil
}

// airPlayServed is what an AirPlay grant serves: its playback's token, and the holder that
// holds it, its sender's person and session, reaching what their role reaches now, and the grant's
// film alone.
type airPlayServed struct {
	dependencies *Dependencies
	mediaToken   string
	holder       playback.Holder
}

// airPlayPlayback is what an AirPlay grant serves to a receiver at address. A grant unknown, revoked
// or expired, or presented from an address other than its own, is not found, all alike (D473 §3).
func (server *Server) airPlayPlayback(
	ctx context.Context, grant string, address netip.Addr,
) (airPlayServed, error) {
	dependencies, err := server.readyDependencies()
	if err != nil {
		return airPlayServed{}, err
	}
	kept, err := dependencies.Auth.AirPlayGrantOf(ctx, grant, address)
	if errors.Is(err, auth.ErrNoAirPlayGrant) {
		return airPlayServed{}, newError(http.StatusNotFound, CodeNotFound, err.Error())
	}
	if err != nil {
		return airPlayServed{}, server.logInternalError(ctx, "find an AirPlay grant", err)
	}
	reach, err := dependencies.Auth.ReachOfUser(ctx, kept.UserID)
	if err != nil {
		return airPlayServed{}, server.logInternalError(ctx, "find what an AirPlay grant's person reaches", err)
	}
	return airPlayServed{
		dependencies: dependencies,
		mediaToken:   kept.MediaToken,
		holder: playback.Holder{
			UserID: kept.UserID, SessionKey: kept.SessionKey, Reach: reach, OnlyFileID: kept.FileID,
		},
	}, nil
}

// getAirPlayMasterPlaylist answers GET /bytes/media/g/{grant}/master.m3u8, held while the film has
// no grid (airPlayHold).
func (server *Server) getAirPlayMasterPlaylist(
	ctx context.Context, input *AirPlayMasterInput,
) (*huma.StreamResponse, error) {
	served, err := server.airPlayPlayback(ctx, input.Grant, input.address)
	if err != nil {
		return nil, err
	}
	return server.masterPlaylistOf(ctx, served.dependencies, served.mediaToken, served.holder, airPlayHold)
}

// getAirPlayFile answers GET /bytes/media/g/{grant}/{streamName}/{fileName}, a chunk not made yet
// held for airPlayHold.
func (server *Server) getAirPlayFile(
	ctx context.Context, input *AirPlayFileInput,
) (*huma.StreamResponse, error) {
	served, err := server.airPlayPlayback(ctx, input.Grant, input.address)
	if err != nil {
		return nil, err
	}
	chunked, err := served.dependencies.Playback.ChunksOf(ctx, served.mediaToken, served.holder)
	if err != nil {
		return nil, server.playbackError(ctx, "find an AirPlay grant's chunks", err)
	}
	asked := chunkFileAsked{streamName: input.StreamName, fileName: input.FileName}
	return server.chunkFileOf(ctx, served.dependencies, served.mediaToken, chunked, asked, airPlayHold)
}

// airPlayGrantsPath is where every request under an AirPlay grant comes, the grant the path's
// first segment after it.
const airPlayGrantsPath = Prefix + airPlayPathPrefix

// loggingAirPlay logs each request under an AirPlay grant at debug, once it ends: the receiver's
// address and its own words for itself, the file it asked for, how it ended, the status, the bytes
// sent, how long it was held before its answer began and how long it took (the owner, 2026-11-13:
// the spike's receiver fetched nothing, and nothing said what it asked). A refused grant's request
// is logged too. A request whose receiver went away before its answer is logged as gone, with no
// status: nothing reached it, and it is no fault (the lab's second run, 2026-11-13). The grant is
// never logged, a bearer credential (D194); nor is any media token.
func (server *Server) loggingAirPlay(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		dependencies := server.dependencies.Load()
		rest, isUnderGrant := strings.CutPrefix(request.URL.Path, airPlayGrantsPath)
		if !isUnderGrant || dependencies == nil || dependencies.Clock == nil {
			next.ServeHTTP(writer, request)
			return
		}
		started := dependencies.Clock.Now()
		answered := &airPlayAnswer{ResponseWriter: writer, status: http.StatusOK, now: dependencies.Clock.Now}
		next.ServeHTTP(answered, request)
		ended := dependencies.Clock.Now()
		heldUntil := answered.startedAt
		if heldUntil.IsZero() {
			heldUntil = ended
		}
		client := request.RemoteAddr
		if addressPort, err := netip.ParseAddrPort(request.RemoteAddr); err == nil {
			client = addressPort.Addr().String()
		}
		_, file, _ := strings.Cut(rest, "/")
		attributes := []any{
			"clientAddress", client, logKeyUserAgent, request.Header.Get("User-Agent"), "file", file,
		}
		if request.Context().Err() != nil {
			attributes = append(attributes, "outcome", airPlayClientGone)
		} else {
			attributes = append(attributes, "outcome", airPlayAnswered, "status", answered.status)
		}
		server.logger.DebugContext(request.Context(), "ended a request under an AirPlay grant",
			append(attributes, "bytes", answered.bytes, "heldMs", heldUntil.Sub(started).Milliseconds(),
				"tookMs", ended.Sub(started).Milliseconds(), slogx.EventKey, "airPlayRequest")...)
	})
}

// How a request under an AirPlay grant ended: answered, or its receiver gone before its answer.
const (
	airPlayAnswered   = "answered"
	airPlayClientGone = "clientGone"
)

// airPlayAnswer is the response writer of a request under an AirPlay grant, keeping what its
// log line says: the status, the bytes written, and when the answer began.
type airPlayAnswer struct {
	http.ResponseWriter
	status    int
	bytes     int
	startedAt time.Time
	now       func() time.Time
}

// WriteHeader keeps the status and when the answer began, then writes it.
func (answer *airPlayAnswer) WriteHeader(status int) {
	answer.began()
	answer.status = status
	answer.ResponseWriter.WriteHeader(status)
}

// Write counts the bytes, the answer begun by the first, then writes them.
func (answer *airPlayAnswer) Write(bytes []byte) (int, error) {
	answer.began()
	written, err := answer.ResponseWriter.Write(bytes)
	answer.bytes += written
	return written, err //nolint:wrapcheck // a writer's error, passed on as it is
}

// Unwrap is the writer kept, for http.ResponseController's flushes and deadlines.
func (answer *airPlayAnswer) Unwrap() http.ResponseWriter { return answer.ResponseWriter }

// began marks when the answer began, at its first status or byte.
func (answer *airPlayAnswer) began() {
	if answer.startedAt.IsZero() {
		answer.startedAt = answer.now()
	}
}

// withoutAirPlayGrant is path as a log line may say it: under an AirPlay grant, the grant, a
// bearer credential (D194), is put out, and the file asked for is kept.
func withoutAirPlayGrant(path string) string {
	rest, isUnderGrant := strings.CutPrefix(path, airPlayGrantsPath)
	if !isUnderGrant {
		return path
	}
	_, file, _ := strings.Cut(rest, "/")
	return airPlayGrantsPath + "(grant)/" + file
}
