// SPDX-License-Identifier: AGPL-3.0-or-later

package home

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/platform/slogx"
	"github.com/kstenerud/orpheum/internal/store"
)

// Origin is where the address at home came from.
type Origin string

// The origins of the address at home: none yet, the config's (ORPHEUM_LAN_ADDRESS, which wins, D452
// §5), learned from a request from home (§3), set by an admin (§4), or guessed from the default
// route's interface by a core sharing the host's network, until one is learned or set.
const (
	OriginNone       Origin = ""
	OriginConfigured Origin = "configured"
	OriginLearned    Origin = "learned"
	OriginSet        Origin = "set"
	OriginGuessed    Origin = "guessed"
)

// EventAddressLearned is the log line's slogx.EventKey when the address at home changes.
const EventAddressLearned = "homeAddressLearned"

// resolveWithin is how long a name's lookup may take (D452 §4).
const resolveWithin = 2 * time.Second

// ErrConfigured is a set refused because the config names the address at home, which wins (D452 §5).
var ErrConfigured = errors.New("the address at home is configured (ORPHEUM_LAN_ADDRESS), which wins over one set here")

// ErrNotAtHome is a set refused because the address is no private IPv4 address.
var ErrNotAtHome = errors.New("the address at home is a private IPv4 address, \"192.168.1.20\"")

// Known is the address at home and where it came from: the device that taught it, or the admin who
// set it, and when. An invalid Address is none known.
type Known struct {
	Address   netip.Addr
	Origin    Origin
	Device    string
	SetByName string
	At        time.Time
}

// Interface is one of the machine's network interfaces as Guess weighs it: its name and addresses.
type Interface struct {
	Name      string
	Addresses []netip.Addr
}

// AddressParts are what an AddressAtHome is made of.
type AddressParts struct {
	Rule *Rule
	// Settings keeps the address at home, as the settings address_at_home and
	// address_at_home_learned_from.
	Settings *settings.Service
	// Database names the admin who set the address.
	Database *store.Database
	Logger   *slog.Logger
	// Configured is the address the config names (discovery.lan_addresses, from ORPHEUM_LAN_ADDRESS);
	// invalid for none. Set, it wins and nothing is learned (D452 §5).
	Configured netip.Addr
	// Resolve looks a name up: net.DefaultResolver's, or a test's (D452 §4).
	Resolve func(ctx context.Context, name string) ([]netip.Addr, error)
	// Interfaces are the machine's interfaces, for Guess: cmd/orpheum's systemInterfaces, or a test's.
	Interfaces func() []Interface
	// DefaultRoute is the address of the interface carrying the default route (cmd/orpheum reads it
	// with DefaultRouteAddress, or a test's), taken as the address at home while none is learned or
	// set; nil for a core in a container's own network, whose default route is Docker's bridge, which
	// would make the internet home. Without it, a core sharing the host's network counts the house's
	// network as one of its own interfaces' and so not home, and no request from home could teach it.
	DefaultRoute func() netip.Addr
	// LearnsOnlyHeld is set for a core sharing the host's network: it learns only an address one of
	// Interfaces holds, as its own address in the house must be. The Host a request names is the
	// device's to write, so without it any device at home could point the server's host candidates and
	// heartbeat at another address. A core in a container's own network holds none of the house's
	// addresses and cannot check this; there the address is configured.
	LearnsOnlyHeld bool
}

// AddressAtHome is the server's address at home (D452 §3, §4): the configured one, else the one last
// learned from a request from home or set by an admin, kept as a setting, and handed live to the rule
// and to whatever OnChange names (the discovery's lan_addresses, WebRTC's host candidates). A change
// on the settings page, a reset too, reaches them the same way.
type AddressAtHome struct {
	parts AddressParts
	known atomic.Pointer[Known]
	// guess is the default route's address, read once at the start; invalid for none.
	guess netip.Addr

	mutex sync.Mutex
	// Guarded by mutex: the reactions to a change, and the names already looked up.
	reactions []func(netip.Addr)
	looked    map[string]bool
}

// NewAddressAtHome is the address at home, configured or as the settings keep it, applied to the rule.
func NewAddressAtHome(ctx context.Context, parts AddressParts) (*AddressAtHome, error) {
	keeper := &AddressAtHome{parts: parts, looked: map[string]bool{}}
	keeper.known.Store(&Known{})
	if parts.Configured.IsValid() {
		known := Known{Address: parts.Configured.Unmap(), Origin: OriginConfigured}
		keeper.known.Store(&known)
		parts.Rule.SetAddressAtHome(known.Address)
		return keeper, nil
	}
	if parts.DefaultRoute != nil {
		if guess := parts.DefaultRoute(); IsAtHome(guess) {
			keeper.guess = guess.Unmap()
		}
	}
	if err := keeper.reload(ctx); err != nil {
		return nil, err
	}
	parts.Settings.OnChange(settings.AddressAtHome, keeper.reload)
	return keeper, nil
}

