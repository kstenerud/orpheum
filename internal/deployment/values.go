// SPDX-License-Identifier: AGPL-3.0-or-later

package deployment

import (
	"encoding/base64"
	"fmt"
	"net"
	"net/netip"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/BurntSushi/toml"
)

// Kind is which deployment a directory is: release refuses a database it cannot migrate, staging
// sets it aside and starts again (D249 §4).
type Kind string

// The kinds of deployment.
const (
	Release Kind = "release"
	Staging Kind = "staging"
)

// The tunnel's MTU by the family of the address it is carried over: WireGuard's packets over IPv6
// cost 80 bytes of a 1500-byte link, and over DS-Lite's IPv4, itself carried in IPv6, 40 more.
const (
	mtuOverIPv6 = 1420
	mtuOverIPv4 = 1380
)

// The tunnel's subnets, whose first address is the VPS's end (deploy/vps/vps-wg0.conf); each
// deployment's end is another address in them, so several deployments share the one VPS (D292 §7).
// The tunnel service (deploy/staging/tunnel.sh) gives wg0 its addresses with these prefixes.
const (
	tunnelSubnetIPv4 = "10.77.0.0/24"
	tunnelSubnetIPv6 = "fd77::/64"
)

// The tunnel addresses a values file that names none has: the first deployment's, which every
// deployment had before each could have its own (D272 §8, revised by D292 §7).
const (
	firstTunnelIPv4 = "10.77.0.2"
	firstTunnelIPv6 = "fd77::2"
)

// turnPort is TURN's port on the tunnel's IPv4 address.
const turnPort = 3478

// The dev loop's ports, which a deployment beside it must not take.
const (
	devServerPort = 8432
	devVitePort   = 5173
)

// Values is the owner's values file, values.toml in the deployment's directory.
type Values struct {
	Deployment Kind   `toml:"deployment"`
	Domain     string `toml:"domain"`
	Contact    string `toml:"contact"`
	VPS        struct {
		// Endpoint is the VPS's address and WireGuard port: "[IPv6]:port" or "IPv4:port".
		Endpoint   string `toml:"endpoint"`
		PublicIPv4 string `toml:"public_ipv4"`
		// PublicIPv6 is the VPS's public IPv6 address that reaches this deployment, when it has one of
		// its own (the edge VPS's ::3 for staging); empty when none does.
		PublicIPv6 string `toml:"public_ipv6"`
		PublicKey  string `toml:"public_key"`
		// MTU is 0 when the values file leaves it out: chosen from the endpoint's family.
		MTU int `toml:"mtu"`
		// TunnelIPv4 and TunnelIPv6 are this deployment's end of the tunnel, which the VPS's
		// wg0.conf names as this deployment's peer.
		TunnelIPv4 string `toml:"tunnel_ipv4"`
		TunnelIPv6 string `toml:"tunnel_ipv6"`
	} `toml:"vps"`
	Media struct {
		Roots []string `toml:"roots"`
	} `toml:"media"`
	Node struct {
		// Name is the machine's host name when the values file leaves it out.
		Name string `toml:"name"`
	} `toml:"node"`
	HTTP struct {
		Publish string `toml:"publish"`
		Port    int    `toml:"port"`
		// ThisMachinePort is plain HTTP for a browser on this machine alone, published on its
		// loopback, the same port inside: a request on it is from home, though Docker hands it the
		// core from its gateway (D334 §4).
		ThisMachinePort int `toml:"this_machine_port"`
	} `toml:"http"`
	// LAN is where devices at home reach the core over HTTPS, by the name its discovery gives this
	// machine's address there: the address, which the core's container cannot see, and the port HTTPS
	// is published on (the owner, 2026-10-14). An empty address names none; devices at home then
	// reach the core through the VPS.
	LAN struct {
		Address   string `toml:"address"`
		HTTPSPort int    `toml:"https_port"`
		// WebRTCPort is the UDP port WebRTC is published on at the address, the same inside (D300 §1).
		WebRTCPort int `toml:"webrtc_port"`
		// Router is the machine's router at home, which the core's port mapping asks: its container's
		// default route is its own network's bridge (item 15 of the owner's comments of 2026-11-02).
		// Empty maps nothing.
		Router string `toml:"router"`
	} `toml:"lan"`
}

