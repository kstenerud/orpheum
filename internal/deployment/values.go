// SPDX-License-Identifier: GPL-3.0-or-later

package deployment

import (
	"encoding/base64"
	"fmt"
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
		PublicKey  string `toml:"public_key"`
		// MTU is 0 when the values file leaves it out: chosen from the endpoint's family.
		MTU int `toml:"mtu"`
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
	} `toml:"http"`
}

// ParseValues reads a values file; a value it leaves out is its starting value, and the node's name
// is hostName.
func ParseValues(text, hostName string) (Values, error) {
	values := Values{}
	values.HTTP.Publish = "0.0.0.0"
	values.HTTP.Port = 8440
	if _, err := toml.Decode(text, &values); err != nil {
		return Values{}, fmt.Errorf("the values file: %w", err)
	}
	values.Domain = strings.TrimSpace(values.Domain)
	values.Contact = strings.TrimSpace(values.Contact)
	values.VPS.Endpoint = strings.TrimSpace(values.VPS.Endpoint)
	values.VPS.PublicIPv4 = strings.TrimSpace(values.VPS.PublicIPv4)
	values.VPS.PublicKey = strings.TrimSpace(values.VPS.PublicKey)
	values.Node.Name = strings.TrimSpace(values.Node.Name)
	if values.Node.Name == "" {
		values.Node.Name = hostName
	}
	return values, nil
}

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

// Problems is what the values file still lacks or holds wrongly, each naming its key.
func (values Values) Problems() []string {
	var problems []string
	if values.Deployment != Release && values.Deployment != Staging {
		problems = append(problems, fmt.Sprintf("deployment %q: want %q or %q", values.Deployment, Release, Staging))
	}
	if values.Domain == "" {
		problems = append(problems, "domain: the public name, whose records point at the VPS")
	}
	if !strings.Contains(values.Contact, "@") {
		problems = append(problems, "contact: the e-mail address Let's Encrypt writes to")
	}
	problems = append(problems, values.vpsProblems()...)
	problems = append(problems, values.mediaProblems()...)
	return append(problems, values.httpProblems()...)
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
	if !isWireGuardKey(values.VPS.PublicKey) {
		problems = append(problems, "vps.public_key: the VPS's WireGuard public key, /etc/wireguard/public on the VPS")
	}
	if values.VPS.MTU != 0 && (values.VPS.MTU < 1280 || values.VPS.MTU > 1500) {
		problems = append(problems, fmt.Sprintf("vps.mtu %d: want 1280 to 1500, or leave it out: %d over IPv6 "+
			"and %d over IPv4 are chosen from the endpoint", values.VPS.MTU, mtuOverIPv6, mtuOverIPv4))
	}
	return problems
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
