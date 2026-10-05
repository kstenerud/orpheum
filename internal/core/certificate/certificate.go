// SPDX-License-Identifier: GPL-3.0-or-later

package certificate

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/acme"

	"github.com/kstenerud/orpheum/internal/core/instancedir"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/providers/cloudflare"
	"github.com/kstenerud/orpheum/internal/platform/clock"
)

// LetsEncrypt is Let's Encrypt's ACME directory, where the certificate comes from.
const LetsEncrypt = "https://acme-v02.api.letsencrypt.org/directory"

// The files the certificate is kept in, in the instance's persistent directory: the certificate is
// costly to lose, since a CA limits how often it issues one for a name, and is backed up.
const (
	certificateFile = "tls-certificate.pem"
	keyFile         = "tls-key.pem"
	accountKeyFile  = "acme-account-key.pem"
)

// renewBefore is how long before it expires a certificate is renewed: a third of Let's Encrypt's
// ninety days, as it advises, so a renewal that fails has a month of daily tries before it matters.
const renewBefore = 30 * 24 * time.Hour

// ErrNoCertificate is a server with no certificate yet.
var ErrNoCertificate = errors.New("the server has no certificate for its name yet")

// DNS sets and removes a DNS-01 challenge's TXT record: cloudflare.Client.
type DNS interface {
	SetTXT(ctx context.Context, name, value string) (cloudflare.TXTRecord, error)
	RemoveTXT(ctx context.Context, record cloudflare.TXTRecord) error
}

// Files keeps the certificate in the persistent directory: instancedir.Directories.
type Files interface {
	ReadPersistentFile(name string) ([]byte, error)
	WritePersistentFile(name string, contents []byte) error
}

// Queue is where a renewal is asked for: jobs.Queue.
type Queue interface {
	Enqueue(ctx context.Context, request jobs.Request) (bool, error)
}

// Parts are what the service is built from.
type Parts struct {
	// Domain is the name the certificate is for, media.orpheum-app.com (D243 §6), and Contact the
	// ACME account's e-mail address.
	Domain, Contact string
	// DirectoryURL is the ACME directory: LetsEncrypt, or a test's fake.
	DirectoryURL string
	// HTTP is the client every ACME request goes through, from internal/platform/outbound.
	HTTP  *http.Client
	DNS   DNS
	Files Files
	Queue Queue
	Clock clock.Clock
	// PropagationWait is how long the TXT record is given to reach the name's servers before the
	// CA is asked to look: Cloudflare's own servers answer within seconds.
	PropagationWait time.Duration
	Logger          *slog.Logger
}

// Service obtains the server's certificate by ACME DNS-01 and renews it, and hands the HTTPS
// listener the current one (D243 §6).
type Service struct {
	parts Parts
	// current is the certificate served now, nil before the first is obtained.
	current atomic.Pointer[tls.Certificate]
}

// New returns the service, with the certificate kept from before when there is one.
func New(parts Parts) (*Service, error) {
	service := &Service{parts: parts}
	certificatePEM, certificateErr := parts.Files.ReadPersistentFile(certificateFile)
	keyPEM, keyErr := parts.Files.ReadPersistentFile(keyFile)
	if errors.Is(certificateErr, instancedir.ErrNotFound) || errors.Is(keyErr, instancedir.ErrNotFound) {
		return service, nil
	}
	if err := errors.Join(certificateErr, keyErr); err != nil {
		return nil, fmt.Errorf("read the kept certificate: %w", err)
	}
	pair, err := tls.X509KeyPair(certificatePEM, keyPEM)
	if err != nil {
		parts.Logger.Warn("the kept certificate cannot be read; a new one is obtained", "err", err)
		return service, nil
	}
	service.current.Store(&pair)
	return service, nil
}

// GetCertificate is the certificate the HTTPS listener serves: tls.Config's GetCertificate.
func (service *Service) GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	current := service.current.Load()
	if current == nil {
		return nil, ErrNoCertificate
	}
	return current, nil
}

