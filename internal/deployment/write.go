// SPDX-License-Identifier: AGPL-3.0-or-later

package deployment

import (
	"bytes"
	"crypto/ecdh"
	"crypto/rand"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
)

//go:embed templates
var templates embed.FS

// The files of a deployment's directory (D272 §3).
const (
	ValuesFile      = "values.toml"
	composeFile     = "compose.yaml"
	overrideFile    = "compose.override.yaml"
	environmentFile = ".env"
	tunnelDir       = "tunnel"
	privateKeyFile  = "tunnel/private"
	publicKeyFile   = "tunnel/public"
	tunnelConfig    = "tunnel/wg0.conf"
	// The instances' full configs, which the instances' own directories hold (D242 §3).
	coreConfig = "core/persistent/config.toml"
	nodeConfig = "node/persistent/config.toml"
)

// keyLength is a WireGuard key's length in bytes: an X25519 key.
const keyLength = 32

// privateFileMode is the mode of a file holding a key, or a config naming one.
const privateFileMode = 0o600

// publicFileMode is the mode of every other file the deployment's directory holds.
const publicFileMode = 0o644

// Runtime is what the deployment's files say that the values file does not: the image to run, who
// owns the directory and so runs the services, and the group of this machine's render node.
type Runtime struct {
	// Image is the runtime image the services run: a tag that names its version, or an image ID.
	Image string
	// UID and GID own the deployment's directory; the core and the node run as them.
	UID, GID int
	// RenderGID is the group of the GPU's render node, or -1 when the machine has none.
	RenderGID int
}

// Begin starts a deployment in dir: the values file, from its template, marked as kind, and the
// tunnel's key pair. It returns the tunnel's public key, for the VPS. It refuses a directory that
// already holds a values file.
func Begin(dir string, kind Kind) (string, error) {
	path := filepath.Join(dir, ValuesFile)
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("%s already holds a values file", dir)
	}
	text, err := render("values.toml", struct{ Deployment Kind }{kind})
	if err != nil {
		return "", err
	}
	if err = os.WriteFile(path, []byte(text), publicFileMode); err != nil {
		return "", fmt.Errorf("write the values file: %w", err)
	}
	return ensureKeys(dir)
}

// Write writes a deployment's files into dir from its values and its runtime: the compose file,
// the .env file it reads, the override giving the node the media and the GPU, the tunnel's config,
// and the core's and the node's full configs, whose instances must already exist. It overwrites
// each, so they always follow the values file and this binary's templates (D272 §7).
func Write(dir string, values Values, runtime Runtime) error {
	if err := values.Check(); err != nil {
		return err
	}
	if _, err := ensureKeys(dir); err != nil {
		return err
	}
	private, err := os.ReadFile(filepath.Join(dir, privateKeyFile)) //nolint:gosec // G304: the deployment's own
	if err != nil {
		return fmt.Errorf("read the tunnel's private key: %w", err)
	}
	data := renderData{
		Values: values, IsStaging: values.Deployment == Staging,
		PrivateKey: strings.TrimSpace(string(private)),
	}
	compose, err := templates.ReadFile("templates/" + composeFile)
	if err != nil {
		return fmt.Errorf("read the compose file's template: %w", err)
	}
	files := []struct {
		name, template string
		mode           os.FileMode
	}{
		{coreConfig, "core.toml", privateFileMode},
		{nodeConfig, "node.toml", privateFileMode},
		{tunnelConfig, "wg0.conf", privateFileMode},
	}
	for _, file := range files {
		text, renderErr := render(file.template, data)
		if renderErr != nil {
			return renderErr
		}
		if writeErr := writeFile(filepath.Join(dir, file.name), []byte(text), file.mode); writeErr != nil {
			return writeErr
		}
	}
	override, err := mediaOverride(values.Media.Roots, runtime.RenderGID)
	if err != nil {
		return err
	}
	for name, contents := range map[string][]byte{
		composeFile:     compose,
		overrideFile:    override,
		environmentFile: []byte(environment(values, runtime)),
	} {
		if writeErr := writeFile(filepath.Join(dir, name), contents, publicFileMode); writeErr != nil {
			return writeErr
		}
	}
	return nil
}

// renderData is what the config templates are rendered from.
type renderData struct {
	Values     Values
	IsStaging  bool
	PrivateKey string
}