// reload reads the address at home from the settings and, when it changed, applies it, tells every
// reaction and logs it.
func (keeper *AddressAtHome) reload(ctx context.Context) error {
	known, err := keeper.stored(ctx)
	if err != nil {
		return err
	}
	keeper.mutex.Lock()
	defer keeper.mutex.Unlock()
	old := *keeper.known.Load()
	keeper.known.Store(&known)
	if old.Address == known.Address {
		return nil
	}
	keeper.parts.Rule.SetAddressAtHome(known.Address)
	for _, react := range keeper.reactions {
		react(known.Address)
	}
	keeper.parts.Logger.InfoContext(ctx, "the address at home changed", slogx.EventKey, EventAddressLearned,
		"old", old.Address, "new", known.Address, "origin", known.Origin, "device", known.Device, "setByName",
		known.SetByName)
	return nil
}

// stored is the address at home the settings keep.
func (keeper *AddressAtHome) stored(ctx context.Context) (Known, error) {
	entries, err := keeper.parts.Settings.List(ctx, false)
	if err != nil {
		return Known{}, fmt.Errorf("read the address at home: %w", err)
	}
	var address, device string
	var setterID *int64
	var setAt int64
	for _, entry := range entries {
		if key := entry.Definition.Key; key == settings.AddressAtHome {
			_ = json.Unmarshal(entry.Value, &address) //nolint:errcheck // the registry checked it is a string
			setterID, setAt = entry.SetByUserID, entry.SetAt
		} else if key == settings.AddressAtHomeLearnedFrom {
			_ = json.Unmarshal(entry.Value, &device) //nolint:errcheck // the registry checked it is a string
		}
	}
	parsed, err := netip.ParseAddr(address)
	if err != nil { // empty, or unparsable: none known but the guess
		if keeper.guess.IsValid() {
			return Known{Address: keeper.guess, Origin: OriginGuessed}, nil
		}
		return Known{}, nil
	}
	known := Known{Address: parsed, Origin: OriginLearned, Device: device, At: time.UnixMilli(setAt)}
	if setterID != nil || device == "" {
		known.Origin, known.Device = OriginSet, ""
		if setterID != nil && keeper.parts.Database != nil {
			if user, userErr := keeper.parts.Database.Read().GetLiveUserByID(ctx, *setterID); userErr == nil {
				known.SetByName = user.Name
			}
		}
	}
	return known, nil
}

// Known is the address at home and where it came from.
func (keeper *AddressAtHome) Known() Known {
	if keeper == nil {
		return Known{}
	}
	return *keeper.known.Load()
}

// Address is the address at home; invalid for none.
func (keeper *AddressAtHome) Address() netip.Addr {
	return keeper.Known().Address
}

// OnChange runs react with the address at home now, when there is one, and at each change, with an
// invalid address when it is forgotten (reset on the settings page).
func (keeper *AddressAtHome) OnChange(react func(netip.Addr)) {
	keeper.mutex.Lock()
	defer keeper.mutex.Unlock()
	keeper.reactions = append(keeper.reactions, react)
	if address := keeper.known.Load().Address; address.IsValid() {
		react(address)
	}
}

// IsAtHome is whether address may be the address at home: a private IPv4 address, not loopback.
func IsAtHome(address netip.Addr) bool {
	address = address.Unmap()
	return address.Is4() && address.IsPrivate()
}

// Learn takes address, which a request from home named the server by, as the address at home, taught
// by device (its user agent), unless the config names one or it is the one known: a request that
// teaches nothing new reads one pointer and writes nothing (D452 §3).
func (keeper *AddressAtHome) Learn(ctx context.Context, address netip.Addr, device string) {
	address = address.Unmap()
	if keeper == nil || !IsAtHome(address) {
		return
	}
	known := keeper.known.Load()
	if known.Origin == OriginConfigured || known.Address == address && known.Origin != OriginGuessed {
		return
	}
	if keeper.parts.LearnsOnlyHeld && !keeper.holds(address) {
		keeper.parts.Logger.InfoContext(ctx, "a request at home named an address this machine does not hold; "+
			"it is not learned", "address", address, "device", truncated(device))
		return
	}
	if err := keeper.write(ctx, address, truncated(device), nil); err != nil {
		keeper.parts.Logger.ErrorContext(ctx, "the address at home was not learned", "address", address, "err", err)
	}
}