// InstallRoute is how a deployment was installed, which its core's config names so that the System
// page shows only that route's steps to update (D450 §2, amended 2026-11-08).
type InstallRoute string

// The routes a deployment is installed by.
const (
	// ByInstallScript is the install script, `orpheum write-deployment` (D272, D314): it updates by
	// running the new release's install script.
	ByInstallScript InstallRoute = "script"
	// ByComposeFile is a compose file whose every start is `orpheum start` (D447, D449): it updates by
	// pulling the images it follows and starting again.
	ByComposeFile InstallRoute = "compose"
)

// thisMachinePortDefault is the port a browser on this machine reaches the core on, as home, unless the
// values file names another (D334 §4).
const thisMachinePortDefault = 8441

// lanHTTPSPortDefault is the port HTTPS is published on at home unless the values file names another.
const lanHTTPSPortDefault = 8443

// StartingValues are a new deployment's values, of kind: every value a values file may leave out at
// its starting value, and nothing filled in. The values file a deployment begins with (Begin) is
// written from them; the first start (D447) fills in what its command line gives.
func StartingValues(kind Kind) Values {
	values := Values{Deployment: kind}
	values.HTTP.Publish = "0.0.0.0"
	values.HTTP.Port = 8440
	values.HTTP.ThisMachinePort = thisMachinePortDefault
	values.LAN.HTTPSPort = lanHTTPSPortDefault
	values.LAN.WebRTCPort = lanHTTPSPortDefault
	values.VPS.TunnelIPv4 = firstTunnelIPv4
	values.VPS.TunnelIPv6 = firstTunnelIPv6
	return values
}

// ParseValues reads a values file; a value it leaves out is its starting value, and the node's name
// is hostName.
func ParseValues(text, hostName string) (Values, error) {
	values := StartingValues("") // the file names its kind
	if _, err := toml.Decode(text, &values); err != nil {
		return Values{}, fmt.Errorf("the values file: %w", err)
	}
	values.Domain = strings.TrimSpace(values.Domain)
	values.Contact = strings.TrimSpace(values.Contact)
	values.VPS.Endpoint = strings.TrimSpace(values.VPS.Endpoint)
	values.VPS.PublicIPv4 = strings.TrimSpace(values.VPS.PublicIPv4)
	values.VPS.PublicIPv6 = strings.TrimSpace(values.VPS.PublicIPv6)
	values.VPS.PublicKey = strings.TrimSpace(values.VPS.PublicKey)
	values.VPS.TunnelIPv4 = strings.TrimSpace(values.VPS.TunnelIPv4)
	values.VPS.TunnelIPv6 = strings.TrimSpace(values.VPS.TunnelIPv6)
	values.LAN.Address = strings.TrimSpace(values.LAN.Address)
	values.Node.Name = strings.TrimSpace(values.Node.Name)
	if values.Node.Name == "" {
		values.Node.Name = hostName
	}
	return values, nil
}

// HasTunnel is whether the deployment reaches the internet through a WireGuard tunnel to a VPS: a
// values file that names the VPS's endpoint. One that names none is reached through its discovery
// over WebRTC and at home by its LAN names, with no relay of its own (D314 §3, §4).
func (values Values) HasTunnel() bool { return values.VPS.Endpoint != "" }

// TunnelMTU is the values file's MTU, or the one for the endpoint's address family.
func (values Values) TunnelMTU() int {
	if values.VPS.MTU != 0 {
		return values.VPS.MTU
	}
	if strings.HasPrefix(values.VPS.Endpoint, "[") {
		return mtuOverIPv6
	}
	return mtuOverIPv4
}

// httpsPort is the port the core serves HTTPS on (templates/core.toml's https.listen), which the VPS
// forwards.
const httpsPort = "443"

