// SPDX-License-Identifier: AGPL-3.0-or-later

package addressdoc

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/kstenerud/orpheum/discovery/trust"
)

// The paths a discovery's signed files are served at, under its issuer and under each of its
// locations, each beside its signature at the same path and ".sig" (the root key audit's §3.1): the
// address document, and its root chain's statements by version and the latest.
const (
	DocumentPath   = "/.well-known/orpheum-discovery"
	RootPathPrefix = "/.well-known/orpheum/root/"
	RootLatestPath = RootPathPrefix + "latest"
	SignatureEnd   = ".sig"
)

// RootPath is where the chain's statement of version is served.
func RootPath(version int64) string {
	return RootPathPrefix + strconv.FormatInt(version, 10)
}

// CorePlace is the place a core's id takes in a document's ClientOrigins (D308).
const CorePlace = "{core}"

// Document is a discovery's address document (the root key audit's §3.1; D391 §3): the discovery's
// id, the root version it was signed under and its sequence, which only rise; when it was issued and
// until when it is current; the discovery's name and issuer, the typed origin; where it is served;
// and what a core enrolled with it may trust it to name.
type Document struct {
	ID          string
	RootVersion int64
	Sequence    int64
	Issued      time.Time
	Expires     time.Time
	Name        string
	Issuer      string
	// Locations are where this document and the chain are served, the issuer among them.
	Locations []string
	// CoreAPI is the base URL a core calls the discovery's protocol at; ClientAPI the client's.
	CoreAPI   string
	ClientAPI string
	// LoaderOrigin is where the loader is served (D308); ClientOrigins the pattern of each core's own
	// client origin, CorePlace standing for its id, or one origin, for a discovery of one household.
	LoaderOrigin  string
	ClientOrigins string
	// AccountOrigin is where a household's account is kept, at a discovery that has accounts;
	// Homepage says what the discovery is, for a stranger; Support is its address for help.
	AccountOrigin, Homepage, Support string
}

// The fields of the document whose names its check also says.
const (
	fieldCoreAPI      = "core-api"
	fieldClientAPI    = "client-api"
	fieldLoaderOrigin = "loader-origin"
)

// documentSchema is the address document's lines, format 1.
func documentSchema() trust.Schema {
	return trust.Schema{Type: "address", Format: 1, Fields: []trust.Field{
		{Name: "id", IsRequired: true},
		{Name: "root-version", IsRequired: true},
		{Name: "sequence", IsRequired: true},
		{Name: "issued", IsRequired: true},
		{Name: "expires", IsRequired: true},
		{Name: "name", IsRequired: true},
		{Name: "issuer", IsRequired: true},
		{Name: "location", IsRepeated: true},
		{Name: fieldCoreAPI, IsRequired: true},
		{Name: fieldClientAPI, IsRequired: true},
		{Name: fieldLoaderOrigin, IsRequired: true},
		{Name: "client-origins", IsRequired: true},
		{Name: "account-origin"},
		{Name: "homepage"},
		{Name: "support"},
	}}
}

