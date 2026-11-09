// SPDX-License-Identifier: AGPL-3.0-or-later

package discovery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/kstenerud/orpheum/discovery/addressdoc"
	"github.com/kstenerud/orpheum/discovery/trust"
	"github.com/kstenerud/orpheum/pkg/strictjson"
)

// documentCheckEvery is how often the core looks at every place its discovery is served: its root
// chain, then a newer address document. Hourly, as long as a cache may keep one; the audit's §3.4
// asks for the chain every six hours at least.
const documentCheckEvery = time.Hour

// placeKeptFor is how long the core keeps asking a place no document names any more: a year, so a
// move does not hide a recovery from a core (the root key audit's F6; D393 §7).
const placeKeptFor = 365 * 24 * time.Hour

// maxSignedBytes bounds a signed file and its signature file as fetched.
const maxSignedBytes = 128 << 10

// maxFollowRounds bounds how many times one look asks again for the next statements.
const maxFollowRounds = 64

// Errors enrolling returns.
var (
	ErrSuggestedHasNoGenesis = errors.New("this build does not know the suggested discovery's root " +
		"(internal/core/discovery/suggested.go names no genesis), so the core cannot trust it and is not enrolled")
	ErrNotTheSuggestedRoot = errors.New("the suggested issuer shows a root chain other than the one this build knows")
	ErrNotTheIdentity      = errors.New("the discovery is not the identity given: " +
		"compare it with what its operator shows")
)

// IdentityToConfirmError is enrolling with a discovery the core does not know, given no identity:
// the admin compares the one found with what the discovery's operator shows, and enrols with it
// (the audit's §3.8 step 3).
type IdentityToConfirmError struct {
	Identified
}

func (confirm IdentityToConfirmError) Error() string {
	return "confirm the discovery's identity " + confirm.Identity + " with its operator, and enrol with it"
}

// Identified is a discovery as an issuer shows it: its issuer, its identity, the name its document
// gives it, and whether it is the suggested service.
type Identified struct {
	Issuer, Identity, Name string
	IsSuggested            bool
}

// place is a place the discovery's signed files are served under, and when a document last named it.
type place struct {
	URL string `json:"url"`
	// NamedAtMs is when a document last named it, in Unix milliseconds, as every other time is.
	NamedAtMs int64 `json:"namedAt"`
}

// UnmarshalJSON reads a place as it is written, or as a core before v0.3.3 wrote it, its namedAt an
// RFC 3339 time (the owner, 2026-11-09): the next write keeps it in milliseconds.
func (each *place) UnmarshalJSON(data []byte) error {
	var read struct {
		URL     string          `json:"url"`
		NamedAt json.RawMessage `json:"namedAt"`
	}
	if err := strictjson.Unmarshal(data, &read); err != nil {
		return fmt.Errorf("read a place: %w", err)
	}
	each.URL, each.NamedAtMs = read.URL, 0
	var written string
	switch {
	case len(read.NamedAt) == 0:
	case strictjson.Unmarshal(read.NamedAt, &written) == nil:
		namedAt, err := time.Parse(time.RFC3339Nano, written)
		if err != nil {
			return fmt.Errorf("read a place's namedAt: %w", err)
		}
		each.NamedAtMs = namedAt.UnixMilli()
	default:
		if err := strictjson.Unmarshal(read.NamedAt, &each.NamedAtMs); err != nil {
			return fmt.Errorf("read a place's namedAt: %w", err)
		}
	}
	return nil
}

// floor is the (root version, sequence) a signed object must be above to be taken (the audit's F9).
type floor struct {
	RootVersion int64 `json:"rootVersion"`
	Sequence    int64 `json:"sequence"`
}

// isBelowOrAt is whether (rootVersion, sequence) is not above the floor.
func (held floor) isBelowOrAt(rootVersion, sequence int64) bool {
	return rootVersion < held.RootVersion || rootVersion == held.RootVersion && sequence <= held.Sequence
}