// ConfiguredAddresses are the VPS's public addresses on the HTTPS port, which the core sends its
// discovery as configured candidates: the VPS forwards them through the tunnel (the protocol's §2.4;
// discovery-core.md K5).
func (values Values) ConfiguredAddresses() []string {
	if !values.HasTunnel() {
		return []string{}
	}
	addresses := []string{net.JoinHostPort(values.VPS.PublicIPv4, httpsPort)}
	if values.VPS.PublicIPv6 != "" {
		addresses = append(addresses, net.JoinHostPort(values.VPS.PublicIPv6, httpsPort))
	}
	return addresses
}

// LANAddresses are this machine's address at home on the published HTTPS port, which the core sends
// its discovery to name for devices at home; none when the values file names no address.
func (values Values) LANAddresses() []string {
	if values.LAN.Address == "" {
		return []string{}
	}
	return []string{net.JoinHostPort(values.LAN.Address, strconv.Itoa(values.LAN.HTTPSPort))}
}

// WebRTCUDPPort is the UDP port the core takes WebRTC on, published at the house's address on the
// same port (D300 §1); 0, none, when the values file names no address at home.
func (values Values) WebRTCUDPPort() int {
	if values.LAN.Address == "" {
		return 0
	}
	return values.LAN.WebRTCPort
}

// homeNetworkBits are how much of the LAN address names the house's network: a home router's /24,
// or an IPv6 /64.
const (
	homeNetworkBitsIPv4 = 24
	homeNetworkBitsIPv6 = 64
)

// HomeNetworks are the networks an admin administers from (D297 §6): the house's, the LAN address's
// /24, never the tunnel's 10.77.0.0/24 nor its fd77::/64, which a private range would take in. With no
// LAN address, which Problems refuses, there are none: a private range in its place took in Docker's
// networks, so a port published through docker-proxy made the internet look like home (the
// Plex-chain review's L1).
//
// The core's own loopback is home too: only a command run in its container reaches it, as the install
// script's, which makes the admin and pairs the core (D334 §2, §3); docker-proxy connects to the
// container's own address, never its loopback.
func (values Values) HomeNetworks() []string {
	address, err := netip.ParseAddr(values.LAN.Address)
	if err != nil {
		return []string{}
	}
	bits := homeNetworkBitsIPv4
	if address.Is6() {
		bits = homeNetworkBitsIPv6
	}
	return []string{netip.PrefixFrom(address, bits).Masked().String(), "127.0.0.0/8", "::1/128"}
}

// TURNPublicAddress is the address browsers reach the core's TURN at: the VPS's public IPv4 through
// the tunnel, or none with no tunnel, which serves no relay (D314 §4).
func (values Values) TURNPublicAddress() string {
	if !values.HasTunnel() {
		return ""
	}
	return values.VPS.PublicIPv4
}

// TURNListen is where the core's TURN listens: the tunnel's IPv4 address, so a UDP answer leaves
// from the address the VPS forwards to (deploy/vps/README.md §3).
func (values Values) TURNListen() string {
	return net.JoinHostPort(values.VPS.TunnelIPv4, strconv.Itoa(turnPort))
}

// Problems is what the values file still lacks or holds wrongly, each naming its key.
func (values Values) Problems() []string {
	var problems []string
	if values.Deployment != Release && values.Deployment != Staging {
		problems = append(problems, fmt.Sprintf("deployment %q: want %q or %q", values.Deployment, Release, Staging))
	}
	// A core with a name of its own gets its certificate itself; one reached through its discovery
	// has the discovery's (D292 §7).
	if values.Domain != "" && !strings.Contains(values.Contact, "@") {
		problems = append(problems, "contact: the e-mail address Let's Encrypt writes to")
	}
	if values.HasTunnel() || values.vpsIsPartlyGiven() {
		problems = append(problems, values.vpsProblems()...)
	}
	problems = append(problems, values.mediaProblems()...)
	return append(problems, values.httpProblems()...)
}