// holds is whether one of the machine's interfaces holds address.
func (keeper *AddressAtHome) holds(address netip.Addr) bool {
	if keeper.parts.Interfaces == nil {
		return false
	}
	for _, each := range keeper.parts.Interfaces() {
		for _, held := range each.Addresses {
			if held.Unmap() == address {
				return true
			}
		}
	}
	return false
}

// IsLearning is whether a name may still teach the address at home: none is known but a guess.
func (keeper *AddressAtHome) IsLearning() bool {
	known := keeper.Known()
	return !known.Address.IsValid() || known.Origin == OriginGuessed
}

// LearnName looks name up, which a request from home named the server by, while no address at home is
// known but a guess, and learns its first private IPv4 answer (D452 §4). Each name is looked up once.
func (keeper *AddressAtHome) LearnName(ctx context.Context, name, device string) {
	if keeper == nil || keeper.parts.Resolve == nil || !keeper.IsLearning() {
		return
	}
	name = strings.TrimSuffix(strings.ToLower(name), ".")
	keeper.mutex.Lock()
	isLooked := keeper.looked[name]
	keeper.looked[name] = true
	keeper.mutex.Unlock()
	if isLooked || name == "" || name == "localhost" {
		return
	}
	lookup, cancel := context.WithTimeout(ctx, resolveWithin)
	defer cancel()
	answers, err := keeper.parts.Resolve(lookup, name)
	if err != nil {
		keeper.parts.Logger.InfoContext(ctx, "the server's name gave no address at home", "name", name, "err", err)
		return
	}
	for _, answer := range answers {
		if IsAtHome(answer) {
			keeper.Learn(ctx, answer, device)
			return
		}
	}
	keeper.parts.Logger.InfoContext(ctx, "the server's name gave no private IPv4 address", "name", name,
		"answers", answers)
}

// Set is an admin's setting of the address at home (D452 §4), kept as a learned one is, naming them.
func (keeper *AddressAtHome) Set(ctx context.Context, address netip.Addr, byUserID int64) error {
	address = address.Unmap()
	switch {
	case keeper.known.Load().Origin == OriginConfigured:
		return ErrConfigured
	case !IsAtHome(address):
		return ErrNotAtHome
	}
	return keeper.write(ctx, address, "", &byUserID)
}

// write keeps address as the settings, which the reaction NewAddressAtHome registered applies.
func (keeper *AddressAtHome) write(ctx context.Context, address netip.Addr, device string, byUserID *int64) error {
	addressJSON, err := json.Marshal(address.String())
	if err != nil {
		return fmt.Errorf("encode the address at home: %w", err)
	}
	deviceJSON, err := json.Marshal(device)
	if err != nil {
		return fmt.Errorf("encode the device: %w", err)
	}
	return keeper.parts.Settings.Set(ctx, map[string]json.RawMessage{
		string(settings.AddressAtHome): addressJSON, string(settings.AddressAtHomeLearnedFrom): deviceJSON,
	}, byUserID, false)
}

// Guess is the core's best guess at the address at home, for the setup page to fill in (D452 §4): the
// first private IPv4 address of an interface that is no container network's, and not in a network
// the rule excepts; invalid for none.
func (keeper *AddressAtHome) Guess() netip.Addr {
	if keeper == nil || keeper.parts.Interfaces == nil {
		return netip.Addr{}
	}
	var notHome []netip.Prefix
	if networks := keeper.parts.Rule.networks.Load(); networks != nil {
		notHome = networks.NotHome
	}
	for _, each := range keeper.parts.Interfaces() {
		if isContainerInterface(each.Name) {
			continue
		}
		for _, address := range each.Addresses {
			if address = address.Unmap(); IsAtHome(address) && !isInAny(address, notHome) {
				return address
			}
		}
	}
	return netip.Addr{}
}

// isContainerInterface is whether name is one of the interfaces Docker and its kin make: a bridge or
// a container's end of a pair, whose addresses nothing in the house reaches.
func isContainerInterface(name string) bool {
	for _, prefix := range []string{"docker", "br-", "veth", "cni", "flannel", "podman", "virbr", "lxc", "wg", "tun"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// truncated is device cut to settings.MaxLearnedFromLength bytes, on a rune's boundary.
func truncated(device string) string {
	if len(device) <= settings.MaxLearnedFromLength {
		return device
	}
	cut := settings.MaxLearnedFromLength
	for cut > 0 && device[cut]&0xC0 == 0x80 {
		cut--
	}
	return device[:cut]
}
