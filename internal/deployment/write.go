// SPDX-License-Identifier: AGPL-3.0-or-later

package deployment

import (
	"bytes"
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha256"
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
	ValuesFile  = "values.toml"
	composeFile = "compose.yaml"
	// The compose file of a deployment with no tunnel (D314 §3), written as composeFile in place of
	// the tunnel's.
	directComposeTemplate = "compose-direct.yaml"
	overrideFile          = "compose.override.yaml"
	environmentFile       = ".env"
	tunnelDir             = "tunnel"
	privateKeyFile        = "tunnel/private"
	publicKeyFile         = "tunnel/public"
	tunnelConfig          = "tunnel/wg0.conf"
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
	// NVIDIA is how Docker hands the machine's NVIDIA GPUs to a container, which the install script
	// found by trying it (D333); NVIDIANone when the machine has none or Docker cannot.
	NVIDIA NVIDIAAccess
	// Directory is the deployment's directory on the machine, which the install script names, or
	// empty when it is not known: what tells two deployments on one machine apart (OutsideIPv6Subnet).
	Directory string
}

// defaultOutsideIPv6Subnet is the outside network's IPv6 subnet for a deployment whose directory
// is not known: the one every deployment had before each took its own.
const defaultOutsideIPv6Subnet = "fd77:5e::/64"

// OutsideIPv6Subnet is the private IPv6 subnet of a deployment's outside network: a /64 of
// fd77:5e::/48 chosen by the hash of its directory, so two deployments on one machine, as a test
// kit's beside production or staging beside a release (deploy/release/README.md §6), each create
// their own network. Every one used fd77:5e::/64 before, and a second could not start (the production NAS,
// 2026-10-23). Nothing outside the machine sees it: Docker translates it on the way out.
func OutsideIPv6Subnet(directory string) string {
	if directory == "" {
		return defaultOutsideIPv6Subnet
	}
	sum := sha256.Sum256([]byte(directory))
	return fmt.Sprintf("fd77:5e:0:%x::/64", uint16(sum[0])<<8|uint16(sum[1]))
}

// NVIDIAAccess is how Docker hands a machine's NVIDIA GPUs to a container, with the driver's own
// libraries, which no image can carry since they must match the host's kernel driver (D333).
type NVIDIAAccess string

const (
	// NVIDIANone gives a container no NVIDIA GPU.
	NVIDIANone NVIDIAAccess = ""
	// NVIDIAByCDI names the GPUs as the NVIDIA container toolkit's CDI spec describes them.
	NVIDIAByCDI NVIDIAAccess = "cdi"
	// NVIDIAByDriver asks Docker's nvidia device driver, which the toolkit's hook provides.
	NVIDIAByDriver NVIDIAAccess = "driver"
)

// ParseNVIDIAAccess is the NVIDIAAccess named name: cdi, driver, or empty for none.
func ParseNVIDIAAccess(name string) (NVIDIAAccess, error) {
	switch access := NVIDIAAccess(name); access {
	case NVIDIANone, NVIDIAByCDI, NVIDIAByDriver:
		return access, nil
	default:
		return NVIDIANone, fmt.Errorf("NVIDIA access %q: want cdi, driver or nothing", name)
	}
}

// nvidiaCDIDevice is every NVIDIA GPU, by the name the toolkit's CDI spec gives them.
const nvidiaCDIDevice = "nvidia.com/gpu=all"

// nvidiaCapabilities are what of the driver an encode needs: CUDA, which NVENC opens its session
// with, the video encoder and decoder, and nvidia-smi's library, which reports the card.
func nvidiaCapabilities() []string { return []string{"gpu", "compute", "video", "utility"} }

// Begin starts a deployment in dir: the values file, from its template, marked as kind, and the
// tunnel's key pair. It returns the tunnel's public key, for the VPS. It refuses a directory that
// already holds a values file.
func Begin(dir string, kind Kind) (string, error) {
	path := filepath.Join(dir, ValuesFile)
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("%s already holds a values file", dir)
	}
	text, err := render("values.toml", StartingValues(kind))
	if err != nil {
		return "", err
	}
	if err = os.WriteFile(path, []byte(text), publicFileMode); err != nil {
		return "", fmt.Errorf("write the values file: %w", err)
	}
	return ensureKeys(dir)
}

