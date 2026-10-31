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

	"github.com/kstenerud/orpheum/discovery/addressdoc"
	"github.com/kstenerud/orpheum/discovery/trust"
)

// documentCheckEvery is how often the core looks for a newer address document, at its issuer and at
// each location its document names: hourly, as long as a cache may keep one (single's servedFor).
const documentCheckEvery = time.Hour

// maxDocumentBytes bounds an address document as served, its root key sets' statements with it.
const maxDocumentBytes = 256 << 10

// ErrSuggestedHasNoRootKeys is enrolling with the suggested discovery in a build whose suggestion
// names no root keys: the core trusts no set the issuer shows in their place (D391 §7).
var ErrSuggestedHasNoRootKeys = errors.New("this build does not know the suggested discovery's root keys " +
	"(internal/core/discovery/suggested.go names none), so the core cannot trust it and is not enrolled")

// held is the discovery as the core holds it: the root key set it pinned, and the last address
// document that set signed, the one whose addresses it uses.
type held struct {
	pinned   trust.Pinned
	document addressdoc.Document
}

// openFirst is the address document at issuer, opened against the root key set the core is to pin:
// the suggestion's when issuer is the suggested one, else the set the document shows, its lowest
// version signed by its own key (trust.FirstUse), which the admin's enrolment trusts (D391 §3).
func (service *Service) openFirst(ctx context.Context, issuer string) (held, error) {
	anchor, isAnchored, err := service.anchorFor(issuer)
	if err != nil {
		return held{}, err
	}
	served, err := service.fetchDocument(ctx, issuer+addressdoc.WellKnownPath)
	if err != nil {
		return held{}, err
	}
	if !isAnchored {
		if anchor, err = trust.FirstUse(served.RootSets); err != nil {
			return held{}, fmt.Errorf("the discovery's root key set: %w", err)
		}
	}
	document, pinned, err := addressdoc.Open(served, anchor, issuer, service.parts.Clock.Now())
	if err != nil {
		return held{}, fmt.Errorf("the discovery's address document: %w", err)
	}
	return held{pinned: pinned, document: document}, nil
}

// anchorFor is the set the core pins for issuer before it asks: the suggestion's, when issuer is the
// suggested one, which must name keys; none for any other.
func (service *Service) anchorFor(issuer string) (trust.Pinned, bool, error) {
	suggested := service.parts.Suggested
	if suggested.Issuer == "" || suggested.Issuer != issuer {
		return trust.Pinned{}, false, nil
	}
	anchor, err := trust.Anchor(suggested.RootSetVersion, suggested.RootKeys)
	if errors.Is(err, trust.ErrNoRootKeys) {
		return trust.Pinned{}, true, ErrSuggestedHasNoRootKeys
	}
	if err != nil {
		return trust.Pinned{}, true, fmt.Errorf("the suggested discovery's root keys: %w", err)
	}
	return anchor, true, nil
}

// fetchDocument is the address document served at url.
func (service *Service) fetchDocument(ctx context.Context, url string) (addressdoc.Served, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return addressdoc.Served{}, fmt.Errorf("ask for the address document at %s: %w", url, err)
	}
	response, err := service.parts.HTTP.Do(request)
	if err != nil {
		return addressdoc.Served{}, fmt.Errorf("reach %s: %w", url, err)
	}
	defer func() { _ = response.Body.Close() }() //nolint:errcheck // read, and closed
	if response.StatusCode != http.StatusOK {
		return addressdoc.Served{}, fmt.Errorf("%s answered %s", url, response.Status)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxDocumentBytes))
	if err != nil {
		return addressdoc.Served{}, fmt.Errorf("read %s: %w", url, err)
	}
	var served addressdoc.Served
	if err = json.Unmarshal(body, &served); err != nil {
		return addressdoc.Served{}, fmt.Errorf("%s: %w: %w", url, addressdoc.ErrNotADocument, err)
	}
	return served, nil
}

// checkDocument looks for a newer address document, hourly: at the issuer and at each location the
// document names, every one, taking the newest that the pinned set, or a set it chains to, signed.
// A document newer than the one held moves the core's addresses: a new core API is called from the
// next session on, under the same issuer, which the core's signatures are bound to (D391 §3, §4).
// When none holds, the last good one is kept and the System page says why.
func (service *Service) checkDocument(ctx context.Context) {
	now := service.parts.Clock.Now()
	service.mutex.Lock()
	isDue := service.documentCheckedAt.IsZero() || now.Sub(service.documentCheckedAt) >= documentCheckEvery
	was, issuer := service.held, service.status.Issuer
	service.mutex.Unlock()
	if !isDue || issuer == "" {
		return
	}
	reached, newest := was, was.document
	var problems []error
	for _, url := range was.document.URLs() {
		served, err := service.fetchDocument(ctx, url)
		if err == nil {
			var document addressdoc.Document
			var pinned trust.Pinned
			document, pinned, err = addressdoc.Open(served, was.pinned, issuer, now)
			if err == nil {
				reached.pinned = laterSet(reached.pinned, pinned)
				newest = laterDocument(newest, document)
				continue
			}
		}
		problems = append(problems, fmt.Errorf("%s: %w", url, err))
	}
	reached.document = newest
	service.adoptDocument(ctx, was, reached, now, problems)
}

// adoptDocument holds reached, the set and document checkDocument found, and says what changed.
func (service *Service) adoptDocument(ctx context.Context, was, reached held, now time.Time, problems []error) {
	isNewDocument := reached.document.IssuedAt > was.document.IssuedAt
	isNewSet := reached.pinned.Version > was.pinned.Version
	problem := ""
	if len(problems) == len(was.document.URLs()) {
		problem = "no location served an address document that holds: " + errors.Join(problems...).Error()
	}
	service.mutex.Lock()
	service.documentCheckedAt, service.status.DocumentProblem = now, problem
	if isNewDocument || isNewSet {
		service.held = reached
		if reached.document.CoreAPI != service.status.BaseURL {
			service.status.BaseURL, service.token = reached.document.CoreAPI, ""
		}
	}
	service.mutex.Unlock()
	if problem != "" {
		service.parts.Logger.WarnContext(ctx, "discovery: no address document holds; keeping the last good one",
			"problems", errors.Join(problems...))
	}
	if !isNewDocument && !isNewSet {
		return
	}
	service.parts.Logger.InfoContext(ctx, "discovery: a newer address document is held",
		"issuedAt", reached.document.IssuedAt, "coreApi", reached.document.CoreAPI,
		"rootSetVersion", reached.pinned.Version)
	if err := service.keep(); err != nil {
		service.parts.Logger.WarnContext(ctx, "discovery: could not keep the newer address document", "err", err)
	}
}

// laterSet is the later of two sets, by version.
func laterSet(one, other trust.Pinned) trust.Pinned {
	if other.Version > one.Version {
		return other
	}
	return one
}

// laterDocument is the later of two documents, by when each was issued.
func laterDocument(one, other addressdoc.Document) addressdoc.Document {
	if other.IssuedAt > one.IssuedAt {
		return other
	}
	return one
}
