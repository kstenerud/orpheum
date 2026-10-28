// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/playback"
	"github.com/kstenerud/orpheum/internal/core/textindex"
	"github.com/kstenerud/orpheum/internal/core/together"
	"github.com/kstenerud/orpheum/internal/core/turn"
)

// tagTogether groups Watch Together's operations.
const tagTogether = "Watch Together"

// OpenTogetherOutput is the host's session, started now, nothing playing, and a membership
// credential their devices say hello with (D246 §5).
type OpenTogetherOutput struct {
	Body struct {
		SessionID            int64  `json:"sessionId"`
		MembershipCredential string `json:"membershipCredential"`
	}
}

// TogetherInput names a session its host acts on.
type TogetherInput struct {
	AuthorizationHeader
	SessionID int64 `minimum:"1" path:"id"`
}

// TogetherLinkBody is one person's link, as the host sees it.
type TogetherLinkBody struct {
	ID       int64  `json:"id"`
	HostNote string `doc:"Who it is for; only the host sees it" json:"hostNote"`
	Secret   string `doc:"The link's secret, after # in its address, to pass on again (D267 §5)" json:"secret"`
	Link     string `doc:"The link to pass on, while a discovery has claimed the server" json:"link,omitempty"`
	// UsedByMemberID is the member who joined by it; absent while it waits.
	UsedByMemberID int64              `json:"usedByMemberId,omitempty"`
	Rights         TogetherRightsBody `doc:"What the person who joins by it may do to playback" json:"rights"`
	// ExpiresAt is when it stops admitting anyone, while it waits (D346).
	ExpiresAt int64 `doc:"Unix milliseconds after which it admits no one; absent once used (D346)" json:"expiresAt,omitempty"` //nolint:lll // one struct tag
}

// TogetherRightsBody is what a guest may do to playback, each given or withheld in their link
// (D271 §4).
type TogetherRightsBody struct {
	CanPlayPause bool `doc:"Play and pause for everyone" json:"canPlayPause"`
	CanSeek      bool `doc:"Change the position for everyone" json:"canSeek"`
}

// TogetherMemberBody is one member, as the host sees them.
type TogetherMemberBody struct {
	ID       int64  `json:"id"`
	Name     string `doc:"The name they chose, which everyone sees" json:"name"`
	HostNote string `doc:"The note of the link they joined by; only the host sees it" json:"hostNote,omitempty"`
	IsHost   bool   `json:"isHost"`
}

// TogetherDetailsOutput is a session as its host sees it: its links and members, with the host's
// notes, and what plays.
type TogetherDetailsOutput struct {
	Body struct {
		SessionID int64                `json:"sessionId"`
		Links     []TogetherLinkBody   `json:"links"`
		Members   []TogetherMemberBody `json:"members"`
		FileID    int64                `doc:"What plays; absent while nothing does" json:"fileId,omitempty"`
		// TitleName and DurationMs are what plays, for the remote on the session's page (D364 §15).
		TitleName  string `doc:"The name of what plays; absent while nothing does" json:"titleName,omitempty"`
		DurationMs int64  `doc:"How long what plays lasts, in ms; absent when unknown" json:"durationMs,omitempty"`
		// NextRights are what a new link starts with: the last one's, every right for the first.
		NextRights TogetherRightsBody `json:"nextRights"`
	}
}

// TogetherLinkInput makes one person's link, with the host's note of who it is for (D246 §3).
type TogetherLinkInput struct {
	AuthorizationHeader
	SessionID int64 `minimum:"1" path:"id"`
	Body      struct {
		HostNote string              `doc:"Who it is for: \"Grandma\"; only the host sees it" json:"hostNote" maxLength:"200" minLength:"1"`          //nolint:lll // one struct tag
		Rights   *TogetherRightsBody `doc:"What they may do to playback; absent, the last link's (D271 §4)" json:"rights,omitempty" required:"false"` //nolint:lll // one struct tag
	}
}

