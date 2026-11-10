// SPDX-License-Identifier: AGPL-3.0-or-later

package deployment

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// NetworkFile is the network settings the System page keeps, in the core's own persistent folder,
// which it may write (D462 §2).
const NetworkFile = "network.toml"

// NetworkPath is the network settings' file in the deployment's directory dir.
func NetworkPath(dir string) string { return filepath.Join(dir, "core", "persistent", NetworkFile) }

// Network are the settings of the house's network a deployment is written with that are the app's,
// not the compose file's (D462 §2): the System page sets them, and every start, and the install
// script's every run, writes them into the core's config.
type Network struct {
	// HTTPSPort is where devices at home reach the core over HTTPS by its LAN name.
	HTTPSPort int `toml:"https_port"`
	// WebRTCPort is the UDP port WebRTC is taken on at home.
	WebRTCPort int `toml:"webrtc_port"`
	// Router is the house's router, which port mapping asks to forward the HTTPS port; empty maps
	// nothing.
	Router string `toml:"router"`
}

// NetworkOf is the network settings values name.
func NetworkOf(values Values) Network {
	return Network{HTTPSPort: values.LAN.HTTPSPort, WebRTCPort: values.LAN.WebRTCPort, Router: values.LAN.Router}
}

// Applied is values with the network's settings in place of their own.
func (network Network) Applied(values Values) Values {
	values.LAN.HTTPSPort, values.LAN.WebRTCPort, values.LAN.Router = network.HTTPSPort, network.WebRTCPort,
		network.Router
	return values
}

// Problems are what is wrong with the settings, each naming its key, for a core taking plain HTTP on
// httpPort (0 for unknown): none for good ones.
func (network Network) Problems(httpPort int) []string {
	values := StartingValues(Release)
	values.HTTP.Port = httpPort
	return network.Applied(values).lanProblems()
}

// ErrNoNetwork is ReadNetwork's when no network settings are kept yet.
var ErrNoNetwork = errors.New("no network settings are kept yet")

// ReadNetwork is the network settings kept at path, ErrNoNetwork when there are none.
func ReadNetwork(path string) (Network, error) {
	text, err := os.ReadFile(path) //nolint:gosec // G304: the deployment's own file
	if errors.Is(err, fs.ErrNotExist) {
		return Network{}, ErrNoNetwork
	}
	if err != nil {
		return Network{}, fmt.Errorf("read the network settings: %w", err)
	}
	var network Network
	if _, err = toml.Decode(string(text), &network); err != nil {
		return Network{}, fmt.Errorf("the network settings in %s: %w", path, err)
	}
	return network, nil
}

// networkHeader is the network settings file's header.
const networkHeader = `# The house's network settings the server's System page keeps (D462 §2): every start writes
# them into the core's config. Change them on that page, which checks them and restarts the server.
`

// WriteNetwork keeps network at path, refusing settings with a problem.
func WriteNetwork(path string, network Network) error {
	if problems := network.Problems(0); len(problems) != 0 {
		return fmt.Errorf("the network settings: %v", problems)
	}
	var text bytes.Buffer
	text.WriteString(networkHeader)
	if err := toml.NewEncoder(&text).Encode(network); err != nil {
		return fmt.Errorf("encode the network settings: %w", err)
	}
	return writeFile(path, text.Bytes(), privateFileMode)
}

// KeptNetwork is the deployment's network settings: the ones kept in dir, else seed, kept from now
// on, so the values file a setting came from is read for it once (D462 §10.6).
func KeptNetwork(dir string, seed Network) (Network, error) {
	path := NetworkPath(dir)
	network, err := ReadNetwork(path)
	if errors.Is(err, ErrNoNetwork) {
		return seed, WriteNetwork(path, seed)
	}
	return network, err
}