// held is the discovery as the core holds it: its issuer, its root chain, the address document as
// received and as read, the floor a newer one must be above, and every place it is served.
type held struct {
	issuer   string
	chain    trust.Chain
	signed   trust.Signed
	document addressdoc.Document
	floor    floor
	places   []place
}

// livePlaces are the places named within placeKeptFor of now.
func (opened held) livePlaces(now time.Time) []string {
	var live []string
	for _, each := range opened.places {
		if now.Sub(time.UnixMilli(each.NamedAtMs)) < placeKeptFor {
			live = append(live, each.URL)
		}
	}
	return live
}

// named is places with every one of names marked named at now.
func named(places []place, names []string, now time.Time) []place {
	kept := slices.Clone(places)
	for _, name := range names {
		index := slices.IndexFunc(kept, func(each place) bool { return each.URL == name })
		if index < 0 {
			kept = append(kept, place{URL: name, NamedAtMs: now.UnixMilli()})
			continue
		}
		kept[index].NamedAtMs = now.UnixMilli()
	}
	return kept
}

// Identify is the discovery typed's issuer shows: its root chain followed from its genesis, its
// address document opened under it, for the admin to compare its identity before enrolling.
func (service *Service) Identify(ctx context.Context, typed string) (Identified, error) {
	issuer, _, err := splitIssuer(typed)
	if err != nil {
		return Identified{}, err
	}
	opened, err := service.openFirst(ctx, issuer)
	if err != nil {
		return Identified{}, err
	}
	return service.identified(opened), nil
}

// identified is what the admin is shown of opened.
func (service *Service) identified(opened held) Identified {
	return Identified{
		Issuer: opened.issuer, Identity: opened.chain.ID(), Name: opened.document.Name,
		IsSuggested: opened.issuer == service.parts.Suggested.Issuer,
	}
}

// splitIssuer is typed as an issuer and the identity it names after "#", "discovery.example#od1-…".
func splitIssuer(typed string) (issuer, named string, err error) {
	name, named, _ := strings.Cut(strings.TrimSpace(typed), "#")
	issuer, err = addressdoc.Issuer(name)
	return issuer, named, err
}

// openFirst is the discovery at issuer as enrolment takes it (§3.8): its chain from its genesis, the
// suggested one's from the genesis this build holds, followed to its latest at the issuer, and its
// address document there, for exactly that issuer.
func (service *Service) openFirst(ctx context.Context, issuer string) (held, error) {
	suggested := service.parts.Suggested
	isSuggested := suggested.Issuer != "" && suggested.Issuer == issuer
	if isSuggested && suggested.Genesis.Payload == "" {
		return held{}, ErrSuggestedHasNoGenesis
	}
	genesis, isFound, err := service.fetchSigned(ctx, issuer+addressdoc.RootPath(1))
	switch {
	case err != nil:
		return held{}, err
	case !isFound:
		return held{}, fmt.Errorf("%s serves no root chain", issuer)
	case isSuggested && genesis.Payload != suggested.Genesis.Payload:
		return held{}, ErrNotTheSuggestedRoot
	case isSuggested:
		genesis = suggested.Genesis
	}
	chain, err := trust.Genesis(trust.ChainDiscovery, genesis)
	if err != nil {
		return held{}, fmt.Errorf("the discovery's root chain: %w", err)
	}
	service.followChain(ctx, &chain, []string{issuer}, addressdoc.RootPath, addressdoc.RootLatestPath)
	signed, isFound, err := service.fetchSigned(ctx, issuer+addressdoc.DocumentPath)
	if err == nil && !isFound {
		err = fmt.Errorf("%s serves no address document", issuer)
	}
	if err != nil {
		return held{}, err
	}
	document, err := addressdoc.Open(signed, chain, issuer)
	if err != nil {
		return held{}, fmt.Errorf("the discovery's address document: %w", err)
	}
	now := service.parts.Clock.Now()
	return held{
		issuer: issuer, chain: chain, signed: signed, document: document,
		floor:  floor{RootVersion: document.RootVersion, Sequence: document.Sequence},
		places: named(nil, document.Places(), now),
	}, nil
}