// TogetherLinkOutput is the new link: its id, and its secret, which goes after # in the link
// (D243 §3).
type TogetherLinkOutput struct {
	Body struct {
		LinkID int64  `json:"linkId"`
		Secret string `json:"secret"`
		Link   string `doc:"The link to pass on, while a discovery has claimed the server" json:"link,omitempty"`
	}
}

// RemoveTogetherLinkInput names a link the host withdraws.
type RemoveTogetherLinkInput struct {
	AuthorizationHeader
	SessionID int64 `minimum:"1" path:"id"`
	LinkID    int64 `minimum:"1" path:"link"`
}

// JoinTogetherInput joins a session by a link's secret, read from after # (D243 §3), with the name
// the others will see (D246 §3).
type JoinTogetherInput struct {
	// AuthorizationHeader is optional: someone signed in joins as themselves, and their session
	// follows them to their other devices (D267 §2).
	AuthorizationHeader
	Body struct {
		Secret string `json:"secret" maxLength:"100" minLength:"1"`
		Name   string `doc:"How the others see you" json:"name" maxLength:"100" minLength:"1"`
		// HeldCredential and ShouldLeaveOther are a person's other session (D372 §8): one this
		// browser keeps a membership of, by link alone, and their yes to leaving it, or the one they
		// are in signed in, to join this one.
		HeldCredential   string `doc:"A membership this browser keeps, of another session" json:"heldCredential,omitempty" maxLength:"100" required:"false"`                              //nolint:lll // one struct tag
		ShouldLeaveOther bool   `doc:"Leave the other session you are in, which a join is refused for otherwise (in_another_session)" json:"shouldLeaveOther,omitempty" required:"false"` //nolint:lll // one struct tag
	}
}

// JoinTogetherOutput is the membership: its credential, which every device of the person says hello
// with, and which their player plays by.
type JoinTogetherOutput struct {
	Body struct {
		SessionID            int64  `json:"sessionId"`
		MemberID             int64  `json:"memberId"`
		MembershipCredential string `json:"membershipCredential"`
		Locator              string `doc:"What this device finds the server by through a discovery" json:"locator"`
	}
}

// TogetherMemberInput names a member of a session, for the host: one to remove, or to give a new link.
type TogetherMemberInput struct {
	AuthorizationHeader
	SessionID int64 `minimum:"1" path:"id"`
	MemberID  int64 `minimum:"1" path:"member"`
}

// ViewerPlaybackInput starts a viewer's playback of what the session plays now: the membership
// credential in the Authorization header, never in the URL (D154), and how this device plays, as
// POST /playback is told; no viewer is asked how they hear it (D336 §4).
type ViewerPlaybackInput struct {
	AuthorizationHeader
	Body struct {
		PlayableFormats PlayableFormatsBody `json:"playableFormats"`
	}
	ClientFacts
}

// ViewerPlaybackOutput is the decision, as POST /playback answers, and the viewer cookie, which
// carries the membership to the byte paths for the player (D246 §3).
type ViewerPlaybackOutput struct {
	SetCookie []http.Cookie `header:"Set-Cookie"`
	Body      StartPlaybackOutputBody
}

// ICEInput asks for a member's voice relay credentials: their membership credential in the
// Authorization header, the host's or a viewer's.
type ICEInput struct {
	AuthorizationHeader
	SessionID int64 `minimum:"1" path:"id"`
}

// ICEServerBody is one of a browser's RTCIceServers.
type ICEServerBody struct {
	URLs       []string `json:"urls"`
	Username   string   `json:"username"`
	Credential string   `json:"credential"`
	ExpiresAt  int64    `doc:"Unix milliseconds; ask again before" json:"expiresAt"`
}

// ICEOutput is the member's ICE servers: none when the server relays nothing, and the browsers
// connect directly.
type ICEOutput struct {
	Body struct {
		ICEServers []ICEServerBody `json:"iceServers"`
	}
}

// LeaveTogetherInput is a viewer leaving: their membership credential in the Authorization header.
type LeaveTogetherInput struct {
	AuthorizationHeader
}

