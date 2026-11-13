// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/playback"
	"github.com/kstenerud/orpheum/internal/core/settings"
)

// labAirPlayPathPrefix is where the AirPlay lab's grants are carried, in the path of the media they
// serve (airplay.md item 3): an AirPlay receiver is Apple's player, which sends no cookie and no
// header of ours, so the grant can only be in the URL it is handed, and every playlist's relative
// URIs carry it on to the next.
const labAirPlayPathPrefix = "/bytes/media/g/"

// labAirPlayHold is how long a request under an AirPlay lab grant is held for a chunk not made yet,
// or a master playlist for its film's grid: AVPlayer does not honour the 425 and Retry-After hls.js
// is told to ask again by, so the core holds it until the chunk is made (airplay.md, the 2026-11-12
// findings). It is chunkHoldMs's longest, a justified constant of the lab's, as the lab is the spike's.
const labAirPlayHold = 30 * time.Second

// LabAirPlayGrantInput asks for the AirPlay lab's grant for one playback the caller holds.
type LabAirPlayGrantInput struct {
	AuthorizationHeader
	Body struct {
		MediaToken string `doc:"The playback's media token, as POST /playback answered it" json:"mediaToken" maxLength:"200" minLength:"1"` //nolint:lll // one struct tag
	}
}

// LabAirPlayGrantOutput is the grant made, and where its master playlist is.
type LabAirPlayGrantOutput struct {
	Body struct {
		MasterPath string    `doc:"The master playlist's path under the grant, on the server's own origin, the grant in it" json:"masterPath"` //nolint:lll // one struct tag
		ExpiresAt  time.Time `doc:"When the grant ends, unless Stop revokes it sooner"                                     json:"expiresAt"`   //nolint:lll // one struct tag
	}
}

// LabAirPlayRevokeOutput is how many of the session's AirPlay lab grants Stop revoked.
type LabAirPlayRevokeOutput struct {
	Body struct {
		Revoked int `doc:"How many of the session's AirPlay grants were still good and are revoked now" json:"revoked"`
	}
}

// LabAirPlayMasterInput asks for the master playlist of an AirPlay lab grant's playback.
type LabAirPlayMasterInput struct {
	Grant string `doc:"The AirPlay lab's grant" path:"grant"`
}

// LabAirPlayFileInput asks for one file of one stream of an AirPlay lab grant's playback.
type LabAirPlayFileInput struct {
	Grant      string `doc:"The AirPlay lab's grant"                                 path:"grant"`
	StreamName string `doc:"A quality such as q1200k480p, or a sound such as sound0-96k" path:"streamName"`
	FileName   string `doc:"stream.m3u8, init.mp4 or a chunk, NNNNN.m4s"                path:"fileName"`
}

func (server *Server) registerLabAirPlay() {
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "makeLabAirPlayGrant", Method: http.MethodPost, Path: "/lab/airplay/grants",
		Summary: "A grant in the media's path for one playback the caller holds, for the AirPlay lab alone, " +
			"while the admin offers it (airplay.md's spike)",
		Tags:          []string{tagRunningIt},
		DefaultStatus: http.StatusOK,
		Extensions:    callableBy(capabilitySignedIn),
	}, server.makeLabAirPlayGrant)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "revokeLabAirPlayGrants", Method: http.MethodDelete, Path: "/lab/airplay/grants",
		Summary:       "Revokes every AirPlay lab grant the caller's session made, as the lab's Stop does",
		Tags:          []string{tagRunningIt},
		DefaultStatus: http.StatusOK,
		Extensions:    callableBy(capabilitySignedIn),
	}, server.revokeLabAirPlayGrants)
	responses := map[string]*huma.Response{
		"200": {Description: "A playlist, an init or a chunk", Content: map[string]*huma.MediaType{
			"application/vnd.apple.mpegurl": {}, "video/iso.segment": {}, "video/mp4": {},
		}},
		"404": {Description: "No such grant, revoked or expired, the lab turned off, or a stream not offered"},
		"425": {Description: "Not made yet, after the request was held for it"},
	}
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getLabAirPlayMasterPlaylist", Method: http.MethodGet,
		Path:    labAirPlayPathPrefix + "{grant}/master.m3u8",
		Summary: "The master playlist of an AirPlay lab grant's playback, for the receiver to fetch itself",
		Tags:    []string{tagPlayback}, Responses: responses,
		Extensions: callableBy(capabilityNone),
	}, server.getLabAirPlayMasterPlaylist)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getLabAirPlayFile", Method: http.MethodGet,
		Path:    labAirPlayPathPrefix + "{grant}/{streamName}/{fileName}",
		Summary: "One stream's playlist, init or chunk of an AirPlay lab grant's playback",
		Tags:    []string{tagPlayback}, Responses: responses,
		Extensions: callableBy(capabilityNone),
	}, server.getLabAirPlayFile)
}

// errLabAirPlayOff is every AirPlay lab path's answer while the admin has the lab off
// (shouldOfferAirPlayLab, off unless turned on): there is no such thing here.
func errLabAirPlayOff() error {
	return newError(http.StatusNotFound, CodeNotFound, "the AirPlay lab is turned off on this server")
}

