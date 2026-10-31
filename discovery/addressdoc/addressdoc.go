// SPDX-License-Identifier: AGPL-3.0-or-later

package addressdoc

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/kstenerud/orpheum/discovery/trust"
)

// WellKnownPath is where a discovery serves its address document, at its issuer and at each of its
// locations (RFC 8615).
const WellKnownPath = "/.well-known/orpheum-discovery"

// CorePlace is the place a core's id takes in a document's ClientOrigins (D308).
const CorePlace = "{core}"

// Document is a discovery's address document, the payload its signature is over: what a core enrolled
// with the issuer may trust its discovery to name (D391 §3).
type Document struct {
	// Issuer is the discovery's typed origin, its identity's name, "https://discovery.example".
	Issuer string `json:"issuer"`
	// CoreAPI is the base URL a core calls the discovery's protocol at; ClientAPI the client's.
	CoreAPI   string `json:"coreApi"`
	ClientAPI string `json:"clientApi"`
	// LoaderOrigin is where the loader is served (D308); ClientOrigins the pattern of each core's own
	// client origin, CorePlace standing for its id, or one origin, for a discovery of one household.
	LoaderOrigin  string `json:"loaderOrigin"`
	ClientOrigins string `json:"clientOrigins"`
	// AccountOrigin is where a household's account is kept, at a discovery that has accounts.
	AccountOrigin string `json:"accountOrigin,omitempty"`
	// Homepage says what the discovery is, for a stranger; Support is the discovery's own address
	// for help, "mailto:" not included.
	Homepage string `json:"homepage,omitempty"`
	Support  string `json:"support,omitempty"`
	// Locations are the further places this very document is served, each a URL that WellKnownPath
	// follows, so a core finds a newer one when the issuer's name is lost.
	Locations []string `json:"locations,omitempty"`
	// IssuedAt is when the document was made, Unix milliseconds: a newer one replaces an older.
	IssuedAt int64 `json:"issuedAt"`
}

// Served is what WellKnownPath answers: the signed document, and the root key set's statements
// (trust.RootSet) from the first, so a core several versions behind catches up from any location.
type Served struct {
	Document trust.Signed   `json:"document"`
	RootSets []trust.Signed `json:"rootSets"`
}

// Errors a document's check returns.
var (
	ErrNotAnIssuer   = errors.New("a discovery is named by its origin: https://name, or http:// only on this machine")
	ErrWrongIssuer   = errors.New("the document is not the asked issuer's")
	ErrNotADocument  = errors.New("not a discovery's address document")
	ErrNotAnOrigin   = errors.New("not an https origin")
	ErrNotAnAddress  = errors.New("not an https URL")
	ErrMissingField  = errors.New("the document lacks a field every document has")
	ErrNoCoreInPlace = errors.New("the client origins' pattern puts the core's id in no label")
)

// Issuer is typed as a discovery's issuer: "discovery.example" is https://discovery.example; a URL is
// taken only when it is an origin, nothing after its host and port.
func Issuer(typed string) (string, error) {
	text := strings.TrimSpace(typed)
	if !strings.Contains(text, "://") {
		text = "https://" + text
	}
	origin, err := originOf(strings.TrimSuffix(text, "/"))
	if err != nil {
		return "", fmt.Errorf("%q: %w", typed, ErrNotAnIssuer)
	}
	return origin, nil
}

// Open is the document served holds once it is checked: its root key sets taken from pinned on
// (trust.Pinned.Advance), its signature by that set at now, and its fields as Check says, for issuer.
// The set reached is returned with it, the core's to keep.
func Open(served Served, pinned trust.Pinned, issuer string, now time.Time) (Document, trust.Pinned, error) {
	advanced, err := pinned.Advance(served.RootSets)
	if err != nil {
		return Document{}, pinned, err
	}
	payload, err := advanced.OpenDocument(served.Document, issuer, now)
	if err != nil {
		return Document{}, pinned, err
	}
	var document Document
	if err = json.Unmarshal(payload, &document); err != nil {
		return Document{}, pinned, fmt.Errorf("%w: %w", ErrNotADocument, err)
	}
	if checkErr := document.Check(issuer); checkErr != nil {
		return Document{}, pinned, checkErr
	}
	return document, advanced, nil
}