func (server *Server) registerTogether() {
	tags := []string{tagTogether}
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "openTogether", Method: http.MethodPost, Path: "/together", Tags: tags,
		Summary: "Start a Watch Together session, nothing playing, ending the one you host and leaving one " +
			"you joined (D246 §5, D372 §8)",
	}, server.openTogether)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "listMyTogether", Method: http.MethodGet, Path: "/together/mine", Tags: tags,
		Summary: "The sessions you are in, as host or as a member who joined signed in, each with a " +
			"credential for this device (D267 §2)",
	}, server.listMyTogether)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getTogether", Method: http.MethodGet, Path: "/together/{id}", Tags: tags,
		Summary: "The session's links and members, with your notes (host)",
	}, server.getTogether)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "addTogetherLink", Method: http.MethodPost, Path: "/together/{id}/links", Tags: tags,
		DefaultStatus: http.StatusCreated,
		Summary:       "A link for one person to join by, once, with a note of who it is for (host; D246 §3)",
	}, server.addTogetherLink)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "addTogetherLinkFor", Method: http.MethodPost, Path: "/together/{id}/members/{member}/links",
		Tags: tags, DefaultStatus: http.StatusCreated,
		Summary: "A new link for a guest's place: they stay in until it is used, and whoever joins by it " +
			"takes the place over, cutting off the devices they came in on (host; D311 §5, D312 §3)",
	}, server.addTogetherLinkFor)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "removeTogetherLink", Method: http.MethodDelete, Path: "/together/{id}/links/{link}",
		Tags: tags, DefaultStatus: http.StatusNoContent, Summary: "Withdraw a link not yet used (host)",
	}, server.removeTogetherLink)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "joinTogether", Method: http.MethodPost, Path: "/together/join", Tags: tags,
		Summary: "Join a session by your link, with the name the others see; no account needed (D246 §3)",
	}, server.joinTogether)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "playTogether", Method: http.MethodPost, Path: "/together/playback", Tags: tags,
		Summary: "A viewer's playback of what the session plays now (D246 §3)",
	}, server.playTogether)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getTogetherICE", Method: http.MethodGet, Path: "/together/{id}/ice", Tags: tags,
		Summary: "A member's voice relay credentials, which end with the membership (D243 §8)",
	}, server.getTogetherICE)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "leaveTogether", Method: http.MethodPost, Path: "/together/leave", Tags: tags,
		DefaultStatus: http.StatusNoContent, Summary: "Leave the session you joined (a viewer)",
	}, server.leaveTogether)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "ejectFromTogether", Method: http.MethodPost, Path: "/together/{id}/eject/{member}",
		Tags: tags, DefaultStatus: http.StatusNoContent,
		Summary: "Remove a member; their devices are told and their credential admits nothing (host; D119)",
	}, server.ejectFromTogether)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "endTogether", Method: http.MethodDelete, Path: "/together/{id}", Tags: tags,
		DefaultStatus: http.StatusNoContent, Summary: "End the session for everyone (host)",
	}, server.endTogether)
}

func (server *Server) openTogether(ctx context.Context, input *AuthorizationHeader) (*OpenTogetherOutput, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if !principal.CanHostWatchTogether() {
		return nil, newError(http.StatusForbidden, CodeForbidden, "your role may not host Watch Together")
	}
	opened, err := dependencies.Together.Open(together.Person{UserID: principal.UserID, Name: principal.Name})
	if err != nil {
		return nil, server.togetherError(ctx, "start a session", err)
	}
	server.logger.InfoContext(ctx, "watch together: a host opened a session",
		"sessionId", opened.SessionID, "hostUserId", principal.UserID)
	output := &OpenTogetherOutput{}
	output.Body.SessionID, output.Body.MembershipCredential = opened.SessionID, opened.MembershipCredential
	return output, nil
}