// ExpiresAt is when the current certificate expires; false when there is none.
func (service *Service) ExpiresAt() (time.Time, bool) {
	current := service.current.Load()
	if current == nil || current.Leaf == nil {
		return time.Time{}, false
	}
	return current.Leaf.NotAfter, true
}

// QueueRenewal asks for the certificate when there is none, or when it expires within renewBefore,
// and says how many it asked for. A sweep calls it.
func (service *Service) QueueRenewal(ctx context.Context) (int, error) {
	if expiresAt, isAny := service.ExpiresAt(); isAny && expiresAt.Sub(service.parts.Clock.Now()) > renewBefore {
		return 0, nil
	}
	if _, err := service.parts.Queue.Enqueue(ctx, jobs.Request{
		Kind: jobs.KindRenewCertificate, ArgsJSON: "{}", Priority: jobs.PriorityBackground,
		Reason: "the server's certificate is missing or expires within a month",
	}); err != nil {
		return 0, fmt.Errorf("queue the certificate's renewal: %w", err)
	}
	return 1, nil
}

// HandleRenew obtains a certificate, the renewal's job: a refused token is the admin's to fix, so
// it is a failure that asks no more; anything else is tried again on the job's back-off.
func (service *Service) HandleRenew(ctx context.Context, _ jobs.Job) error {
	err := service.Obtain(ctx)
	if errors.Is(err, cloudflare.ErrTokenRefused) || errors.Is(err, cloudflare.ErrNoZone) {
		return jobs.MarkPermanent(err)
	}
	return err
}

// Obtain obtains a certificate for the domain by ACME DNS-01 and makes it the one served and kept:
// the challenge's TXT record is set through the DNS provider, given time to reach the name's
// servers, answered, and removed again whatever happened (D243 §6).
func (service *Service) Obtain(ctx context.Context) error {
	client, err := service.client(ctx)
	if err != nil {
		return err
	}
	order, err := client.AuthorizeOrder(ctx, acme.DomainIDs(service.parts.Domain))
	if err != nil {
		return fmt.Errorf("order a certificate for %s: %w", service.parts.Domain, err)
	}
	for _, authorizationURL := range order.AuthzURLs {
		if authorizeErr := service.authorize(ctx, client, authorizationURL); authorizeErr != nil {
			return authorizeErr
		}
	}
	if order, err = client.WaitOrder(ctx, order.URI); err != nil {
		return fmt.Errorf("wait for the order of %s: %w", service.parts.Domain, err)
	}
	certificateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("make the certificate's key: %w", err)
	}
	request, err := x509.CreateCertificateRequest(rand.Reader,
		&x509.CertificateRequest{DNSNames: []string{service.parts.Domain}}, certificateKey)
	if err != nil {
		return fmt.Errorf("make the certificate request: %w", err)
	}
	chain, _, err := client.CreateOrderCert(ctx, order.FinalizeURL, request, true)
	if err != nil {
		return fmt.Errorf("finalize the certificate of %s: %w", service.parts.Domain, err)
	}
	return service.keep(chain, certificateKey)
}

// authorize answers one authorization's DNS-01 challenge.
func (service *Service) authorize(ctx context.Context, client *acme.Client, authorizationURL string) error {
	authorization, err := client.GetAuthorization(ctx, authorizationURL)
	if err != nil {
		return fmt.Errorf("read an authorization: %w", err)
	}
	if authorization.Status == acme.StatusValid {
		return nil
	}
	var challenge *acme.Challenge
	for _, each := range authorization.Challenges {
		if each.Type == "dns-01" {
			challenge = each
		}
	}
	if challenge == nil {
		return fmt.Errorf("the CA offers no dns-01 challenge for %s", authorization.Identifier.Value)
	}
	value, err := client.DNS01ChallengeRecord(challenge.Token)
	if err != nil {
		return fmt.Errorf("make the challenge's record: %w", err)
	}
	record, err := service.parts.DNS.SetTXT(ctx, "_acme-challenge."+authorization.Identifier.Value, value)
	if err != nil {
		return err
	}
	defer func() {
		if removeErr := service.parts.DNS.RemoveTXT(context.WithoutCancel(ctx), record); removeErr != nil {
			service.parts.Logger.WarnContext(ctx, "could not remove a challenge's TXT record", "err", removeErr)
		}
	}()
	if waitErr := service.waitForPropagation(ctx); waitErr != nil {
		return waitErr
	}
	if _, err = client.Accept(ctx, challenge); err != nil {
		return fmt.Errorf("answer the challenge: %w", err)
	}
	if _, err = client.WaitAuthorization(ctx, authorizationURL); err != nil {
		return fmt.Errorf("wait for the challenge to be checked: %w", err)
	}
	return nil
}