// startValuesHeader is the header of a values file a compose file's start writes (D449 §1), in place
// of the template's, which tells the owner to edit the file.
const startValuesHeader = `# The deployment's values (D449): ` + "`orpheum start`" + ` writes this file at every start
# from the compose file's variables, which compose fills into its command line, and the core's and the node's
# configs from it and this image's templates. An edit here is lost at the next start: change the
# variable in the compose file (or the .env beside it) and start it again.
`

// WriteStartValues writes dir's values file from values, over the one there, as a compose file's
// start does each time (D449 §1), its header saying that the compose file is where a change goes. The
// tunnel's key pair is made the first time, as Begin makes it.
func WriteStartValues(dir string, values Values) error {
	text, err := render("values.toml", values)
	if err != nil {
		return err
	}
	_, body, isSplit := strings.Cut(text, "\n\n")
	if !isSplit {
		return errors.New("the values file's template has no header")
	}
	contents := []byte(startValuesHeader + "\n" + body)
	if writeErr := writeFile(filepath.Join(dir, ValuesFile), contents, publicFileMode); writeErr != nil {
		return writeErr
	}
	_, err = ensureKeys(dir)
	return err
}

// Write writes a deployment's files into dir from its values and its runtime: the compose file,
// the .env file it reads, the override giving the node the media and the GPU, the tunnel's config,
// and the core's and the node's full configs, whose instances must already exist. It overwrites
// each, so they always follow the values file and this binary's templates (D272 §7).
func Write(dir string, values Values, runtime Runtime) error {
	if err := WriteConfigs(dir, values, Installation{Route: ByInstallScript}); err != nil {
		return err
	}
	composeTemplate := directComposeTemplate
	if values.HasTunnel() {
		composeTemplate = composeFile
	}
	compose, err := templates.ReadFile("templates/" + composeTemplate)
	if err != nil {
		return fmt.Errorf("read the compose file's template: %w", err)
	}
	override, err := mediaOverride(values.Media.Roots, runtime.RenderGID, runtime.NVIDIA)
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

// WriteConfigs writes the configs of a deployment's instances into dir from its values: the core's
// and the node's, and the tunnel's when it has one. The instances must already exist. Write writes
// them so, beside the compose files; a first start (D447), whose compose file is the household's own,
// writes them alone. The core's names how the deployment was installed: its route (D450 §2) and its
// compose file's version (D461 §3).
func WriteConfigs(dir string, values Values, installation Installation) error {
	if err := values.Check(); err != nil {
		return err
	}
	data := renderData{
		Values: values, IsStaging: values.Deployment == Staging, InstallRoute: string(installation.Route),
		ComposeVersion: installation.ComposeVersion,
	}
	files := []renderedFile{{coreConfig, "core.toml", privateFileMode}, {nodeConfig, "node.toml", privateFileMode}}
	if values.HasTunnel() {
		privateKey, err := tunnelPrivateKey(dir)
		if err != nil {
			return err
		}
		data.PrivateKey = privateKey
		files = append(files, renderedFile{tunnelConfig, "wg0.conf", privateFileMode})
	}
	for _, file := range files {
		text, err := render(file.template, data)
		if err != nil {
			return err
		}
		if writeErr := writeFile(filepath.Join(dir, file.name), []byte(text), file.mode); writeErr != nil {
			return writeErr
		}
	}
	return nil
}

// InstallScript is the script a household installs a core with (D314), installing image: `orpheum
// install-script` prints it, so it comes from the image it installs.
func InstallScript(image string) (string, error) {
	return render("install.sh", struct{ Image string }{image})
}

// renderedFile is a file Write renders from a template into the deployment's directory.
type renderedFile struct {
	name, template string
	mode           os.FileMode
}

// tunnelPrivateKey is the tunnel's private key, made the first time.
func tunnelPrivateKey(dir string) (string, error) {
	if _, err := ensureKeys(dir); err != nil {
		return "", err
	}
	private, err := os.ReadFile(filepath.Join(dir, privateKeyFile)) //nolint:gosec // G304: the deployment's own
	if err != nil {
		return "", fmt.Errorf("read the tunnel's private key: %w", err)
	}
	return strings.TrimSpace(string(private)), nil
}

// renderData is what the config templates are rendered from.
type renderData struct {
	Values     Values
	IsStaging  bool
	PrivateKey string
	// InstallRoute is how the deployment was installed, for the core's config (D450 §2).
	InstallRoute string
	// ComposeVersion is the version of the compose file that started it, 0 for the install script's
	// (D461 §3).
	ComposeVersion int
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
		"ORPHEUM_THIS_MACHINE_PORT=" + strconv.Itoa(values.HTTP.ThisMachinePort),
	}
	// With no tunnel the core shares this machine's network and publishes nothing (D454 §5): its
	// ports are its config's.
	if values.HasTunnel() {
		lines = append(lines,
			"ORPHEUM_HTTP_PUBLISH="+values.HTTP.Publish,
			"ORPHEUM_HTTP_PORT="+strconv.Itoa(values.HTTP.Port),
			"ORPHEUM_LAN_HTTPS_PORT="+strconv.Itoa(values.LAN.HTTPSPort),
			"ORPHEUM_LAN_WEBRTC_PORT="+strconv.Itoa(values.LAN.WebRTCPort),
			"ORPHEUM_OUTSIDE_IPV6_SUBNET="+OutsideIPv6Subnet(runtime.Directory),
			"ORPHEUM_TUNNEL_MTU="+strconv.Itoa(values.TunnelMTU()),
			"ORPHEUM_TUNNEL_IPV4="+values.VPS.TunnelIPv4,
			"ORPHEUM_TUNNEL_IPV6="+values.VPS.TunnelIPv6,
		)
	}
	return strings.Join(lines, "\n") + "\n"
}

