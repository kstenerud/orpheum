// SPDX-License-Identifier: AGPL-3.0-or-later

package discovery

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
	"github.com/kstenerud/orpheum/discovery/release"
)

// releaseCheckEvery is how often the core asks its discovery's release record about its version:
// at its start, and daily (security-updates.md Part A).
const releaseCheckEvery = 24 * time.Hour

// releaseRetryAfter is how long after a failed asking the core asks again.
const releaseRetryAfter = time.Hour

// Release is what the core knows of releases newer than its own, for its admins (security-updates.md
// Part B).
type Release struct {
	// Version is this build's: a release's tag, or "dev" for a build of none, which asks nothing.
	Version string
	// Latest is the newest release, only when it is newer than Version, and Digest its image's; both
	// empty when the core runs the latest, or knows of none.
	Latest, Digest string
	// Security is the security fix this version lacks, nil when none is pending.
	Security *discoveryproto.SecurityFix
	// CheckedAt is when the discovery last answered; zero before its first answer.
	CheckedAt time.Time
}

// IsRelease is whether the build is a release's, which asks its discovery about newer ones: a
// development build has no version to ask about.
func (known Release) IsRelease() bool {
	_, isRelease := release.Compare(known.Version, known.Version)
	return isRelease
}

// Release is what the core knows now of releases newer than its own.
func (service *Service) Release() Release {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	known := service.release
	known.Version = service.parts.Version
	if known.Security != nil {
		fix := *known.Security
		known.Security = &fix
	}
	return known
}

// checkRelease asks the discovery what its release record says of this version, at the core's start
// and daily, an hour after a failure. A discovery that has no such call, older than it, knows nothing,
// as a discovery whose record says nothing does; a development build asks nothing. A failure is
// logged and never the step's problem: the core's standing does not hang on it.
func (service *Service) checkRelease(ctx context.Context, status Status) {
	now := service.parts.Clock.Now()
	service.mutex.Lock()
	isDue := service.releaseAskedAt.IsZero() || now.Sub(service.releaseAskedAt) >= service.releaseWait
	service.mutex.Unlock()
	if !isDue || !(Release{Version: service.parts.Version}).IsRelease() {
		return
	}
	var answered discoveryproto.ReleaseResponse
	err := service.signedCall(ctx, callOf{
		base: status.BaseURL, method: http.MethodPost, path: discoveryproto.PathRelease,
		body: discoveryproto.ReleaseRequest{OrpheumVersion: service.parts.Version}, into: &answered,
	})
	var refused *statusError
	if errors.As(err, &refused) && refused.code == http.StatusNotFound {
		answered, err = discoveryproto.ReleaseResponse{}, nil
	}
	service.mutex.Lock()
	service.releaseAskedAt, service.releaseWait = now, releaseCheckEvery
	if err != nil {
		service.releaseWait = releaseRetryAfter
		service.mutex.Unlock()
		service.parts.Logger.WarnContext(ctx,
			"discovery: the release record could not be asked; asking again in an hour", "err", err)
		return
	}
	was := service.release
	service.release = Release{
		Latest: answered.Latest, Digest: answered.Digest, Security: answered.Security, CheckedAt: now,
	}
	service.mutex.Unlock()
	service.toldRelease(ctx, was, answered)
}

// toldRelease logs what the discovery's answer changed: a security fix newly pending, or a newer
// release newly known.
func (service *Service) toldRelease(ctx context.Context, was Release, answered discoveryproto.ReleaseResponse) {
	switch {
	case answered.Security != nil && (was.Security == nil || *was.Security != *answered.Security):
		service.parts.Logger.WarnContext(ctx, "discovery: a security update is available; the admins are shown it",
			"version", service.parts.Version, "fixedIn", answered.Security.FixedIn, "latest", answered.Latest,
			"summary", answered.Security.Summary)
	case answered.Latest != was.Latest && answered.Latest != "":
		service.parts.Logger.InfoContext(ctx, "discovery: a newer release is available",
			"version", service.parts.Version, "latest", answered.Latest)
	}
}