// render renders the named template; a value it names that data lacks is an error, not a blank.
func render(name string, data any) (string, error) {
	text, err := templates.ReadFile("templates/" + name)
	if err != nil {
		return "", fmt.Errorf("read the template %s: %w", name, err)
	}
	parsed, err := template.New(name).Option("missingkey=error").
		Funcs(template.FuncMap{"toml": quoteTOML}).Parse(string(text))
	if err != nil {
		return "", fmt.Errorf("parse the template %s: %w", name, err)
	}
	var out bytes.Buffer
	if err = parsed.Execute(&out, data); err != nil {
		return "", fmt.Errorf("render %s: %w", name, err)
	}
	return out.String(), nil
}

// environment is the .env file the compose file reads its variables from. Each value is one
// token: an image reference, a number or an address.
func environment(values Values, runtime Runtime) string {
	project := "orpheum"
	if values.Deployment == Staging {
		project = "orpheum-staging"
	}
	lines := []string{
		"# Written by `orpheum write-deployment` from values.toml and this machine; an edit here is lost.",
		"COMPOSE_PROJECT_NAME=" + project,
		"ORPHEUM_IMAGE=" + runtime.Image,
		"ORPHEUM_UID=" + strconv.Itoa(runtime.UID),
		"ORPHEUM_GID=" + strconv.Itoa(runtime.GID),
		"ORPHEUM_HTTP_PUBLISH=" + values.HTTP.Publish,
		"ORPHEUM_HTTP_PORT=" + strconv.Itoa(values.HTTP.Port),
		"ORPHEUM_TUNNEL_MTU=" + strconv.Itoa(values.TunnelMTU()),
	}
	return strings.Join(lines, "\n") + "\n"
}

// mediaOverride is the compose override that gives the node the media, read-only at their own
// paths, and the GPU when the machine has one (renderGID >= 0): what differs from machine to
// machine. Compose reads compose.override.yaml beside compose.yaml by itself.
func mediaOverride(roots []string, renderGID int) ([]byte, error) {
	volumes := make([]string, 0, len(roots))
	for _, root := range roots {
		volumes = append(volumes, root+":"+root+":ro")
	}
	node := map[string]any{"volumes": volumes}
	if renderGID >= 0 {
		node["devices"] = []string{"/dev/dri:/dev/dri"}
		node["group_add"] = []string{strconv.Itoa(renderGID)}
	}
	// JSON is YAML; written as JSON, no path needs quoting by hand.
	text, err := json.MarshalIndent(map[string]any{"services": map[string]any{"orpheum-node": node}}, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("render the media override: %w", err)
	}
	return append(text, '\n'), nil
}

// ensureKeys makes the tunnel's key pair the first time, as `wg genkey` and `wg pubkey` would, and
// returns the public key.
func ensureKeys(dir string) (string, error) {
	private, public := filepath.Join(dir, privateKeyFile), filepath.Join(dir, publicKeyFile)
	if text, err := os.ReadFile(public); err == nil { //nolint:gosec // G304: the deployment's own
		return strings.TrimSpace(string(text)), nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("read the tunnel's public key: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, tunnelDir), 0o700); err != nil {
		return "", fmt.Errorf("make the tunnel's directory: %w", err)
	}
	privateKey, publicKey, err := newKeyPair()
	if err != nil {
		return "", err
	}
	if writeErr := writeFile(private, []byte(privateKey+"\n"), privateFileMode); writeErr != nil {
		return "", writeErr
	}
	return publicKey, writeFile(public, []byte(publicKey+"\n"), publicFileMode)
}

// newKeyPair is a WireGuard key pair in base64: a clamped X25519 private key, as `wg genkey` writes
// one, and its public key.
func newKeyPair() (privateKey, publicKey string, err error) {
	secret := make([]byte, keyLength)
	if _, err = rand.Read(secret); err != nil {
		return "", "", fmt.Errorf("make the tunnel's key: %w", err)
	}
	secret[0] &= 248
	secret[31] = secret[31]&127 | 64
	key, err := ecdh.X25519().NewPrivateKey(secret)
	if err != nil {
		return "", "", fmt.Errorf("make the tunnel's key: %w", err)
	}
	return base64.StdEncoding.EncodeToString(secret), base64.StdEncoding.EncodeToString(key.PublicKey().Bytes()), nil
}

// writeFile writes contents to path with mode, the mode set even when the file was there before.
func writeFile(path string, contents []byte, mode os.FileMode) error {
	if err := os.WriteFile(path, contents, mode); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := os.Chmod(path, mode); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
