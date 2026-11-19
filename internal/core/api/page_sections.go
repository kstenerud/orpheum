// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"errors"

	"github.com/kstenerud/orpheum/internal/core/auth"
)

// An admin's page is built of sections that each read their own endpoint. Its first read carries
// what its other sections read, each as its own endpoint answers it, so the page waits on one
// request (the client-server audit's finding 10, principles §30). Each section stays fresh at its
// own endpoint: its poll and a write's refresh read there, and a plain read carries none of this.
//
// GET /system and GET /users are sent gzipped, and so carry no credential (D342): the discovery's
// claim code and root keys, the invitations' links and the secrets' state stay their own reads,
// which are not compressed.

// SectionsInput asks a page's first read for what the page's other sections read.
type SectionsInput struct {
	IsSections bool `doc:"Also what the page's other sections read, each as its own endpoint answers it (sections): the page's first read, not their polls" query:"sections"` //nolint:lll // one struct tag
}

// sectionsPageLimit is the page of a list a first read carries: the client's allPages asks for its
// pages at PageInput's most, so the page carried stands for the first it would ask.
const sectionsPageLimit = 500

// SystemInput asks for the System page's facts, and, for its first read, its sections'.
type SystemInput struct {
	AuthorizationHeader
	SectionsInput
}

// SystemSectionsBody is what the System page's other sections read, each as its endpoint answers
// it. The reachability check, which takes its time, is asked for on its own, as the admin asks, and
// the discovery, which carries credentials (D342), is read on its own.
type SystemSectionsBody struct {
	// NowPlaying is GET /now-playing's answer.
	NowPlaying NowPlayingOutputBody `json:"nowPlaying"`
	// ResourceUse is GET /system/resource-use's answer.
	ResourceUse ResourceUseOutputBody `json:"resourceUse"`
	// Backups is GET /backups' answer.
	Backups BackupsOutputBody `json:"backups"`
	// UnusedSettings is GET /config/unused's answer.
	UnusedSettings UnusedOutputBody `json:"unusedSettings"`
	// FilingReports is GET /filing-reports' first page of 500, its nextCursor the rest's.
	FilingReports FilingReportsOutputBody `json:"filingReports"`
	// ProblemReports is GET /problem-reports' answer.
	ProblemReports ProblemReportsOutputBody `json:"problemReports"`
	// Network is GET /system/network's answer; absent where the server keeps no network settings,
	// its config file's, where that endpoint refuses with a conflict.
	Network *NetworkStateBody `json:"network,omitempty"`
}

// systemSections is what the System page's other sections read.
func (server *Server) systemSections(ctx context.Context, dependencies *Dependencies) (*SystemSectionsBody, error) {
	playing, err := server.nowPlaying(ctx, dependencies)
	if err != nil {
		return nil, err
	}
	use, err := server.resourceUse(ctx, dependencies)
	if err != nil {
		return nil, err
	}
	backups, err := server.backupsListed(ctx, dependencies)
	if err != nil {
		return nil, err
	}
	unused, err := server.unusedSettings(ctx, dependencies)
	if err != nil {
		return nil, err
	}
	reports, err := server.filingReportsPage(ctx, dependencies, PageInput{Limit: sectionsPageLimit})
	if err != nil {
		return nil, err
	}
	problems, err := server.problemReportsListed(ctx, dependencies)
	if err != nil {
		return nil, err
	}
	sections := &SystemSectionsBody{
		NowPlaying: playing.Body, ResourceUse: use.Body, Backups: backups.Body,
		UnusedSettings: unused.Body, FilingReports: reports.Body, ProblemReports: problems.Body,
	}
	network, err := server.networkState(ctx, dependencies)
	switch {
	case errors.Is(err, errNoNetworkSettings):
	case err != nil:
		return nil, err
	default:
		sections.Network = &network.Body
	}
	return sections, nil
}

// UserPageInput asks for a user's page, and, for its first read, its sections'.
type UserPageInput struct {
	UserInput
	SectionsInput
}

// UserSectionsBody is what a user's page's other sections read.
type UserSectionsBody struct {
	// Devices is GET /users/{id}/devices' answer.
	Devices DevicesOutputBody `json:"devices"`
	// Roles is GET /roles' answer: the roles to choose the person's from.
	Roles RolesOutputBody `json:"roles"`
}

// userSections is what a user's page's other sections read, for the admin principal.
func (server *Server) userSections(
	ctx context.Context, dependencies *Dependencies, userID int64, principal auth.Principal,
) (*UserSectionsBody, error) {
	devices, err := server.devicesOf(ctx, dependencies, userID, principal.DeviceID)
	if err != nil {
		return nil, err
	}
	roles, err := server.rolesListed(ctx, dependencies)
	if err != nil {
		return nil, err
	}
	return &UserSectionsBody{Devices: devices.Body, Roles: roles.Body}, nil
}

// UsersSectionsBody is what the Users page's other sections read: the roles. The invitations, which
// carry their links (D342), are read on their own.
type UsersSectionsBody struct {
	// Roles is GET /roles' answer.
	Roles RolesOutputBody `json:"roles"`
}

// usersSections is what the Users page's other sections read.
func (server *Server) usersSections(ctx context.Context, dependencies *Dependencies) (*UsersSectionsBody, error) {
	roles, err := server.rolesListed(ctx, dependencies)
	if err != nil {
		return nil, err
	}
	return &UsersSectionsBody{Roles: roles.Body}, nil
}

// withSections is sections read, or nil, logged, when a part of them cannot be read: the page's own
// answer still comes, and each section reads its own endpoint.
func withSections[T any](
	ctx context.Context, server *Server, isAsked bool, read func() (*T, error),
) *T {
	if !isAsked {
		return nil
	}
	sections, err := read()
	if err != nil {
		server.logger.WarnContext(ctx,
			"a page's sections were left out of its first read; each reads its own", "err", err)
		return nil
	}
	return sections
}