// vpsIsPartlyGiven is whether [vps] names any of the VPS's own values while leaving the endpoint
// out: a tunnel half described, which is a mistake rather than no tunnel.
func (values Values) vpsIsPartlyGiven() bool {
	return values.VPS.PublicIPv4 != "" || values.VPS.PublicIPv6 != "" || values.VPS.PublicKey != ""
}

// vpsProblems is what [vps] lacks or holds wrongly.
func (values Values) vpsProblems() []string {
	var problems []string
	if values.VPS.Endpoint == "" {
		problems = append(problems,
			`vps.endpoint: the VPS's IPv6 address and WireGuard port, "[2a01:...::1]:51820"`)
	} else if problem := endpointProblem(values.VPS.Endpoint); problem != "" {
		problems = append(problems, problem)
	}
	if address, err := netip.ParseAddr(values.VPS.PublicIPv4); err != nil || !address.Is4() {
		problems = append(problems, fmt.Sprintf("vps.public_ipv4 %q: the VPS's public IPv4 address",
			values.VPS.PublicIPv4))
	}
	if address, err := netip.ParseAddr(values.VPS.PublicIPv6); values.VPS.PublicIPv6 != "" &&
		(err != nil || !address.Is6() || address.Is4In6()) {
		problems = append(problems, fmt.Sprintf("vps.public_ipv6 %q: the VPS's public IPv6 address that reaches "+
			"this deployment, or leave it out", values.VPS.PublicIPv6))
	}
	if !isWireGuardKey(values.VPS.PublicKey) {
		problems = append(problems, "vps.public_key: the VPS's WireGuard public key, /etc/wireguard/public on the VPS")
	}
	if values.VPS.MTU != 0 && (values.VPS.MTU < 1280 || values.VPS.MTU > 1500) {
		problems = append(problems, fmt.Sprintf("vps.mtu %d: want 1280 to 1500, or leave it out: %d over IPv6 "+
			"and %d over IPv4 are chosen from the endpoint", values.VPS.MTU, mtuOverIPv6, mtuOverIPv4))
	}
	if problem := tunnelAddressProblem("vps.tunnel_ipv4", values.VPS.TunnelIPv4, tunnelSubnetIPv4); problem != "" {
		problems = append(problems, problem)
	}
	if problem := tunnelAddressProblem("vps.tunnel_ipv6", values.VPS.TunnelIPv6, tunnelSubnetIPv6); problem != "" {
		problems = append(problems, problem)
	}
	return problems
}

// tunnelAddressProblem says why address is not a deployment's address in subnet, or "": the
// subnet's first address is the VPS's.
func tunnelAddressProblem(key, address, subnetText string) string {
	subnet := netip.MustParsePrefix(subnetText)
	parsed, err := netip.ParseAddr(address)
	vps := subnet.Addr().Next()
	if err != nil || !subnet.Contains(parsed) || parsed == subnet.Addr() || parsed == vps {
		return fmt.Sprintf("%s %q: want an address in %s other than the VPS's %s, one no other "+
			"deployment on the VPS has", key, address, subnet, vps)
	}
	return ""
}

// mediaProblems is what [media] lacks or holds wrongly.
func (values Values) mediaProblems() []string {
	if len(values.Media.Roots) == 0 {
		return []string{"media.roots: the media directories the node serves"}
	}
	var problems []string
	for _, root := range values.Media.Roots {
		if !filepath.IsAbs(root) {
			problems = append(problems, fmt.Sprintf("media.roots %q: want an absolute path", root))
		}
	}
	return problems
}

