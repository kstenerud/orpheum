// SPDX-License-Identifier: GPL-3.0-or-later

package certificatetest

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/acme"
)

// The states an order and its authorization pass through.
const (
	statusPending = "pending"
	statusReady   = "ready"
	statusValid   = "valid"
)

// challengeToken is the one challenge's token.
const challengeToken = "the-token"

// TXTRecords is the DNS a challenge is checked against: the TXT record of a name, empty for none;
// cloudflaretest.Fake, or a test's own.
type TXTRecords interface {
	TXT(name string) string
}

// CA is an ACME CA for tests (RFC 8555), as small as golang.org/x/crypto/acme's client needs: one
// account, an order of one name at a time with a dns-01 challenge it checks against the fake DNS,
// and certificates signed by its own root, valid for validFor from now.
type CA struct {
	server   *httptest.Server
	dns      TXTRecords
	validFor time.Duration
	now      time.Time
	rootKey  *ecdsa.PrivateKey
	root     *x509.Certificate

	mutex sync.Mutex
	// thumbprint is the account key's, from its registration, which the challenge's record names.
	thumbprint string
	domain     string
	status     string
	leaf       []byte
	// issuedCount is how many certificates it issued.
	issuedCount int
}

// NewCA serves a fake CA checking challenges against dns, issuing certificates valid from now, until
// the test ends.
func NewCA(t *testing.T, dns TXTRecords, now time.Time, validFor time.Duration) *CA {
	t.Helper()
	rootKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "fake root"}, NotBefore: now.Add(-time.Hour),
		NotAfter: now.Add(10 * 365 * 24 * time.Hour), IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &rootKey.PublicKey, rootKey)
	if err != nil {
		t.Fatal(err)
	}
	root, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	authority := &CA{dns: dns, validFor: validFor, now: now, rootKey: rootKey, root: root}
	authority.server = httptest.NewServer(http.HandlerFunc(authority.serve))
	t.Cleanup(authority.server.Close)
	return authority
}

// Directory is the fake's ACME directory.
func (authority *CA) Directory() string { return authority.server.URL + "/directory" }

// Roots are the certificates a client trusts to accept the fake's: its root.
func (authority *CA) Roots() *x509.CertPool {
	pool := x509.NewCertPool()
	pool.AddCert(authority.root)
	return pool
}

// Issued is how many certificates it issued.
func (authority *CA) Issued() int {
	authority.mutex.Lock()
	defer authority.mutex.Unlock()
	return authority.issuedCount
}

// signedRequest is a request's flattened JWS, whose signature the fake does not check.
type signedRequest struct {
	Protected string `json:"protected"`
	Payload   string `json:"payload"`
}

// The resources the fake answers, as RFC 8555 shapes them.
type (
	directoryBody struct {
		NewNonce   string `json:"newNonce"`
		NewAccount string `json:"newAccount"`
		NewOrder   string `json:"newOrder"`
	}
	identifierBody struct {
		Type  string `json:"type"`
		Value string `json:"value"`
	}
	orderBody struct {
		Status         string           `json:"status"`
		Identifiers    []identifierBody `json:"identifiers"`
		Authorizations []string         `json:"authorizations"`
		Finalize       string           `json:"finalize"`
		Certificate    string           `json:"certificate,omitempty"`
	}
	challengeBody struct {
		Type   string `json:"type"`
		URL    string `json:"url,omitempty"`
		Token  string `json:"token,omitempty"`
		Status string `json:"status,omitempty"`
	}
	authorizationBody struct {
		Status     string          `json:"status"`
		Identifier identifierBody  `json:"identifier"`
		Challenges []challengeBody `json:"challenges"`
		Wildcard   bool            `json:"wildcard,omitempty"`
	}
	statusBody struct {
		Status string `json:"status"`
	}
	problemBody struct {
		Type   string `json:"type"`
		Detail string `json:"detail"`
	}
)

func (authority *CA) serve(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Replay-Nonce", "nonce")
	var signed signedRequest
	if request.Method == http.MethodPost {
		raw, err := io.ReadAll(request.Body)
		if err != nil || json.Unmarshal(raw, &signed) != nil {
			http.Error(writer, "not a JWS", http.StatusBadRequest)
			return
		}
	}
	payload, err := base64.RawURLEncoding.DecodeString(signed.Payload)
	if err != nil {
		http.Error(writer, "not base64url", http.StatusBadRequest)
		return
	}
	authority.mutex.Lock()
	defer authority.mutex.Unlock()
	switch request.URL.Path {
	case "/directory":
		writeJSON(writer, http.StatusOK, directoryBody{
			NewNonce: authority.url("/nonce"), NewAccount: authority.url("/account"), NewOrder: authority.url("/order"),
		})
	case "/nonce":
		writer.WriteHeader(http.StatusOK)
	case "/account":
		authority.thumbprint = thumbprintOf(signed.Protected)
		writer.Header().Set("Location", authority.url("/account/1"))
		writeJSON(writer, http.StatusCreated, statusBody{Status: statusValid})
	case "/order":
		authority.startOrder(writer, payload)
	case "/order/1":
		writeJSON(writer, http.StatusOK, authority.order())
	case "/authz/1":
		writeJSON(writer, http.StatusOK, authority.authorization())
	case "/challenge/1":
		authority.check(writer)
	case "/finalize/1":
		authority.issue(writer, payload)
	case "/certificate/1":
		writer.Header().Set("Content-Type", "application/pem-certificate-chain")
		writePEM(writer, authority.leaf)
		writePEM(writer, authority.root.Raw)
	default:
		http.Error(writer, "no such resource", http.StatusNotFound)
	}
}