func (server *Server) getTogether(ctx context.Context, input *TogetherInput) (*TogetherDetailsOutput, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	details, err := dependencies.Together.DetailsOf(input.SessionID, principal.UserID)
	if err != nil {
		return nil, server.togetherError(ctx, "read a session", err)
	}
	output := &TogetherDetailsOutput{}
	output.Body.SessionID, output.Body.FileID = details.SessionID, details.Group.FileID
	if details.Group.FileID != 0 {
		// What plays is the host's, in a library they reach; one not read now leaves its name out.
		work, workErr := dependencies.Browser.WorkOfFile(ctx, principal.Reach, details.Group.FileID)
		if workErr == nil {
			output.Body.TitleName, output.Body.DurationMs = work.Name, work.DurationMs
		}
	}
	output.Body.NextRights = TogetherRightsBody(details.NextRights)
	output.Body.Links = make([]TogetherLinkBody, 0, len(details.Links))
	for _, each := range details.Links {
		output.Body.Links = append(output.Body.Links, TogetherLinkBody{
			ID: each.ID, HostNote: each.HostNote, Secret: each.Secret, UsedByMemberID: each.UsedBy,
			Rights: TogetherRightsBody(each.Rights), Link: togetherLink(dependencies, each.Secret),
			ExpiresAt: waitingUntil(each),
		})
	}
	output.Body.Members = make([]TogetherMemberBody, 0, len(details.Members))
	for _, each := range details.Members {
		output.Body.Members = append(output.Body.Members, TogetherMemberBody{
			ID: each.ID, Name: each.Name, HostNote: each.HostNote, IsHost: each.IsHost,
		})
	}
	return output, nil
}

// waitingUntil is when a link stops admitting anyone, in Unix milliseconds, while it waits; 0 once used.
func waitingUntil(link together.LinkView) int64 {
	if link.UsedBy != 0 {
		return 0
	}
	return link.ExpiresAt.UnixMilli()
}

func (server *Server) addTogetherLink(ctx context.Context, input *TogetherLinkInput) (*TogetherLinkOutput, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	var rights together.Rights
	if input.Body.Rights == nil {
		details, detailsErr := dependencies.Together.DetailsOf(input.SessionID, principal.UserID)
		if detailsErr != nil {
			return nil, server.togetherError(ctx, "make a link", detailsErr)
		}
		rights = details.NextRights
	} else {
		rights = together.Rights(*input.Body.Rights)
	}
	made, err := dependencies.Together.AddLink(
		input.SessionID, principal.UserID, textindex.Line(input.Body.HostNote), rights)
	if err != nil {
		return nil, server.togetherError(ctx, "make a link", err)
	}
	server.logger.InfoContext(ctx, "watch together: the host made a link",
		"sessionId", input.SessionID, "linkId", made.LinkID, "hostNote", input.Body.HostNote,
		"mayPlayPause", rights.CanPlayPause, "maySeek", rights.CanSeek)
	output := &TogetherLinkOutput{}
	output.Body.LinkID, output.Body.Secret = made.LinkID, made.Secret
	output.Body.Link = togetherLink(dependencies, made.Secret)
	return output, nil
}

func (server *Server) addTogetherLinkFor(ctx context.Context, input *TogetherMemberInput) (*TogetherLinkOutput, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	made, err := dependencies.Together.AnotherLinkFor(input.SessionID, principal.UserID, input.MemberID)
	if err != nil {
		return nil, server.togetherError(ctx, "make a new link for a guest", err)
	}
	server.logger.InfoContext(ctx, "watch together: the host made a new link for a guest's place",
		"sessionId", input.SessionID, "memberId", input.MemberID, "linkId", made.LinkID)
	output := &TogetherLinkOutput{}
	output.Body.LinkID, output.Body.Secret = made.LinkID, made.Secret
	output.Body.Link = togetherLink(dependencies, made.Secret)
	return output, nil
}

func (server *Server) removeTogetherLink(ctx context.Context, input *RemoveTogetherLinkInput) (*struct{}, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if err = dependencies.Together.RemoveLink(input.SessionID, principal.UserID, input.LinkID); err != nil {
		return nil, server.togetherError(ctx, "withdraw a link", err)
	}
	server.logger.InfoContext(ctx, "watch together: the host withdrew a link",
		"sessionId", input.SessionID, "linkId", input.LinkID)
	return &struct{}{}, nil
}