// waitForPropagation waits PropagationWait, or until ctx ends.
func (service *Service) waitForPropagation(ctx context.Context) error {
	if service.parts.PropagationWait <= 0 {
		return nil
	}
	timer := service.parts.Clock.NewTimer(service.parts.PropagationWait)
	defer timer.Stop()
	select {
	case <-timer.C():
		return nil
	case <-ctx.Done():
		return fmt.Errorf("wait for the challenge's record to propagate: %w", ctx.Err())
	}
}

// client is an ACME client with the account's key, made and kept on first use, registered.
func (service *Service) client(ctx context.Context) (*acme.Client, error) {
	key, err := service.accountKey()
	if err != nil {
		return nil, err
	}
	client := &acme.Client{Key: key, DirectoryURL: service.parts.DirectoryURL, HTTPClient: service.parts.HTTP}
	account := &acme.Account{}
	if service.parts.Contact != "" {
		account.Contact = []string{"mailto:" + service.parts.Contact}
	}
	_, err = client.Register(ctx, account, acme.AcceptTOS)
	if err != nil && !errors.Is(err, acme.ErrAccountAlreadyExists) {
		return nil, fmt.Errorf("register with the CA: %w", err)
	}
	return client, nil
}

// accountKey is the ACME account's key, made and kept the first time.
func (service *Service) accountKey() (crypto.Signer, error) {
	kept, err := service.parts.Files.ReadPersistentFile(accountKeyFile)
	if err == nil {
		block, _ := pem.Decode(kept)
		if block == nil {
			return nil, errors.New("the ACME account key file holds no key")
		}
		key, parseErr := x509.ParseECPrivateKey(block.Bytes)
		if parseErr != nil {
			return nil, fmt.Errorf("read the ACME account key: %w", parseErr)
		}
		return key, nil
	}
	if !errors.Is(err, instancedir.ErrNotFound) {
		return nil, fmt.Errorf("read the ACME account key: %w", err)
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("make the ACME account key: %w", err)
	}
	encoded, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("encode the ACME account key: %w", err)
	}
	if err = service.parts.Files.WritePersistentFile(accountKeyFile,
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: encoded})); err != nil {
		return nil, fmt.Errorf("keep the ACME account key: %w", err)
	}
	return key, nil
}

// keep makes a certificate chain and its key the one served, and keeps them.
func (service *Service) keep(chain [][]byte, key *ecdsa.PrivateKey) error {
	var certificatePEM []byte
	for _, der := range chain {
		certificatePEM = append(certificatePEM, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})...)
	}
	encodedKey, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return fmt.Errorf("encode the certificate's key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: encodedKey})
	pair, err := tls.X509KeyPair(certificatePEM, keyPEM)
	if err != nil {
		return fmt.Errorf("the CA's certificate does not match its key: %w", err)
	}
	if err = service.parts.Files.WritePersistentFile(keyFile, keyPEM); err != nil {
		return fmt.Errorf("keep the certificate's key: %w", err)
	}
	if err = service.parts.Files.WritePersistentFile(certificateFile, certificatePEM); err != nil {
		return fmt.Errorf("keep the certificate: %w", err)
	}
	service.current.Store(&pair)
	service.parts.Logger.Info("obtained the server's certificate", "domain", service.parts.Domain,
		"expiresAt", pair.Leaf.NotAfter)
	return nil
}