// mediaMount is a media folder in compose's long volume syntax, the one that can refuse a missing
// folder.
type mediaMount struct {
	Type     string    `json:"type"`
	Source   string    `json:"source"`
	Target   string    `json:"target"`
	ReadOnly bool      `json:"read_only"` //nolint:tagliatelle // compose's own key
	Bind     mediaBind `json:"bind"`
}

// mediaBind is a media mount's bind options: never make the folder on the host.
type mediaBind struct {
	CreateHostPath bool `json:"create_host_path"` //nolint:tagliatelle // compose's own key
}

// mediaOverride is the compose override that gives the node the media, read-only at their own
// paths, the GPU when the machine has one (renderGID >= 0), and its NVIDIA GPUs as Docker hands them
// over (D333): what differs from machine to machine. Compose reads compose.override.yaml beside
// compose.yaml by itself. A media folder that does not exist stops the node's start with its path,
// rather than Docker making it, empty and root's.
func mediaOverride(roots []string, renderGID int, nvidia NVIDIAAccess) ([]byte, error) {
	volumes := make([]mediaMount, 0, len(roots))
	for _, root := range roots {
		volumes = append(volumes, mediaMount{
			Type: "bind", Source: root, Target: root, ReadOnly: true, Bind: mediaBind{CreateHostPath: false},
		})
	}
	node := map[string]any{"volumes": volumes}
	var devices []string
	if renderGID >= 0 {
		devices = append(devices, "/dev/dri:/dev/dri")
		node["group_add"] = []string{strconv.Itoa(renderGID)}
	}
	switch nvidia {
	case NVIDIAByCDI:
		devices = append(devices, nvidiaCDIDevice)
	case NVIDIAByDriver:
		node["deploy"] = map[string]any{"resources": map[string]any{"reservations": map[string]any{
			"devices": []map[string]any{{"driver": "nvidia", "count": "all", "capabilities": nvidiaCapabilities()}},
		}}}
	case NVIDIANone:
	}
	if devices != nil {
		node["devices"] = devices
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