// signedInPerson is the person a Watch Together join or device is signed in as, by its session token:
// nil for none, or for one lapsed or ended, which joins as nobody, as the link allows. A token that
// could not be checked is the server's failure: taken for nobody, the session would not have followed
// its person (D267 §2), and nothing said why (the M4 closing audit, 2026-10-08).
func signedInPerson(ctx context.Context, dependencies *Dependencies, token string) (*together.Person, error) {
	if token == "" {
		return nil, nil //nolint:nilnil // no sign-in is no error
	}
	principal, err := dependencies.Auth.Authenticate(ctx, token)
	switch {
	case err == nil:
		return &together.Person{UserID: principal.UserID, Name: principal.Name}, nil
	case errors.Is(err, auth.ErrNotAuthenticated):
		return nil, nil //nolint:nilnil // a lapsed sign-in joins as nobody
	}
	return nil, fmt.Errorf("check a session token: %w", err)
}

func (server *Server) joinTogether(ctx context.Context, input *JoinTogetherInput) (*JoinTogetherOutput, error) {
	dependencies, err := server.readyDependencies()
	if err != nil {
		return nil, err
	}
	name := textindex.Line(input.Body.Name)
	if name == "" {
		return nil, invalidRequestAt(http.StatusUnprocessableEntity, "body.name", "a name is needed")
	}
	person, err := signedInPerson(ctx, dependencies, bearerToken(input.Authorization))
	if err != nil {
		return nil, server.authError(ctx, "check the sign-in of someone joining by a link", err)
	}
	joined, err := dependencies.Together.Join(input.Body.Secret, name, person, together.Leaving{
		HeldCredential: input.Body.HeldCredential, ShouldLeave: input.Body.ShouldLeaveOther,
	})
	if err != nil {
		server.logger.InfoContext(ctx, "watch together: a join was refused", "name", name, "err", err)
		return nil, server.togetherError(ctx, "join a session", err)
	}
	server.logger.InfoContext(ctx, "watch together: someone joined by a link",
		"sessionId", joined.SessionID, "memberId", joined.MemberID, "name", name, "isSignedIn", person != nil)
	output := &JoinTogetherOutput{}
	output.Body.SessionID, output.Body.MemberID = joined.SessionID, joined.MemberID
	output.Body.MembershipCredential, output.Body.Locator = joined.MembershipCredential, joined.Locator
	return output, nil
}

// playTogether starts a viewer's playback of the session's film, in the host's libraries: a viewer
// plays nothing else, and nothing once the host no longer reaches it (D246 §3, D243 §4).
func (server *Server) playTogether(ctx context.Context, input *ViewerPlaybackInput) (*ViewerPlaybackOutput, error) {
	dependencies, err := server.readyDependencies()
	if err != nil {
		return nil, err
	}
	credential := bearerToken(input.Authorization)
	holder, isViewer := server.viewerHolder(ctx, dependencies, credential)
	if !isViewer {
		return nil, newError(http.StatusNotFound, CodeNotFound, "nothing plays for you in a session now")
	}
	started, err := dependencies.Playback.Start(ctx, holder, playback.Request{
		FileID: holder.OnlyFileID, PlayableFormats: playableFormatsOf(input.Body.PlayableFormats),
		ClientAddress: input.clientAddress,
	})
	sessionID, memberID, _ := dependencies.Together.MemberOf(credential)
	who := []any{
		"sessionId", sessionID, "memberId", memberID, "fileId", holder.OnlyFileID, "userAgent", input.userAgent,
	}
	if unreadable, isUnreadable := errors.AsType[*library.UnreadableError](err); isUnreadable {
		server.logPlaybackRefused(ctx, who, err)
		return nil, mediaUnreachable(unreadable, server.nodeNamer(ctx, dependencies))
	}
	if err != nil {
		server.logPlaybackRefused(ctx, who, err)
		return nil, server.playbackError(ctx, "start a viewer's playback", err)
	}
	server.logPlaybackStarted(ctx, who, started)
	named, _, err := server.namedOutput(ctx, dependencies, holder.Reach, holder.OnlyFileID, started)
	if err != nil {
		return nil, err
	}
	return &ViewerPlaybackOutput{SetCookie: viewerCookies(credential, input.isOverTLS), Body: named.Body}, nil
}

