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
	Command   string           `doc:"What installs the update, run on the server's machine as the user who installed it (D314); empty with no update" json:"command,omitempty"` //nolint:lll // one struct tag
	CheckedAt int64            `doc:"Unix milliseconds the server's discovery last answered; 0 before it has" json:"checkedAt"`                                                 //nolint:lll // one struct tag
}

// UpdateOutput is GET /system/update's answer.
type UpdateOutput struct {
	Body UpdateBody
}

func (server *Server) registerUpdate() {
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getSystemUpdate", Method: http.MethodGet, Path: "/system/update",
		Tags:    []string{tagRunningIt},
		Summary: "What the server knows of an update: a security fix its version lacks, or a newer release (admin)",
	}, server.getSystemUpdate)
}

func (server *Server) getSystemUpdate(ctx context.Context, input *AuthorizationHeader) (*UpdateOutput, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if dependencies.Releases == nil {
		return &UpdateOutput{Body: UpdateBody{Version: dependencies.Version}}, nil
	}
	return &UpdateOutput{Body: updateBody(dependencies.Releases.Release(), dependencies.PublishedImage)}, nil
}

// updateBody is known as the API sends it, its command the install script of the release to go to
// from image, the repository each version is published in: the latest, which has every fix.
func updateBody(known discovery.Release, image string) UpdateBody {
	body := UpdateBody{Version: known.Version, Latest: known.Latest, Digest: known.Digest}
	if !known.CheckedAt.IsZero() {
		body.CheckedAt = known.CheckedAt.UnixMilli()
	}
	target := known.Latest
	if known.Security != nil {
		body.Security = &SecurityFixBody{Summary: known.Security.Summary, FixedIn: known.Security.FixedIn}
		if target == "" {
			target = known.Security.FixedIn
		}
	}
	if target != "" && image != "" {
		// The install script of the release's own image, run again over the household's deployment,
		// updates it and keeps everything (D314; install.sh).
		body.Command = "docker run --rm " + image + ":" + target + " install-script | sh"
	}
	return body
}
