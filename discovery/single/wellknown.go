// SPDX-License-Identifier: AGPL-3.0-or-later

package single

import (
	"net/http"

	"github.com/kstenerud/orpheum/discovery/trust"
)

// WellKnownHandler serves the operator's published directory (Parts.Published) under /.well-known/,
// to be mounted at the issuer's root, through trust.Served: the address document, the root chain, the
// release record and the publisher's chain, each beside its .sig, as `discovery-sign` wrote them.
// With no directory, nothing is found.
func (discovery *Discovery) WellKnownHandler() http.Handler {
	if discovery.parts.Published == "" {
		return http.NotFoundHandler()
	}
	return trust.Served(discovery.parts.Published, discovery.parts.Logger)
}
