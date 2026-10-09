// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"crypto/tls"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kstenerud/orpheum/internal/core/certificate"
	"github.com/kstenerud/orpheum/internal/core/jobs"
)

// discoveryFilePrefix keeps the certificate of the core's names under its discovery apart from the
// owner's own name's.
const discoveryFilePrefix = "discovery-"

// servedCertificates are the certificates HTTPS serves: the owner's own name's (https.domain,
// D243 §6), and the wildcard of the core's names under its discovery's zone, once the discovery has
// claimed the core (D273). A browser is served the one for the name it asks for.
type servedCertificates struct {
	// own is the owner's own name's; nil for a server with no https.domain.
	own *certificate.Service
	// discoveryParts make the discovery's certificate, but for its name.
	discoveryParts certificate.Parts
	// underDiscovery is the discovery's; nil before the discovery has named the core.
	underDiscovery atomic.Pointer[certificate.Service]

	mutex sync.Mutex
	// Guarded by mutex: onServable is told once there is a certificate to serve; nil tells nothing.
	onServable func()
}

// errNoPublicName is a renewal asked of a server with no name to hold a certificate for.
var errNoPublicName = errors.New("the server has no public name (https.domain) and no discovery, " +
	"so no certificate to renew")

// all are the certificates the server holds.
func (served *servedCertificates) all() []*certificate.Service {
	var all []*certificate.Service
	if served.own != nil {
		all = append(all, served.own)
	}
	if discovered := served.underDiscovery.Load(); discovered != nil {
		all = append(all, discovered)
	}
	return all
}

// useDiscoveryName makes name, the wildcard the discovery publishes the core's names under, the
// discovery's certificate's, serving one kept for it; an empty or unchanged name does nothing.
func (served *servedCertificates) useDiscoveryName(name string) error {
	if current := served.underDiscovery.Load(); name == "" || (current != nil && current.Domain() == name) {
		return nil
	}
	parts := served.discoveryParts
	parts.Domain, parts.FilePrefix = name, discoveryFilePrefix
	service, err := certificate.New(parts)
	if err != nil {
		return err
	}
	served.underDiscovery.Store(service)
	served.tellIfServable()
	return nil
}

// whenServable has start told once there is a certificate to serve: at once when there is one.
func (served *servedCertificates) whenServable(start func()) {
	served.mutex.Lock()
	served.onServable = start
	served.mutex.Unlock()
	served.tellIfServable()
}

// tellIfServable tells onServable, once, when a certificate can be served.
func (served *servedCertificates) tellIfServable() {
	if _, err := served.GetCertificate(&tls.ClientHelloInfo{}); err != nil {
		return
	}
	served.mutex.Lock()
	start := served.onServable
	served.onServable = nil
	served.mutex.Unlock()
	if start != nil {
		start()
	}
}

// GetCertificate is the certificate for the name a browser asks for: the discovery's for a name
// under its wildcard, the owner's own name's otherwise, and whichever there is when only one is.
func (served *servedCertificates) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	discovered := served.underDiscovery.Load()
	if discovered != nil && (served.own == nil || isUnder(hello.ServerName, discovered.Domain())) {
		return discovered.GetCertificate(hello)
	}
	if served.own != nil {
		return served.own.GetCertificate(hello)
	}
	return nil, certificate.ErrNoCertificate
}

// isUnder is whether name is one label under wildcard's (*.k7xq2.<zone>), as the wildcard covers.
func isUnder(name, wildcard string) bool {
	base, isWildcard := strings.CutPrefix(wildcard, "*")
	label, isUnderBase := strings.CutSuffix(strings.ToLower(name), base)
	return isWildcard && isUnderBase && label != "" && !strings.Contains(label, ".")
}

// ExpiresAt is when the owner's own name's certificate expires, or the discovery's when the server
// has only that: the System page's view.
func (served *servedCertificates) ExpiresAt() (time.Time, bool) {
	held := served.all()
	if len(held) == 0 {
		return time.Time{}, false
	}
	return held[0].ExpiresAt()
}

// QueueRenewal asks for the renewal of each certificate that is due, and says how many it asked for.
func (served *servedCertificates) QueueRenewal(ctx context.Context) (int, error) {
	asked := 0
	for _, held := range served.all() {
		count, err := held.QueueRenewal(ctx)
		if err != nil {
			return asked, err
		}
		asked += count
	}
	return asked, nil
}

// HandleRenew renews each certificate that is due, the renewal's job: one that fails to be tried
// again is tried again, after the others; only when every failure asks no more does the job.
func (served *servedCertificates) HandleRenew(ctx context.Context, job jobs.Job) error {
	held := served.all()
	if len(held) == 0 {
		return jobs.MarkPermanent(errNoPublicName)
	}
	var retried, permanent []error
	for _, each := range held {
		if !each.IsDue() {
			continue
		}
		err := each.HandleRenew(ctx, job)
		switch {
		case err == nil:
		case jobs.IsMarkedPermanent(err):
			permanent = append(permanent, err)
		default:
			retried = append(retried, err)
		}
	}
	served.tellIfServable()
	if len(retried) > 0 {
		return errors.Join(retried...)
	}
	if len(permanent) > 0 {
		return jobs.MarkPermanent(errors.Join(permanent...))
	}
	return nil
}