// url is the fake's address of path.
func (authority *CA) url(path string) string { return authority.server.URL + path }

// startOrder begins the one order, for the name its payload names.
func (authority *CA) startOrder(writer http.ResponseWriter, payload []byte) {
	var order struct {
		Identifiers []identifierBody `json:"identifiers"`
	}
	if json.Unmarshal(payload, &order) != nil || len(order.Identifiers) != 1 {
		http.Error(writer, "one name per order", http.StatusBadRequest)
		return
	}
	authority.domain, authority.status, authority.leaf = order.Identifiers[0].Value, statusPending, nil
	writer.Header().Set("Location", authority.url("/order/1"))
	writeJSON(writer, http.StatusCreated, authority.order())
}

// order is the one order as the fake says it.
func (authority *CA) order() orderBody {
	order := orderBody{
		Status: authority.status, Identifiers: []identifierBody{{Type: "dns", Value: authority.domain}},
		Authorizations: []string{authority.url("/authz/1")}, Finalize: authority.url("/finalize/1"),
	}
	if authority.status == statusValid {
		order.Certificate = authority.url("/certificate/1")
	}
	return order
}

// authorization is the order's one authorization, valid once its challenge is.
func (authority *CA) authorization() authorizationBody {
	status := statusValid
	if authority.status == statusPending {
		status = statusPending
	}
	// A wildcard's authorization names the name under it, and says it is for a wildcard (RFC 8555
	// §7.1.3), as Let's Encrypt's does.
	return authorizationBody{
		Status: status, Identifier: identifierBody{Type: "dns", Value: authority.provenName()},
		Challenges: []challengeBody{{Type: "dns-01", URL: authority.url("/challenge/1"), Token: challengeToken}},
		Wildcard:   authority.provenName() != authority.domain,
	}
}

// provenName is the name the order's challenge proves: the name itself, or the one a wildcard
// stands under.
func (authority *CA) provenName() string {
	return strings.TrimPrefix(authority.domain, "*.")
}

// check answers the challenge: valid when the fake DNS holds the record its token and the
// account's key make, as RFC 8555 §8.4 says; else it refuses.
func (authority *CA) check(writer http.ResponseWriter) {
	digest := sha256.Sum256([]byte(challengeToken + "." + authority.thumbprint))
	if authority.dns.TXT("_acme-challenge."+authority.provenName()) != base64.RawURLEncoding.EncodeToString(digest[:]) {
		writer.Header().Set("Content-Type", "application/problem+json")
		writeJSON(writer, http.StatusForbidden, problemBody{
			Type: "urn:ietf:params:acme:error:unauthorized", Detail: "the TXT record does not match",
		})
		return
	}
	authority.status = statusReady
	writeJSON(writer, http.StatusOK, challengeBody{Type: "dns-01", Status: statusValid})
}

// issue signs the order's certificate request.
func (authority *CA) issue(writer http.ResponseWriter, payload []byte) {
	var finalize struct {
		CSR string `json:"csr"`
	}
	der, err := []byte(nil), json.Unmarshal(payload, &finalize)
	if err == nil {
		der, err = base64.RawURLEncoding.DecodeString(finalize.CSR)
	}
	var request *x509.CertificateRequest
	if err == nil {
		request, err = x509.ParseCertificateRequest(der)
	}
	if err != nil || authority.status != statusReady {
		http.Error(writer, "no certificate for this", http.StatusForbidden)
		return
	}
	authority.issuedCount++
	template := &x509.Certificate{
		SerialNumber: big.NewInt(int64(authority.issuedCount) + 1), DNSNames: request.DNSNames,
		NotBefore: authority.now.Add(-time.Hour), NotAfter: authority.now.Add(authority.validFor),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	authority.leaf, err = x509.CreateCertificate(rand.Reader, template, authority.root, request.PublicKey,
		authority.rootKey)
	if err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	authority.status = statusValid
	writeJSON(writer, http.StatusOK, authority.order())
}

// thumbprintOf is the RFC 7638 thumbprint of the key in a JWS's protected header, or "" for one it
// cannot read, which no record then matches.
func thumbprintOf(protected string) string {
	header, err := base64.RawURLEncoding.DecodeString(protected)
	var parsed struct {
		JWK struct {
			X string `json:"x"`
			Y string `json:"y"`
		} `json:"jwk"`
	}
	if err != nil || json.Unmarshal(header, &parsed) != nil {
		return ""
	}
	x, xErr := base64.RawURLEncoding.DecodeString(parsed.JWK.X)
	y, yErr := base64.RawURLEncoding.DecodeString(parsed.JWK.Y)
	if xErr != nil || yErr != nil {
		return ""
	}
	key, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), append(append([]byte{4}, x...), y...))
	if err != nil {
		return ""
	}
	thumbprint, err := acme.JWKThumbprint(key)
	if err != nil {
		return ""
	}
	return thumbprint
}

// writeJSON answers with value as JSON, the Content-Type already set when it is not application/json.
func writeJSON[T directoryBody | orderBody | authorizationBody | statusBody | challengeBody | problemBody](
	writer http.ResponseWriter, status int, value T,
) {
	if writer.Header().Get("Content-Type") == "" {
		writer.Header().Set("Content-Type", "application/json")
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	writer.WriteHeader(status)
	_, _ = writer.Write(encoded) //nolint:errcheck // the client went; a test's fake
}

// writePEM writes one certificate as PEM.
func writePEM(writer io.Writer, der []byte) {
	encoded := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	_, _ = writer.Write(encoded) //nolint:errcheck // the client went; a test's fake
}