// followChain follows chain at places (§3.4): each place's latest statement and the one after the
// chain's version, again while any is taken. It returns what was followed, and whether any place
// answered at all, which makes the core caught up.
func (service *Service) followChain(
	ctx context.Context, chain *trust.Chain, places []string, pathOf func(int64) string, latest string,
) (trust.Followed, bool) {
	var followed trust.Followed
	isReached := false
	for range maxFollowRounds {
		var candidates []trust.Signed
		for _, at := range places {
			for _, path := range []string{latest, pathOf(chain.Version() + 1)} {
				signed, isFound, err := service.fetchSigned(ctx, at+path)
				isReached = isReached || err == nil
				if isFound {
					candidates = append(candidates, signed)
				}
			}
		}
		var round trust.Followed
		*chain, round = chain.Follow(candidates)
		followed.Accepted += round.Accepted
		for _, conflict := range round.Conflicts {
			if !slices.Contains(followed.Conflicts, conflict) {
				followed.Conflicts = append(followed.Conflicts, conflict)
			}
		}
		if round.Accepted == 0 {
			break
		}
	}
	return followed, isReached
}

// fetchSigned is the signed file at url and its signature at url.sig, and whether both are served:
// not-found is none, and no error.
func (service *Service) fetchSigned(ctx context.Context, url string) (trust.Signed, bool, error) {
	payload, isFound, err := service.fetch(ctx, url)
	if err != nil || !isFound {
		return trust.Signed{}, false, err
	}
	signature, isFound, err := service.fetch(ctx, url+addressdoc.SignatureEnd)
	if err != nil || !isFound {
		return trust.Signed{}, false, err
	}
	return trust.Signed{Payload: string(payload), Signature: string(signature)}, true, nil
}

// fetch is the body served at url, and whether it is: not-found is none.
func (service *Service) fetch(ctx context.Context, url string) ([]byte, bool, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, false, fmt.Errorf("ask for %s: %w", url, err)
	}
	response, err := service.parts.HTTP.Do(request)
	if err != nil {
		return nil, false, fmt.Errorf("reach %s: %w", url, err)
	}
	defer func() { _ = response.Body.Close() }() //nolint:errcheck // read, and closed
	switch {
	case response.StatusCode == http.StatusNotFound:
		return nil, false, nil
	case response.StatusCode != http.StatusOK:
		return nil, false, fmt.Errorf("%s answered %s", url, response.Status)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxSignedBytes))
	if err != nil {
		return nil, false, fmt.Errorf("read %s: %w", url, err)
	}
	return body, true, nil
}