func (server *Server) getTogetherICE(ctx context.Context, input *ICEInput) (*ICEOutput, error) {
	dependencies, err := server.readyDependencies()
	if err != nil {
		return nil, err
	}
	sessionID, memberID, isMember := dependencies.Together.MemberOf(bearerToken(input.Authorization))
	if !isMember || sessionID != input.SessionID {
		return nil, newError(http.StatusNotFound, CodeNotFound, together.ErrNoSession.Error())
	}
	servers, err := dependencies.Relay.ICEServersFor(ctx, turn.Membership{SessionID: sessionID, MemberID: memberID},
		dependencies.Clock.Now())
	if err != nil {
		return nil, server.logInternalError(ctx, "issue relay credentials", err)
	}
	output := &ICEOutput{}
	output.Body.ICEServers = make([]ICEServerBody, 0, len(servers))
	for _, each := range servers {
		output.Body.ICEServers = append(output.Body.ICEServers, ICEServerBody{
			URLs: each.URLs, Username: each.Credentials.Username, Credential: each.Credentials.Password,
			ExpiresAt: each.Credentials.ExpiresAt.UnixMilli(),
		})
	}
	return output, nil
}

func (server *Server) leaveTogether(ctx context.Context, input *LeaveTogetherInput) (*struct{}, error) {
	dependencies, err := server.readyDependencies()
	if err != nil {
		return nil, err
	}
	credential := bearerToken(input.Authorization)
	sessionID, memberID, _ := dependencies.Together.MemberOf(credential)
	if err = dependencies.Together.Leave(credential); err != nil {
		server.logger.InfoContext(ctx, "watch together: a leave was refused", "err", err)
		return nil, server.togetherError(ctx, "leave a session", err)
	}
	server.logger.InfoContext(ctx, "watch together: a member left at their own asking",
		"sessionId", sessionID, "memberId", memberID)
	return &struct{}{}, nil
}

func (server *Server) ejectFromTogether(ctx context.Context, input *TogetherMemberInput) (*struct{}, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if err = dependencies.Together.Eject(input.SessionID, principal.UserID, input.MemberID); err != nil {
		return nil, server.togetherError(ctx, "eject a member", err)
	}
	server.logger.InfoContext(ctx, "watch together: the host removed a member",
		"sessionId", input.SessionID, "memberId", input.MemberID, "hostUserId", principal.UserID)
	return &struct{}{}, nil
}

func (server *Server) endTogether(ctx context.Context, input *TogetherInput) (*struct{}, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if err = dependencies.Together.End(input.SessionID, principal.UserID); err != nil {
		return nil, server.togetherError(ctx, "end a session", err)
	}
	server.logger.InfoContext(ctx, "watch together: the host ended the session", "sessionId", input.SessionID)
	return &struct{}{}, nil
}

// togetherError maps Watch Together's refusals to the API's errors.
func (server *Server) togetherError(ctx context.Context, action string, err error) error {
	switch {
	case errors.Is(err, together.ErrNoSession), errors.Is(err, together.ErrNoMember),
		errors.Is(err, together.ErrNoLink):
		return newError(http.StatusNotFound, CodeNotFound, err.Error())
	case errors.Is(err, together.ErrNotHost):
		return newError(http.StatusForbidden, CodeForbidden, err.Error())
	case errors.Is(err, together.ErrNoNote):
		return invalidRequestAt(http.StatusUnprocessableEntity, "body.hostNote", err.Error())
	}
	if other, isInAnother := errors.AsType[*together.InAnotherSessionError](err); isInAnother {
		return newDetailedError(http.StatusConflict, err.Error(), InAnotherSessionDetailBody{
			HostName: other.HostName, IsHost: other.IsHost,
		})
	}
	return server.logInternalError(ctx, action, err)
}