// Errors a document's check returns.
var (
	ErrNotAnIssuer   = errors.New("a discovery is named by its origin: https://name, or http:// only on this machine")
	ErrWrongIssuer   = errors.New("the document is not the asked issuer's")
	ErrWrongID       = errors.New("the document is another discovery's")
	ErrAheadOfChain  = errors.New("the document names a root version the chain held has not reached: catch up first")
	ErrNotAnOrigin   = errors.New("not an https origin")
	ErrNotAnAddress  = errors.New("not an https URL")
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

// Parse reads a document's payload, checking only its form.
func Parse(payload []byte) (Document, error) {
	lines, err := documentSchema().Parse(payload)
	if err != nil {
		return Document{}, err
	}
	document := Document{
		ID: lines.One("id"), Name: lines.One("name"), Issuer: lines.One("issuer"), Locations: lines["location"],
		CoreAPI: lines.One(fieldCoreAPI), ClientAPI: lines.One(fieldClientAPI),
		LoaderOrigin: lines.One(fieldLoaderOrigin), ClientOrigins: lines.One("client-origins"),
		AccountOrigin: lines.One("account-origin"), Homepage: lines.One("homepage"), Support: lines.One("support"),
	}
	if document.RootVersion, err = trust.ParseCount(lines.One("root-version")); err != nil {
		return Document{}, err
	}
	if document.Sequence, err = trust.ParseCount(lines.One("sequence")); err != nil {
		return Document{}, err
	}
	if document.Issued, err = trust.ParseTime(lines.One("issued")); err != nil {
		return Document{}, err
	}
	if document.Expires, err = trust.ParseTime(lines.One("expires")); err != nil {
		return Document{}, err
	}
	return document, nil
}

// Payload is the document written as its lines, which a key then signs.
func (document Document) Payload() ([]byte, error) {
	lines := trust.Lines{
		"id": {document.ID}, "root-version": {strconv.FormatInt(document.RootVersion, 10)},
		"sequence": {strconv.FormatInt(document.Sequence, 10)}, "issued": {trust.TimeText(document.Issued)},
		"expires": {trust.TimeText(document.Expires)}, "name": {document.Name}, "issuer": {document.Issuer},
		"location": document.Locations, fieldCoreAPI: {document.CoreAPI}, fieldClientAPI: {document.ClientAPI},
		fieldLoaderOrigin: {document.LoaderOrigin}, "client-origins": {document.ClientOrigins},
	}
	for key, value := range map[string]string{
		"account-origin": document.AccountOrigin, "homepage": document.Homepage, "support": document.Support,
	} {
		if value != "" {
			lines[key] = []string{value}
		}
	}
	return documentSchema().Write(lines)
}

// Open is the document signed holds once it is checked (§3.5): a signature of chain's current set's
// in the address namespace, its id the chain's, its root version one the chain has reached, its
// issuer exactly issuer (RFC 8414 §3.3), and its addresses ones a core takes. The core keeps the
// floors; a newer document is one with a higher (root version, sequence).
func Open(signed trust.Signed, chain trust.Chain, issuer string) (Document, error) {
	if err := chain.Verify(trust.NamespaceDiscoveryAddress, signed); err != nil {
		return Document{}, err
	}
	document, err := Parse([]byte(signed.Payload))
	if err != nil {
		return Document{}, err
	}
	switch {
	case document.ID != chain.ID():
		return Document{}, fmt.Errorf("%w: %s, not %s", ErrWrongID, document.ID, chain.ID())
	case document.RootVersion > chain.Version():
		return Document{}, fmt.Errorf("%w: %d, held %d", ErrAheadOfChain, document.RootVersion, chain.Version())
	}
	return document, document.Check(issuer)
}

// Check is whether the document is issuer's and every address in it is one a core takes: its issuer
// exactly issuer, and every origin and URL https, or http on this machine.
func (document Document) Check(issuer string) error {
	if document.Issuer != issuer {
		return fmt.Errorf("%w: %q, asked %q", ErrWrongIssuer, document.Issuer, issuer)
	}
	addresses := map[string]string{fieldCoreAPI: document.CoreAPI, fieldClientAPI: document.ClientAPI}
	if document.Homepage != "" {
		addresses["homepage"] = document.Homepage
	}
	for index, location := range document.Locations {
		addresses[fmt.Sprintf("location %d", index)] = location
	}
	for name, address := range addresses {
		if !isAddress(address) {
			return fmt.Errorf("%s %q: %w", name, address, ErrNotAnAddress)
		}
	}
	origins := map[string]string{fieldLoaderOrigin: document.LoaderOrigin}
	if document.AccountOrigin != "" {
		origins["account-origin"] = document.AccountOrigin
	}
	for name, origin := range origins {
		if checked, err := originOf(origin); err != nil || checked != origin {
			return fmt.Errorf("%s %q: %w", name, origin, ErrNotAnOrigin)
		}
	}
	return checkPattern(document.ClientOrigins)
}

// IsNewerThan is whether the document comes after other: a higher root version, or the same and a
// higher sequence (the audit's F9).
func (document Document) IsNewerThan(other Document) bool {
	if document.RootVersion != other.RootVersion {
		return document.RootVersion > other.RootVersion
	}
	return document.Sequence > other.Sequence
}

// ClientOrigin is the client origin of the core coreID: the pattern's, its id in CorePlace.
func (document Document) ClientOrigin(coreID string) string {
	return strings.ReplaceAll(document.ClientOrigins, CorePlace, coreID)
}

// Places are the bases the discovery's signed files are served under, each with no closing slash:
// its issuer, then each location not the issuer, in turn.
func (document Document) Places() []string {
	places := []string{document.Issuer}
	for _, location := range document.Locations {
		place := strings.TrimSuffix(location, "/")
		if place != document.Issuer {
			places = append(places, place)
		}
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
