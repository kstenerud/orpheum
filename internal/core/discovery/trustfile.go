// SPDX-License-Identifier: AGPL-3.0-or-later

package discovery

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/kstenerud/orpheum/discovery/addressdoc"
	"github.com/kstenerud/orpheum/discovery/trust"
	"github.com/kstenerud/orpheum/internal/core/instancedir"
	"github.com/kstenerud/orpheum/pkg/strictjson"
)

// The trust state's files, one per chain the core follows, in the persistent directory (the root key
// audit's §3.3): written whole, every signed statement kept as received, checked again at each start.
const (
	DiscoveryTrustFile = "trust-discovery"
	PublisherTrustFile = "trust-publisher"
)

// discoveryTrust is trust-discovery: the discovery's identity and issuer, every root statement
// accepted, the address document held, its floor, and every place it is served with when it was last
// named. Only the admin's enrolment makes it; a Leave removes it.
type discoveryTrust struct {
	Identity   string         `json:"identity"`
	Issuer     string         `json:"issuer"`
	Statements []trust.Signed `json:"statements"`
	Document   trust.Signed   `json:"document"`
	Floor      floor          `json:"floor"`
	Places     []place        `json:"places"`
}

// keepTrust writes trust-discovery from what the core holds now.
func (service *Service) keepTrust() error {
	service.mutex.Lock()
	opened := service.held
	service.mutex.Unlock()
	state := discoveryTrust{
		Identity: opened.chain.ID(), Issuer: opened.issuer, Statements: opened.chain.Statements(),
		Document: opened.signed, Floor: opened.floor, Places: opened.places,
	}
	return service.writeJSON(DiscoveryTrustFile, state)
}

// writeJSON writes value to the persistent file name, whole.
func (service *Service) writeJSON(name string, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode %s: %w", name, err)
	}
	if err = service.parts.Files.WritePersistentFile(name, encoded); err != nil {
		return fmt.Errorf("keep %s: %w", name, err)
	}
	return nil
}

// readJSON reads the persistent file name into value, and whether there is one.
func (service *Service) readJSON(name string, value any) (bool, error) {
	contents, err := service.parts.Files.ReadPersistentFile(name)
	if errors.Is(err, instancedir.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read %s: %w", name, err)
	}
	if err = strictjson.Unmarshal(contents, value); err != nil {
		return false, fmt.Errorf("read %s: %w", name, err)
	}
	return true, nil
}

// loadTrust is the discovery trust-discovery holds, its chain checked again from its genesis and its
// document under it; a state that does not hold is refused (§3.3). A document the chain no longer
// signs is still read, its addresses used until a newer one holds, and the problem said.
func (service *Service) loadTrust() (held, string, error) {
	var state discoveryTrust
	isFound, err := service.readJSON(DiscoveryTrustFile, &state)
	if err != nil || !isFound {
		return held{}, "", err
	}
	chain, err := trust.Load(trust.ChainDiscovery, state.Identity, state.Statements)
	if err != nil {
		return held{}, "", fmt.Errorf("%s does not hold: %w", DiscoveryTrustFile, err)
	}
	problem := ""
	document, err := addressdoc.Open(state.Document, chain, state.Issuer)
	if err != nil {
		problem = "the address document held is no longer signed by the discovery's keys"
		if document, err = addressdoc.Parse([]byte(state.Document.Payload)); err != nil {
			return held{}, "", fmt.Errorf("%s's address document: %w", DiscoveryTrustFile, err)
		}
	}
	return held{
		issuer: state.Issuer, chain: chain, signed: state.Document, document: document, floor: state.Floor,
		places: state.Places,
	}, problem, nil
}
