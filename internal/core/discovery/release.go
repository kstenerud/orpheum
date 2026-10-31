// SPDX-License-Identifier: AGPL-3.0-or-later

package discovery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/kstenerud/orpheum/discovery/release"
	"github.com/kstenerud/orpheum/discovery/trust"
)

// releaseCheckEvery is how often the core reads its discovery's release record about its version:
// at its start, and daily (security-updates.md Part A).
const releaseCheckEvery = 24 * time.Hour

// releaseRetryAfter is how long after a failed reading the core reads again.
const releaseRetryAfter = time.Hour

// maxRecordBytes bounds a signed release record.
const maxRecordBytes = 256 << 10

// Release is what the core knows of releases newer than its own, for its admins (security-updates.md
// Part B).
type Release struct {
	// Version is this build's: a release's tag, or "dev" for a build of none, which asks nothing.
	Version string
	// Latest is the newest release, only when it is newer than Version, Digest its image's and
	// Location where it may be pulled from; all empty when the core runs the latest, or knows of none.
	Latest, Digest, Location string
	// Security is the security fix this version lacks, nil when none is pending.
	Security *release.Fix
	// CheckedAt is when a record that holds was last read; zero before the first.
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

// errNoRecord is a discovery that serves no release record anywhere: nothing known, no failure.
var errNoRecord = errors.New("no release record is served")

// checkRelease reads the signed release record, at the core's start and daily, an hour after a
// failure: at the discovery's issuer and each location its address document names, taking the
// newest one a root key of the pinned set signed, and keeps what it says of this version (D391 §6).
// A discovery that serves none knows nothing; a record no root key signed is never believed; a
// development build reads nothing. A failure is logged and never the step's problem.
func (service *Service) checkRelease(ctx context.Context, _ Status) {
	now := service.parts.Clock.Now()
	service.mutex.Lock()
	isDue := service.releaseAskedAt.IsZero() || now.Sub(service.releaseAskedAt) >= service.releaseWait
	opened := service.held
	service.mutex.Unlock()
	if !isDue || !(Release{Version: service.parts.Version}).IsRelease() {
		return
	}
	record, err := service.newestRecord(ctx, opened)
	if errors.Is(err, errNoRecord) {
		record, err = release.Record{}, nil
	}
	service.mutex.Lock()
	service.releaseAskedAt, service.releaseWait = now, releaseCheckEvery
	if err != nil {
		service.releaseWait = releaseRetryAfter
		service.mutex.Unlock()
		service.parts.Logger.WarnContext(ctx,
			"discovery: no release record that holds could be read; reading again in an hour", "err", err)
		return
	}
	told := record.For(service.parts.Version)
	was := service.release
	service.release = Release{
		Latest: told.Latest, Digest: told.Digest, Location: told.Location, Security: told.Security, CheckedAt: now,
	}
	service.mutex.Unlock()
	service.toldRelease(ctx, was, told)
}

// newestRecord is the newest record a root key of opened's set signed, among those served at its
// issuer and its document's locations; errNoRecord when none serves one, and why none held when one
// was served but none held.
func (service *Service) newestRecord(ctx context.Context, opened held) (release.Record, error) {
	var newest release.Record
	var problems []error
	isServed := false
	for _, place := range opened.document.Places() {
		url := place + release.WellKnownPath
		signed, isFound, err := service.fetchRecord(ctx, url)
		isServed = isServed || isFound
		if err == nil && isFound {
			var record release.Record
			if record, err = release.Open(signed, opened.pinned); err == nil {
				if record.IssuedAt > newest.IssuedAt {
					newest = record
				}
				continue
			}
		}
		if err != nil {
			problems = append(problems, fmt.Errorf("%s: %w", url, err))
		}
	}
	switch {
	case newest.IssuedAt != 0:
		return newest, nil
	case !isServed && len(problems) == 0:
		return release.Record{}, errNoRecord
	}
	return release.Record{}, errors.Join(problems...)
}

// fetchRecord is the signed record served at url, and whether one is: not-found is none.
func (service *Service) fetchRecord(ctx context.Context, url string) (trust.Signed, bool, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return trust.Signed{}, false, fmt.Errorf("ask for the release record: %w", err)
	}
	response, err := service.parts.HTTP.Do(request)
	if err != nil {
		return trust.Signed{}, false, fmt.Errorf("reach %s: %w", url, err)
	}
	defer func() { _ = response.Body.Close() }() //nolint:errcheck // read, and closed
	switch {
	case response.StatusCode == http.StatusNotFound:
		return trust.Signed{}, false, nil
	case response.StatusCode != http.StatusOK:
		return trust.Signed{}, false, fmt.Errorf("%s answered %s", url, response.Status)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxRecordBytes))
	if err != nil {
		return trust.Signed{}, false, fmt.Errorf("read %s: %w", url, err)
	}
	var signed trust.Signed
	if err = json.Unmarshal(body, &signed); err != nil {
		return trust.Signed{}, true, fmt.Errorf("%w: %w", release.ErrNotARecord, err)
	}
	return signed, true, nil
}

// toldRelease logs what the record changed: a security fix newly pending, or a newer release newly
// known.
func (service *Service) toldRelease(ctx context.Context, was Release, told release.Told) {
	switch {
	case told.Security != nil && (was.Security == nil || *was.Security != *told.Security):
		service.parts.Logger.WarnContext(ctx, "discovery: a security update is available; the admins are shown it",
			"version", service.parts.Version, "fixedIn", told.Security.FixedIn, "latest", told.Latest,
			"summary", told.Security.Summary)
	case told.Latest != was.Latest && told.Latest != "":
		service.parts.Logger.InfoContext(ctx, "discovery: a newer release is available",
			"version", service.parts.Version, "latest", told.Latest)
	}
}