// httpProblems is what [http] holds wrongly.
func (values Values) httpProblems() []string {
	var problems []string
	if _, err := netip.ParseAddr(values.HTTP.Publish); err != nil {
		problems = append(problems, fmt.Sprintf("http.publish %q: want an address of this machine, "+
			`"0.0.0.0" for every one`, values.HTTP.Publish))
	}
	if port := values.HTTP.Port; port == devServerPort || port == devVitePort || port < 1 || port > 65535 {
		problems = append(problems, fmt.Sprintf("http.port %d: want a port the dev loop does not use (%d, %d)",
			port, devServerPort, devVitePort))
	}
	if port := values.HTTP.ThisMachinePort; port == values.HTTP.Port || port == devServerPort ||
		port == devVitePort || port < 1 || port > 65535 {
		problems = append(problems, fmt.Sprintf("http.this_machine_port %d: want a port of its own, not http.port's "+
			"nor one the dev loop uses (%d, %d)", port, devServerPort, devVitePort))
	}
	return append(problems, values.lanProblems()...)
}

// lanProblems is what [lan] holds wrongly.
func (values Values) lanProblems() []string {
	var problems []string
	// Required: the house's network is where an admin administers from, and nothing else names it
	// (the Plex-chain review's L1).
	if address, err := netip.ParseAddr(values.LAN.Address); err != nil || !address.Unmap().IsPrivate() {
		problems = append(problems, fmt.Sprintf("lan.address %q: fill in this machine's address on the house's "+
			`network, "192.168.1.20"`, values.LAN.Address))
	}
	if port := values.LAN.HTTPSPort; port == devServerPort || port == devVitePort || port == values.HTTP.Port ||
		port < 1 || port > 65535 {
		problems = append(problems, fmt.Sprintf("lan.https_port %d: want a port other than http.port's and the "+
			"dev loop's (%d, %d)", port, devServerPort, devVitePort))
	}
	if port := values.LAN.WebRTCPort; port < 1 || port > 65535 {
		problems = append(problems, fmt.Sprintf("lan.webrtc_port %d: want a UDP port", port))
	}
	if router, err := netip.ParseAddr(values.LAN.Router); values.LAN.Router != "" &&
		(err != nil || !router.Is4() || !router.IsPrivate()) {
		problems = append(problems, fmt.Sprintf("lan.router %q: want this machine's router on the house's "+
			`network, "192.168.1.1", or nothing`, values.LAN.Router))
	}
	return problems
}

// Check is an error listing the values file's problems, or nil when it has none.
func (values Values) Check() error {
	if problems := values.Problems(); len(problems) > 0 {
		return fmt.Errorf("%s needs:\n  %s", ValuesFile, strings.Join(problems, "\n  "))
	}
	return nil
}

// endpointProblem says why endpoint is not "[IPv6]:port" or "IPv4:port", or "".
func endpointProblem(endpoint string) string {
	address, err := netip.ParseAddrPort(endpoint)
	if err != nil || address.Port() == 0 {
		return fmt.Sprintf(`vps.endpoint %q: want "[IPv6 address]:51820" or "IPv4 address:51820"`, endpoint)
	}
	return ""
}

// isWireGuardKey reports whether key is a WireGuard key: 32 bytes in base64.
func isWireGuardKey(key string) bool {
	decoded, err := base64.StdEncoding.DecodeString(key)
	return err == nil && len(decoded) == keyLength
}

// quoteTOML renders a string or a list of strings as TOML. A TOML basic string escapes as a Go
// quoted string does for everything a path or a name holds; a control character other than tab, and
// bytes that are not UTF-8, are refused, since TOML and Go escape those differently.
func quoteTOML(value any) (string, error) {
	switch typed := value.(type) {
	case string:
		if !utf8.ValidString(typed) {
			return "", fmt.Errorf("%q is not UTF-8", typed)
		}
		for _, character := range typed {
			if character < 0x20 && character != '\t' || character == 0x7f {
				return "", fmt.Errorf("%q holds a control character", typed)
			}
		}
		return strconv.Quote(typed), nil
	case []string:
		quoted := make([]string, 0, len(typed))
		for _, item := range typed {
			one, err := quoteTOML(item)
			if err != nil {
				return "", err
			}
			quoted = append(quoted, one)
		}
		return "[" + strings.Join(quoted, ", ") + "]", nil
	}
	return "", fmt.Errorf("cannot render %T as TOML", value)
}