// Check is whether the document is issuer's and every address in it is one a core takes: its issuer
// exactly issuer (RFC 8414 §3.3), and every origin and URL https, or http on this machine.
func (document Document) Check(issuer string) error {
	if document.Issuer != issuer {
		return fmt.Errorf("%w: %q, asked %q", ErrWrongIssuer, document.Issuer, issuer)
	}
	if document.CoreAPI == "" || document.ClientAPI == "" || document.LoaderOrigin == "" ||
		document.ClientOrigins == "" || document.IssuedAt == 0 {
		return ErrMissingField
	}
	addresses := map[string]string{"coreApi": document.CoreAPI, "clientApi": document.ClientAPI}
	if document.Homepage != "" {
		addresses["homepage"] = document.Homepage
	}
	for index, location := range document.Locations {
		addresses[fmt.Sprintf("locations[%d]", index)] = location
	}
	for name, address := range addresses {
		if !isAddress(address) {
			return fmt.Errorf("%s %q: %w", name, address, ErrNotAnAddress)
		}
	}
	origins := map[string]string{"loaderOrigin": document.LoaderOrigin}
	if document.AccountOrigin != "" {
		origins["accountOrigin"] = document.AccountOrigin
	}
	for name, origin := range origins {
		if checked, err := originOf(origin); err != nil || checked != origin {
			return fmt.Errorf("%s %q: %w", name, origin, ErrNotAnOrigin)
		}
	}
	return checkPattern(document.ClientOrigins)
}

// ClientOrigin is the client origin of the core coreID: the pattern's, its id in CorePlace.
func (document Document) ClientOrigin(coreID string) string {
	return strings.ReplaceAll(document.ClientOrigins, CorePlace, coreID)
}

// URLs are where the document is served: at its issuer, then at each location, in turn.
func (document Document) URLs() []string {
	urls := make([]string, 0, len(document.Locations)+1)
	for _, place := range document.Places() {
		urls = append(urls, place+WellKnownPath)
	}
	return urls
}

// Places are the bases the discovery's signed files are served under, each with no closing slash:
// its issuer, then each location, in turn. The document is at each one's WellKnownPath, the release
// record at its own (discovery/release).
func (document Document) Places() []string {
	places := []string{document.Issuer}
	for _, location := range document.Locations {
		places = append(places, strings.TrimSuffix(location, "/"))
	}
	return places
}

// checkPattern is whether pattern is an origin once a core's id is in its place, which the pattern
// puts in one whole label of its host, or has no place, for a discovery of one household.
func checkPattern(pattern string) error {
	if strings.Count(pattern, CorePlace) > 1 {
		return fmt.Errorf("clientOrigins %q: %w", pattern, ErrNoCoreInPlace)
	}
	filled := strings.ReplaceAll(pattern, CorePlace, "k7xq2")
	if checked, err := originOf(filled); err != nil || checked != filled {
		return fmt.Errorf("clientOrigins %q: %w", pattern, ErrNotAnOrigin)
	}
	if strings.Contains(pattern, CorePlace) {
		host := strings.TrimPrefix(strings.TrimPrefix(pattern, "https://"), "http://")
		labels := strings.Split(strings.Split(host, ":")[0], ".")
		isWhole := false
		for _, label := range labels {
			isWhole = isWhole || label == CorePlace
		}
		if !isWhole {
			return fmt.Errorf("clientOrigins %q: %w", pattern, ErrNoCoreInPlace)
		}
	}
	return nil
}

// originOf is address as an origin, scheme://host[:port] in lower case, when it is one: https, or
// http on this machine, with nothing after its host and port.
func originOf(address string) (string, error) {
	parsed, err := url.Parse(address)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" ||
		parsed.Fragment != "" || parsed.Opaque != "" || strings.HasSuffix(address, "?") ||
		strings.HasSuffix(address, "#") {
		return "", ErrNotAnOrigin
	}
	if !isSecure(parsed) {
		return "", ErrNotAnOrigin
	}
	return parsed.Scheme + "://" + strings.ToLower(parsed.Host), nil
}

// isAddress is whether address is an absolute https URL, or http on this machine, with no
// credentials, query or fragment.
func isAddress(address string) bool {
	parsed, err := url.Parse(address)
	return err == nil && parsed.Host != "" && parsed.User == nil && parsed.RawQuery == "" && parsed.Fragment == "" &&
		isSecure(parsed)
}

// isSecure is whether parsed crosses no network in the clear: https, or http to this machine's
// loopback, as a development discovery is (RFC 6761 §6.3 for localhost's names).
func isSecure(parsed *url.URL) bool {
	switch parsed.Scheme {
	case "https":
		return true
	case "http":
		name := parsed.Hostname()
		address := net.ParseIP(name)
		return address != nil && address.IsLoopback() || name == "localhost" || strings.HasSuffix(name, ".localhost")
	}
	return false
}