// checkDocument looks at every place the discovery is served, hourly (§3.4, §3.5): it follows the
// root chain first, telling the admins of any conflict; then it takes the newest address document
// that holds under the chain and is above the floor, from any place. A document of the floor's own
// (root version, sequence) with other bytes is equivocation: refused, and said. A newer document
// moves the core's addresses, its API included, which it then calls under the same identity.
func (service *Service) checkDocument(ctx context.Context) {
	now := service.parts.Clock.Now()
	service.mutex.Lock()
	isDue := service.documentCheckedAt.IsZero() || now.Sub(service.documentCheckedAt) >= documentCheckEvery
	opened := service.held
	opened.places = slices.Clone(opened.places)
	service.mutex.Unlock()
	if !isDue || opened.issuer == "" {
		return
	}
	places := opened.livePlaces(now)
	followed, isReached := service.followChain(ctx, &opened.chain, places, addressdoc.RootPath,
		addressdoc.RootLatestPath)
	var problems []string
	if len(followed.Conflicts) > 0 {
		problems = append(problems, fmt.Sprintf("two different root statements of version %v are signed: refused; "+
			"the discovery's owner must resolve it", followed.Conflicts))
		service.parts.Logger.ErrorContext(ctx, "discovery: two root statements of one version are signed",
			"versions", followed.Conflicts)
	}
	if followed.Accepted > 0 {
		// A new root lifts the floor to its version, and a document its set no longer signs is dropped.
		opened.floor = higherFloor(opened.floor, floor{RootVersion: opened.chain.Version()})
		if _, err := addressdoc.Open(opened.signed, opened.chain, opened.issuer); err != nil {
			problems = append(problems, "the address document held is no longer signed by the discovery's keys")
		}
	}
	newest, documentProblems := service.newestDocument(ctx, opened, places)
	problems = append(problems, documentProblems...)
	isMoved := newest != nil && isReached
	if isMoved {
		opened.signed, opened.document = newest.signed, newest.document
		opened.floor = floor{RootVersion: newest.document.RootVersion, Sequence: newest.document.Sequence}
		opened.places = named(opened.places, newest.document.Places(), now)
	}
	service.adoptHeld(ctx, opened, now, isReached, isMoved || followed.Accepted > 0, problems)
}

// higherFloor is the higher of two floors.
func higherFloor(one, other floor) floor {
	if one.isBelowOrAt(other.RootVersion, other.Sequence) {
		return other
	}
	return one
}

// openedDocument is a document as received and as read.
type openedDocument struct {
	signed   trust.Signed
	document addressdoc.Document
}

// newestDocument is the newest document above opened's floor that holds, from any of places, or nil,
// and what was wrong with those that did not.
func (service *Service) newestDocument(ctx context.Context, opened held, places []string) (*openedDocument, []string) {
	var newest *openedDocument
	var problems []string
	for _, place := range places {
		signed, isFound, err := service.fetchSigned(ctx, place+addressdoc.DocumentPath)
		if err != nil || !isFound {
			continue
		}
		document, err := addressdoc.Open(signed, opened.chain, opened.issuer)
		if err != nil {
			problems = append(problems, place+": "+err.Error())
			continue
		}
		atFloor := document.RootVersion == opened.floor.RootVersion && document.Sequence == opened.floor.Sequence
		switch {
		case atFloor && signed.Payload != opened.signed.Payload:
			problems = append(problems, fmt.Sprintf("%s serves another address document of sequence %d: refused",
				place, document.Sequence))
			service.parts.Logger.ErrorContext(ctx, "discovery: two address documents of one sequence are signed",
				"place", place, "sequence", document.Sequence)
		case opened.floor.isBelowOrAt(document.RootVersion, document.Sequence):
		case newest == nil || document.IsNewerThan(newest.document):
			newest = &openedDocument{signed: signed, document: document}
		}
	}
	return newest, problems
}

// adoptHeld holds opened, the look's outcome, and keeps it when it changed.
func (service *Service) adoptHeld(ctx context.Context, opened held, now time.Time, isReached, isChanged bool,
	problems []string,
) {
	service.mutex.Lock()
	service.documentCheckedAt = now
	service.isCaughtUp = service.isCaughtUp || isReached
	service.status.DocumentProblem = strings.Join(problems, "; ")
	service.held = opened
	if opened.document.CoreAPI != service.status.BaseURL {
		service.status.BaseURL, service.token = opened.document.CoreAPI, ""
	}
	service.mutex.Unlock()
	if !isChanged {
		return
	}
	service.parts.Logger.InfoContext(ctx, "discovery: a newer root or address document is held",
		"rootVersion", opened.chain.Version(), "sequence", opened.document.Sequence, "coreApi", opened.document.CoreAPI)
	if err := service.keepTrust(); err != nil {
		service.parts.Logger.WarnContext(ctx, "discovery: could not keep the discovery's trust state", "err", err)
	}
}
