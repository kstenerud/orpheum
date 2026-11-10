// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/discovery"
)

// Releases says what the server knows of releases newer than its own: discovery.Service, which
// asks its discovery's release record (security-updates.md Part A).
type Releases interface {
	Release() discovery.Release
}

// SecurityFixBody is a security fix the server's version lacks.
type SecurityFixBody struct {
	Summary string `doc:"What the fix fixes, in a sentence for the admin" json:"summary"`
	FixedIn string `doc:"The first release that has it, \"v0.3.2\"" json:"fixedIn"`
}

// UpdateBody is what the server knows of an update for its admins (security-updates.md Part B): a
// security fix its version lacks, shown on every page, or a newer release, shown on its settings.
type UpdateBody struct {
	Version string `doc:"This server's version: a release's tag, or dev for a build of none, which knows of no update" json:"version"`                     //nolint:lll // one struct tag
	Latest  string `doc:"The newest release, when it is newer than this server's; empty when it runs the latest or knows of none" json:"latest,omitempty"` //nolint:lll // one struct tag
	Digest  string `doc:"The newest release's image digest, sha256:..., for the admin to check" json:"digest,omitempty"`
	// Security is the security fix this version lacks; absent when none is pending.
	Security  *SecurityFixBody `json:"security,omitempty"`
	Command   string           `doc:"What installs the update, run on the server's machine as the user who installed it (D314); empty with no update" json:"command,omitempty"`                //nolint:lll // one struct tag
	CheckedAt int64            `doc:"Unix milliseconds a signed release record was last read; 0 before one has" json:"checkedAt"`                                                              //nolint:lll // one struct tag
	StaleAt   int64            `doc:"Unix milliseconds the release record held expired, since when the server cannot confirm it knows of every fix; 0 while current" json:"staleAt,omitempty"` //nolint:lll // one struct tag
	Problem   string           `doc:"What the last reading of the release record found wrong, for the admin" json:"problem,omitempty"`                                                         //nolint:lll // one struct tag
	// InstallRoute is how the server was installed, so the page shows only that way's steps to update
	// (D450 §2).
	InstallRoute string `doc:"How the server was installed: script (the install script) or the route of the compose file it runs; absent when unknown, when every route's steps apply" enum:"script,omv,synology,portainer,casaos,docker" json:"installRoute,omitempty"` //nolint:lll // one struct tag
	// ComposeFile is a newer compose file than the one that started the server, which the household
	// takes by pasting it (D461 §3); absent when it runs the newest or no compose file started it.
	ComposeFile *ComposeFileBody `json:"composeFile,omitempty"`
}

// ComposeFile is the version of the compose file that started the server, 0 when none did, and what
// this build knows of the newest: its version and what each version after the first brings (D461 §3).
type ComposeFile struct {
	Running int
	Newest  int
	Changes map[int]string
}

// ComposeFileBody is a newer compose file than the one that started the server, and what it brings.
type ComposeFileBody struct {
	Version int      `doc:"The version of the compose file that started the server" json:"version"`
	Newest  int      `doc:"The newest compose file's version" json:"newest"`
	Brings  []string `doc:"What each version after the server's brings, oldest first, a sentence each for the admin" json:"brings" nullable:"false"` //nolint:lll // one struct tag
}

// newerComposeFile is the newer compose file there is than the one that started the server; nil when
// none started it or it is the newest.
func newerComposeFile(file ComposeFile) *ComposeFileBody {
	if file.Running < 1 || file.Running >= file.Newest {
		return nil
	}
	body := &ComposeFileBody{Version: file.Running, Newest: file.Newest, Brings: []string{}}
	for version := file.Running + 1; version <= file.Newest; version++ {
		if brings := file.Changes[version]; brings != "" {
			body.Brings = append(body.Brings, brings)
		}
	}
	return body
}

// UpdateOutput is GET /system/update's answer.
type UpdateOutput struct {
	Body UpdateBody
}

func (server *Server) registerUpdate() {
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getSystemUpdate", Method: http.MethodGet, Path: "/system/update",
		Tags:       []string{tagRunningIt},
		Summary:    "What the server knows of an update: a security fix its version lacks, or a newer release",
		Extensions: callableBy(capabilityAdmin),
	}, server.getSystemUpdate)
}

func (server *Server) getSystemUpdate(ctx context.Context, input *AuthorizationHeader) (*UpdateOutput, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	body := UpdateBody{Version: dependencies.Version}
	if dependencies.Releases != nil {
		body = updateBody(dependencies.Releases.Release(), dependencies.PublishedImage)
	}
	body.InstallRoute = dependencies.InstallRoute
	body.ComposeFile = newerComposeFile(dependencies.ComposeFile)
	return &UpdateOutput{Body: body}, nil
}

// updateBody is known as the API sends it, its command the install script of the release to go to
// from image, the repository each version is published in: the latest, which has every fix.
func updateBody(known discovery.Release, image string) UpdateBody {
	body := UpdateBody{Version: known.Version, Latest: known.Latest, Digest: known.Digest, Problem: known.Problem}
	if !known.CheckedAt.IsZero() {
		body.CheckedAt = known.CheckedAt.UnixMilli()
	}
	if !known.StaleSince.IsZero() {
		body.StaleAt = known.StaleSince.UnixMilli()
	}
	target := known.Latest
	if known.Security != nil {
		body.Security = &SecurityFixBody{Summary: known.Security.Summary, FixedIn: known.Security.FixedIn}
		if target == "" {
			target = known.Security.FixedIn
		}
	}
	switch {
	case known.Latest != "" && known.Location != "" && known.Digest != "":
		// From where the signed record says the release may be pulled, by its digest, which the
		// record signed: the registry is a mirror (D391 §6).
		body.Command = "docker run --rm " + known.Location + "@" + known.Digest + " install-script | sh"
	case target != "" && image != "":
		// The install script of the release's own image, run again over the household's deployment,
		// updates it and keeps everything (D314; install.sh).
		body.Command = "docker run --rm " + image + ":" + target + " install-script | sh"
	}
	return body
}