// requireLabAirPlay refuses while the admin has the AirPlay lab off: every AirPlay lab path's one
// gate, so the path grant exists nowhere the lab is not offered (D154 §6 stays as it is elsewhere).
func (server *Server) requireLabAirPlay(ctx context.Context, dependencies *Dependencies) error {
	isOffered, err := dependencies.Settings.Bool(ctx, settings.ShouldOfferAirPlayLab)
	if err != nil {
		return server.settingsError(ctx, "read "+string(settings.ShouldOfferAirPlayLab), err)
	}
	if !isOffered {
		return errLabAirPlayOff()
	}
	return nil
}

// makeLabAirPlayGrant answers POST /lab/airplay/grants: the caller's playback, as its session holds
// it, gets a grant for its film alone, good for a day unless Stop revokes it.
func (server *Server) makeLabAirPlayGrant(
	ctx context.Context, input *LabAirPlayGrantInput,
) (*LabAirPlayGrantOutput, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if offErr := server.requireLabAirPlay(ctx, dependencies); offErr != nil {
		return nil, offErr
	}
	chunked, err := dependencies.Playback.ChunksOf(ctx, input.Body.MediaToken, playback.HolderOf(principal))
	if err != nil {
		return nil, server.playbackError(ctx, "find the playback to AirPlay", err)
	}
	grant, err := dependencies.Auth.IssueLabAirPlayGrant(principal, input.Body.MediaToken, chunked.FileID)
	if err != nil {
		return nil, server.logInternalError(ctx, "make an AirPlay lab grant", err)
	}
	output := &LabAirPlayGrantOutput{}
	output.Body.MasterPath = Prefix + labAirPlayPathPrefix + grant + "/master.m3u8"
	output.Body.ExpiresAt = dependencies.Clock.Now().Add(auth.LabAirPlayGrantLife).UTC()
	return output, nil
}

// revokeLabAirPlayGrants answers DELETE /lab/airplay/grants: the caller's session's AirPlay lab grants
// end, and a receiver presenting one is refused from then on. Taken while the lab is off too, so a
// grant made before an admin turned it off can still be revoked.
func (server *Server) revokeLabAirPlayGrants(
	ctx context.Context, input *AuthorizationHeader,
) (*LabAirPlayRevokeOutput, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	output := &LabAirPlayRevokeOutput{}
	output.Body.Revoked = dependencies.Auth.RevokeLabAirPlayGrants(principal)
	return output, nil
}

// labAirPlayServed is what an AirPlay lab grant serves: its playback's token, and the holder that
// holds it, its sender's person and session, reaching what their role reaches now, and the grant's
// film alone.
type labAirPlayServed struct {
	dependencies *Dependencies
	mediaToken   string
	holder       playback.Holder
}

// labAirPlayPlayback is what an AirPlay lab grant serves while the lab is offered. A grant unknown,
// revoked or expired is not found.
func (server *Server) labAirPlayPlayback(ctx context.Context, grant string) (labAirPlayServed, error) {
	dependencies, err := server.readyDependencies()
	if err != nil {
		return labAirPlayServed{}, err
	}
	if offErr := server.requireLabAirPlay(ctx, dependencies); offErr != nil {
		return labAirPlayServed{}, offErr
	}
	kept, err := dependencies.Auth.LabAirPlayGrantOf(ctx, grant)
	if errors.Is(err, auth.ErrNoLabAirPlayGrant) {
		return labAirPlayServed{}, newError(http.StatusNotFound, CodeNotFound, err.Error())
	}
	if err != nil {
		return labAirPlayServed{}, server.logInternalError(ctx, "find an AirPlay grant", err)
	}
	reach, err := dependencies.Auth.ReachOfUser(ctx, kept.UserID)
	if err != nil {
		return labAirPlayServed{}, server.logInternalError(ctx, "find what an AirPlay grant's person reaches", err)
	}
	return labAirPlayServed{
		dependencies: dependencies,
		mediaToken:   kept.MediaToken,
		holder: playback.Holder{
			UserID: kept.UserID, SessionKey: kept.SessionKey, Reach: reach, OnlyFileID: kept.FileID,
		},
	}, nil
}

// getLabAirPlayMasterPlaylist answers GET /bytes/media/g/{grant}/master.m3u8, held while the film
// has no grid (labAirPlayHold).
func (server *Server) getLabAirPlayMasterPlaylist(
	ctx context.Context, input *LabAirPlayMasterInput,
) (*huma.StreamResponse, error) {
	served, err := server.labAirPlayPlayback(ctx, input.Grant)
	if err != nil {
		return nil, err
	}
	return server.masterPlaylistOf(ctx, served.dependencies, served.mediaToken, served.holder, labAirPlayHold)
}

// getLabAirPlayFile answers GET /bytes/media/g/{grant}/{streamName}/{fileName}, a chunk not made yet
// held for labAirPlayHold.
func (server *Server) getLabAirPlayFile(
	ctx context.Context, input *LabAirPlayFileInput,
) (*huma.StreamResponse, error) {
	served, err := server.labAirPlayPlayback(ctx, input.Grant)
	if err != nil {
		return nil, err
	}
	chunked, err := served.dependencies.Playback.ChunksOf(ctx, served.mediaToken, served.holder)
	if err != nil {
		return nil, server.playbackError(ctx, "find an AirPlay grant's chunks", err)
	}
	asked := chunkFileAsked{streamName: input.StreamName, fileName: input.FileName}
	return server.chunkFileOf(ctx, served.dependencies, served.mediaToken, chunked, asked, labAirPlayHold)
}
