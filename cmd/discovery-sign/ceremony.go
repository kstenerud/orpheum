// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/kstenerud/orpheum/discovery/addressdoc"
	"github.com/kstenerud/orpheum/discovery/trust"
	"github.com/kstenerud/orpheum/internal/core/discovery"
)

// guided is one run of a guided command: when it is, where the owner's answers come from and where its
// words go, and the ssh-keygen it uses.
type guided struct {
	now  time.Time
	say  *prompter
	keys keygen
	err  io.Writer
}

// The guided commands, which read answers and run ssh-keygen, and so are not the flag commands of run:
// day one, a key's test, a release's signature, and the changes to a key's standing (guidedchange.go).
func (guide *guided) commands() map[string]func(arguments []string) error {
	commands := map[string]func([]string) error{
		"ceremony":     guide.ceremony,
		"test-key":     guide.testKey,
		"sign-release": guide.signRelease,
	}
	maps.Copy(commands, guide.changeCommands())
	return commands
}

// runGuided runs the guided command arguments name, and says whether it is one.
func runGuided(arguments []string, now time.Time, in io.Reader, stdout, stderr io.Writer, keys keygen) (int, bool) {
	guide := &guided{now: now, say: newPrompter(in, stdout), keys: keys, err: stderr}
	if len(arguments) == 0 || guide.commands()[arguments[0]] == nil {
		return 0, false
	}
	err := guide.commands()[arguments[0]](arguments[1:])
	switch {
	case errors.Is(err, flag.ErrHelp):
		return write(stderr, usage, exitUsage), true
	case err != nil:
		return write(stderr, fmt.Sprintf("discovery-sign: %v\n", err), exitFailure), true
	case guide.say.wrote != nil:
		return exitFailure, true
	}
	return exitSuccess, true
}

// The tiers as the notes and the questions say them.
const (
	roleEveryday = string(trust.RoleEveryday)
	roleRecovery = string(trust.RoleRecovery)
)

// The folder's layout. Nothing in it is secret (D394 §6) but a hardware key's stub, which only its key
// can use and which stubs/ keeps out of a git repository.
const (
	stateFile        = "ceremony.json"
	ledgerDirectory  = "ledger"
	keysDirectory    = "keys"
	stubsDirectory   = "stubs"
	attestDirectory  = "attestations"
	publishDirectory = "publish"
)

// device is a key the owner holds: its name, its tier and what it is.
type device struct {
	Name string `json:"name"`
	Role string `json:"role"`
	Kind string `json:"kind"`
}

// keyEntry is one key made: a device's key for a chain.
type keyEntry struct {
	Chain string `json:"chain"`
	Name  string `json:"name"`
	Role  string `json:"role"`
	Kind  string `json:"kind"`
	// File is the private key, or a hardware key's stub; Pub the public key, relative to the folder.
	File string `json:"file"`
	Pub  string `json:"pub"`
}

// addressAnswers are the address document's fields the owner confirmed.
type addressAnswers struct {
	Issuer        string `json:"issuer"`
	Name          string `json:"name"`
	CoreAPI       string `json:"coreApi"`
	ClientAPI     string `json:"clientApi"`
	LoaderOrigin  string `json:"loaderOrigin"`
	ClientOrigins string `json:"clientOrigins"`
	AccountOrigin string `json:"accountOrigin"`
	Homepage      string `json:"homepage"`
	Support       string `json:"support"`
}

// ceremonyState is what the ceremony remembers in its folder, so it carries on after an interruption.
type ceremonyState struct {
	// Software is the directory plain key files are made in, when the keys are software ones.
	Software string          `json:"software,omitempty"`
	Devices  []device        `json:"devices"`
	Keys     []keyEntry      `json:"keys"`
	Address  *addressAnswers `json:"address,omitempty"`
}

// chainKeys are the keys of chain in role (any role when empty), in the order they were made.
func (state ceremonyState) chainKeys(chain trust.ChainKind, role trust.Role) []keyEntry {
	var found []keyEntry
	for _, key := range state.Keys {
		if key.Chain == string(chain) && (role == "" || key.Role == string(role)) {
			found = append(found, key)
		}
	}
	return found
}

// folder is the owner's records folder: the state, the ledger, the keys' public halves, the
// attestations and what is published.
type folder struct {
	path  string
	state ceremonyState
}

func (place *folder) join(parts ...string) string {
	return filepath.Join(append([]string{place.path}, parts...)...)
}

func (place *folder) ledger() ledger { return ledger(place.join(ledgerDirectory)) }

// openFolder is the folder at path with the state it holds, created if it is not there.
func openFolder(path string) (*folder, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("the folder %s: %w", path, err)
	}
	place := &folder{path: absolute}
	if err = os.MkdirAll(place.path, 0o755); err != nil { //nolint:gosec // the owner's records, nothing secret
		return nil, fmt.Errorf("make the folder %s: %w", path, err)
	}
	contents, err := os.ReadFile(place.join(stateFile))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return place, nil
	case err != nil:
		return nil, fmt.Errorf("read the ceremony's notes: %w", err)
	}
	if err = json.Unmarshal(contents, &place.state); err != nil {
		return nil, fmt.Errorf("read the ceremony's notes %s: %w", place.join(stateFile), err)
	}
	return place, nil
}

// save writes the state.
func (place *folder) save() error {
	contents, err := json.MarshalIndent(place.state, "", "  ")
	if err != nil {
		return err
	}
	//nolint:gosec // nothing secret
	if writeErr := os.WriteFile(place.join(stateFile), append(contents, '\n'), 0o644); writeErr != nil {
		return fmt.Errorf("write the ceremony's notes: %w", writeErr)
	}
	return nil
}

// ceremony is day one (D394 §5): the keys of both chains, each chain's genesis, the address document,
// and the files to publish.
func (guide *guided) ceremony(arguments []string) error {
	flags := flag.NewFlagSet("discovery-sign ceremony", flag.ContinueOnError)
	flags.SetOutput(guide.err)
	where := flags.String("folder", "", "where the records are kept (asked when not given)")
	software := flags.String("software", "", "make plain key files in this directory, with no hardware key")
	if flags.Parse(arguments) != nil || flags.NArg() != 0 {
		return flag.ErrHelp
	}
	guide.say.sayf("Orpheum signing ceremony. It takes about an hour, and you can stop and run it again at any\n" +
		"point: it carries on from where it stopped. You type no commands: I ask, and run ssh-keygen myself.")
	if *where == "" {
		var err error
		if *where, err = guide.say.ask("Which folder keeps your signing records? (Nothing in it is secret.)",
			"orpheum-security"); err != nil {
			return err
		}
	}
	records, err := openFolder(*where)
	if err != nil {
		return err
	}
	if err := records.useSoftware(*software); err != nil {
		return err
	}
	steps := []func(*folder) error{
		guide.planDevices, guide.makeKeys, guide.askAddress,
		func(place *folder) error { return guide.makeGenesis(place, trust.ChainDiscovery) },
		func(place *folder) error { return guide.makeGenesis(place, trust.ChainPublisher) },
		guide.makeAddress, guide.publishFiles, guide.closing,
	}
	for _, step := range steps {
		if err := step(records); err != nil {
			return err
		}
	}
	return guide.say.wrote
}

// useSoftware sets the software keys' directory from the flag, or the notes; it is outside the folder.
func (place *folder) useSoftware(directory string) error {
	if directory == "" {
		return nil
	}
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return fmt.Errorf("the keys directory %s: %w", directory, err)
	}
	if relative, relErr := filepath.Rel(place.path, absolute); relErr == nil && !strings.HasPrefix(relative, "..") {
		return fmt.Errorf("the keys directory %s is inside the records folder %s: private keys must be kept "+
			"outside it, since the folder is kept in a git repository", absolute, place.path)
	}
	if place.state.Software != "" && place.state.Software != absolute {
		return fmt.Errorf("this ceremony began with its keys in %s, not %s", place.state.Software, absolute)
	}
	place.state.Software = absolute
	return nil
}

// planDevices asks what keys the owner has.
func (guide *guided) planDevices(place *folder) error {
	if len(place.state.Devices) > 0 {
		guide.say.sayf("\nCarrying on: %d keys planned, %d made so far.",
			len(place.state.Devices), len(place.state.Keys))
		return nil
	}
	say := guide.say
	say.sayf("\nStep 1: which keys do you have?\n" +
		"Two sets of keys protect Orpheum. EVERYDAY keys sign the routine things (the address document,\n" +
		"each release) so they are carried and used often; RECOVERY keys sign anything and can replace the\n" +
		"others, so they are locked away (a bank, a second site). Two tiers mean a stolen everyday key can\n" +
		"be cancelled by the recovery keys, which a thief never held.")
	if place.state.Software != "" {
		return guide.planSoftware(place)
	}
	say.sayf("Each YubiKey needs a PIN of 8 or more characters first (ykman fido access change-pin), and a\n" +
		"stolen key is useless without it. Put a sticker on each key with the name I give it below.")
	count, err := say.number("How many YubiKeys will you use?", 4, 2)
	if err != nil {
		return err
	}
	for index := range count {
		role, defaultName := roleEveryday, "e"+strconv.Itoa(index+1)
		if index >= 2 {
			role, defaultName = roleRecovery, "r"+strconv.Itoa(index-1)
		}
		name, askErr := say.ask(fmt.Sprintf("Name for YubiKey number %d?", index+1), defaultName)
		if askErr != nil {
			return askErr
		}
		if role, err = say.choose(fmt.Sprintf("Is %s an everyday key or a recovery key?", name),
			[]string{roleEveryday, roleRecovery}, role); err != nil {
			return err
		}
		place.state.Devices = append(place.state.Devices, device{Name: name, Role: role, Kind: kindYubiKey})
	}
	say.sayf("A Mac's Secure Enclave (unlocked by Touch ID) can be one more everyday key, so you carry nothing\n" +
		"extra. Only do this if `discovery-sign test-key` on that Mac said its signatures carry the\n" +
		"user-verified flag (D394 §7); otherwise the Mac would not be as safe as a YubiKey.")
	useMac, err := say.yes("Add this Mac as an everyday key?", false)
	if err != nil {
		return err
	}
	if useMac {
		place.state.Devices = append(place.state.Devices, device{Name: "mac", Role: roleEveryday, Kind: kindMac})
	}
	return guide.checkPlan(place)
}

// planSoftware asks how many software keys to make.
func (guide *guided) planSoftware(place *folder) error {
	guide.say.sayf("These are plain key files in %s, with no hardware key: for staging's own chain or a\n"+
		"self-hoster's (D394 §1). Keep that directory out of every repository, and back it up.", place.state.Software)
	everyday, err := guide.say.number("How many everyday keys?", 1, 1)
	if err != nil {
		return err
	}
	recovery, err := guide.say.number("How many recovery keys?", 1, 1)
	if err != nil {
		return err
	}
	for index := range everyday {
		place.state.Devices = append(place.state.Devices,
			device{Name: "e" + strconv.Itoa(index+1), Role: roleEveryday, Kind: kindSoftware})
	}
	for index := range recovery {
		place.state.Devices = append(place.state.Devices,
			device{Name: "r" + strconv.Itoa(index+1), Role: roleRecovery, Kind: kindSoftware})
	}
	return guide.checkPlan(place)
}

// checkPlan refuses a plan with no key of a tier or two keys of a name, and keeps it.
func (guide *guided) checkPlan(place *folder) error {
	names := map[string]bool{}
	hasRole := map[string]bool{}
	for _, each := range place.state.Devices {
		if names[each.Name] || strings.ContainsAny(each.Name, " /\\") || each.Name == "" {
			place.state.Devices = nil
			return fmt.Errorf("the key name %q is empty, used twice, or has a space or slash: run again", each.Name)
		}
		names[each.Name], hasRole[each.Role] = true, true
	}
	if !hasRole[roleEveryday] || !hasRole[roleRecovery] {
		place.state.Devices = nil
		return errors.New("a chain needs at least one everyday key and one recovery key: run again")
	}
	return place.save()
}

// stubPath is where a key's private half is: a software key in the keys directory outside the folder, a
// hardware key's stub in the folder's stubs directory.
func (place *folder) stubPath(each device, chain trust.ChainKind) string {
	name := string(chain) + "-" + each.Name
	if each.Kind == kindSoftware {
		return filepath.Join(place.state.Software, name)
	}
	return place.join(stubsDirectory, name)
}

// makeKeys makes each device's key for each chain: one key per chain per device, separate namespaces,
// so no key of one chain can stand for the other.
func (guide *guided) makeKeys(place *folder) error {
	guide.say.sayf("\nStep 2: make the keys.\n" +
		"Each device makes a SEPARATE key for each chain (discovery: which servers cores trust; publisher:\n" +
		"which software releases they install), so a signature for one is never valid for the other.\n" +
		"Each key stays inside its device; only its public half, which is safe to publish, leaves.")
	for _, each := range place.state.Devices {
		pending := false
		for _, chain := range []trust.ChainKind{trust.ChainDiscovery, trust.ChainPublisher} {
			pending = pending || !place.hasKey(each, chain)
		}
		if !pending {
			continue
		}
		if each.Kind != kindSoftware {
			plug := fmt.Sprintf("Plug in the key named %q (only that one, and no other security key).", each.Name)
			if err := guide.say.press(plug); err != nil {
				return err
			}
		}
		for _, chain := range []trust.ChainKind{trust.ChainDiscovery, trust.ChainPublisher} {
			if err := guide.makeKey(place, each, chain); err != nil {
				return err
			}
		}
	}
	return nil
}

// hasKey is whether the key of device for chain was made.
func (place *folder) hasKey(each device, chain trust.ChainKind) bool {
	return slices.ContainsFunc(place.state.Keys, func(key keyEntry) bool {
		return key.Chain == string(chain) && key.Name == each.Name
	})
}

// makeKey makes the key of device for chain, keeps its public half in the folder, and notes it.
func (guide *guided) makeKey(place *folder, each device, chain trust.ChainKind) error {
	if place.hasKey(each, chain) {
		return nil
	}
	label := string(chain) + "-" + each.Name
	file := place.stubPath(each, chain)
	spec := keySpec{Kind: each.Kind, File: file, Application: "ssh:orpheum-" + label, Label: label}
	if each.Kind == kindYubiKey {
		spec.Attestation = place.join(attestDirectory, label+".att")
	}
	for _, directory := range []string{filepath.Dir(file), place.join(keysDirectory), place.join(attestDirectory)} {
		//nolint:gosec // nothing secret; the stub's own file is private
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return fmt.Errorf("make %s: %w", directory, err)
		}
	}
	guide.say.sayf("\nMaking the %s key of %q. %s", chain, each.Name, touchHint(each.Kind))
	if err := guide.keys.Make(spec); err != nil {
		return fmt.Errorf("making %s: %w", label, err)
	}
	public, err := os.ReadFile(file + ".pub") //nolint:gosec // as above
	if err != nil {
		return fmt.Errorf("ssh-keygen finished but %s.pub is not there: %w", file, err)
	}
	copied := filepath.Join(keysDirectory, label+".pub")
	if err = os.WriteFile(place.join(copied), public, 0o644); err != nil { //nolint:gosec // a public key
		return fmt.Errorf("keep the public key: %w", err)
	}
	if _, readErr := readKey(place.join(copied)); readErr != nil {
		return readErr
	}
	if spec.Attestation != "" {
		if _, statErr := os.Stat(spec.Attestation); statErr != nil {
			guide.say.sayf("Note: the key wrote no attestation file (%s). It is optional proof the key was\n"+
				"made in the device; the ceremony goes on without it.", spec.Attestation)
		}
	}
	place.state.Keys = append(place.state.Keys, keyEntry{
		Chain: string(chain), Name: each.Name, Role: each.Role, Kind: each.Kind, File: file, Pub: copied,
	})
	return place.save()
}

// touchHint says what the owner does while ssh-keygen works.
func touchHint(kind string) string {
	switch kind {
	case kindYubiKey:
		return "It will ask for the key's PIN, then the key blinks: touch it."
	case kindMac:
		return "It will ask for Touch ID."
	default:
		return "It will ask for a passphrase: choose a long one, or press Enter for none."
	}
}

// askAddress asks for the address document's fields, with the service's own as the defaults (D390).
func (guide *guided) askAddress(place *folder) error {
	if place.state.Address != nil {
		return nil
	}
	guide.say.sayf("\nStep 3: the address document.\n" +
		"This is the signed page that tells a core where to find the service. Because a key signs it, no\n" +
		"one who takes over the website can point cores somewhere else.")
	fallback := ""
	if place.state.Software == "" {
		fallback = discovery.Suggested().Issuer
	}
	typed, err := guide.say.ask("What is the service's address?", fallback)
	if err != nil {
		return err
	}
	issuer, err := addressdoc.Issuer(typed)
	if err != nil {
		return err
	}
	answers := addressDefaults(issuer)
	guide.say.sayf("The document will say:\n%s", answers.describe())
	same, err := guide.say.yes("Use these?", true)
	if err != nil {
		return err
	}
	if !same {
		for _, field := range answers.fields() {
			if *field.value, err = guide.say.ask(field.question, *field.value); err != nil {
				return err
			}
		}
	}
	place.state.Address = &answers
	return place.save()
}

// addressDefaults are the document's fields for issuer: a plain layout under its name, which the owner
// may change. The paid service's own names live in one place, internal/core/discovery/suggested.go (D391 §7).
func addressDefaults(issuer string) addressAnswers {
	host := strings.TrimPrefix(issuer, "https://")
	return addressAnswers{
		Issuer: issuer, Name: host, LoaderOrigin: issuer,
		CoreAPI: issuer + "/core/v1", ClientAPI: issuer + "/client/v1", ClientOrigins: issuer,
	}
}

// addressField is a field of the document, with the question that asks it.
type addressField struct {
	name, question string
	value          *string
}

func (answers *addressAnswers) fields() []addressField {
	return []addressField{
		{"name", "Its name?", &answers.Name},
		{"core-api", "Where cores call it (the core API address)?", &answers.CoreAPI},
		{"client-api", "Where apps and TVs call it (the client API address)?", &answers.ClientAPI},
		{"loader-origin", "Where the web app is served?", &answers.LoaderOrigin},
		{"client-origins", "The pattern of each core's web address ({core} is its id)?", &answers.ClientOrigins},
		{"account-origin", "Where accounts are kept (empty for none)?", &answers.AccountOrigin},
		{"homepage", "Its home page (empty for none)?", &answers.Homepage},
		{"support", "Its support email address (empty for none)?", &answers.Support},
	}
}

// describe lists the fields in words.
func (answers *addressAnswers) describe() string {
	lines := []string{"  address (issuer): " + answers.Issuer}
	for _, field := range answers.fields() {
		if *field.value != "" {
			lines = append(lines, "  "+field.name+": "+*field.value)
		}
	}
	return strings.Join(lines, "\n")
}

// makeGenesis writes, signs with every key of the chain, checks and keeps the chain's first statement.
func (guide *guided) makeGenesis(place *folder, chain trust.ChainKind) error {
	records := place.ledger()
	if _, isHeld, err := records.chain(chain); err != nil || isHeld {
		return err
	}
	guide.say.sayf("\nStep: the %s chain's first statement.\n"+
		"It lists the keys that may sign for this chain. Every key signs it once, which proves that key\n"+
		"and its PIN work before anything depends on it. A signature by a key that is not listed, or\n"+
		"made without the PIN, is refused.", chain)
	arguments := []string{
		"--ledger", string(records), "--chain", string(chain), "--location", place.state.Address.Issuer,
	}
	for _, key := range place.state.chainKeys(chain, "") {
		flagName := "--everyday"
		if key.Role == string(trust.RoleRecovery) {
			flagName = "--recovery"
		}
		arguments = append(arguments, flagName, place.join(key.Pub))
	}
	run := signing{now: guide.now, ledger: records, stdout: io.Discard}
	if _, err := genesis(run, withLedger("genesis"), arguments); err != nil {
		return err
	}
	name := string(chain) + "-root-1"
	path := records.path("pending", name)
	payload, err := os.ReadFile(path) //nolint:gosec // the owner's own ledger
	if err != nil {
		return err
	}
	guide.say.sayf("This is the statement, exactly as it will be signed:\n\n%s", payload)
	signers := place.state.chainKeys(chain, "")
	signed, err := guide.signWith(place, chain.RootNamespace(), path, signers, func(one trust.Signed) error {
		_, genesisErr := trust.Genesis(chain, one)
		return genesisErr
	})
	if err != nil {
		return err
	}
	return guide.accept(run, string(chain), kindRoot, signed)
}

// makeAddress writes the address document, signs it with the first everyday discovery key, and keeps it.
func (guide *guided) makeAddress(place *folder) error {
	newest, err := place.ledger().newest(string(trust.ChainDiscovery), kindAddress)
	if err != nil || newest.signed.Payload != "" {
		return err
	}
	guide.say.sayf("\nStep: sign the address document.\n" +
		"An everyday key signs it, so a core can tell it came from you and was not changed on the way.")
	return guide.signAddress(place, place.state.chainKeys(trust.ChainDiscovery, trust.RoleEveryday)[0])
}

// signAddress writes the next address document from the folder's answers, under the discovery chain's
// current version, has signer sign it, and keeps it.
func (guide *guided) signAddress(place *folder, signer keyEntry) error {
	records := place.ledger()
	chain, isHeld, err := records.chain(trust.ChainDiscovery)
	if err != nil || !isHeld {
		return errors.Join(err, errors.New("the discovery chain is not kept yet"))
	}
	newest, err := records.newest(string(trust.ChainDiscovery), kindAddress)
	if err != nil {
		return err
	}
	sequence := int64(1)
	if newest.signed.Payload != "" && newest.rootVersion == chain.Version() {
		sequence = newest.sequence + 1
	}
	answers := place.state.Address
	arguments := []string{
		"--ledger", string(records), "--issuer", answers.Issuer, "--location", answers.Issuer,
		"--name", answers.Name, "--core-api", answers.CoreAPI, "--client-api", answers.ClientAPI,
		"--loader-origin", answers.LoaderOrigin, "--client-origins", answers.ClientOrigins,
	}
	for name, value := range map[string]string{
		"--account-origin": answers.AccountOrigin, "--homepage": answers.Homepage, "--support": answers.Support,
	} {
		if value != "" {
			arguments = append(arguments, name, value)
		}
	}
	run := signing{now: guide.now, ledger: records, stdout: io.Discard}
	if _, err = address(run, withLedger("address"), arguments); err != nil {
		return err
	}
	path := records.path("pending", fmt.Sprintf("address-%d-%d", chain.Version(), sequence))
	signed, err := guide.signWith(place, trust.NamespaceDiscoveryAddress, path, []keyEntry{signer},
		func(one trust.Signed) error {
			_, openErr := addressdoc.Open(one, chain, answers.Issuer)
			return openErr
		})
	if err != nil {
		return err
	}
	return guide.accept(run, string(trust.ChainDiscovery), kindAddress, signed)
}

// accept checks signed by the code a core uses, as check does, and keeps it in the ledger.
func (guide *guided) accept(run signing, chain, kind string, signed trust.Signed) error {
	var where []string
	var err error
	switch kind {
	case kindRoot:
		where, err = run.checkRoot(trust.ChainKind(chain), signed)
	case kindAddress:
		where, err = run.checkAddress(signed)
	case kindRelease:
		where, err = run.checkRecord(signed)
	default:
		err = fmt.Errorf("nothing checks a %s here", kind)
	}
	if err != nil {
		return fmt.Errorf("the signature did not pass the check a core makes, so nothing was kept: %w", err)
	}
	if err := run.ledger.keep(signed, where...); err != nil {
		return err
	}
	guide.say.sayf("Checked as a core checks it, and kept in the ledger (%s).", filepath.Join(where...))
	return nil
}

// publishFiles writes the files every place serves, and the install's bootstrap.
func (guide *guided) publishFiles(place *folder) error {
	out := place.join(publishDirectory)
	if err := place.ledger().publish(out); err != nil {
		return err
	}
	chain, isHeld, err := place.ledger().chain(trust.ChainPublisher)
	if err != nil || !isHeld {
		return errors.Join(err, errors.New("no publisher chain"))
	}
	script, err := renderBootstrap(chain.Current(), chain.Current().Locations)
	if err != nil {
		return err
	}
	//nolint:gosec // published, read by all
	if err := os.WriteFile(filepath.Join(out, "install"), []byte(script), 0o644); err != nil {
		return err
	}
	guide.say.sayf("\nThe files to publish are in %s (the install script's SHA-256 is %s).",
		out, trust.PayloadHash(script))
	return nil
}

// closing says what the owner still does by hand.
func (guide *guided) closing(place *folder) error {
	records := place.ledger()
	say := guide.say
	say.sayf("\nDone. Both chains are made, signed and checked by the code a core runs.")
	for _, chain := range []trust.ChainKind{trust.ChainDiscovery, trust.ChainPublisher} {
		held, _, err := records.chain(chain)
		if err != nil {
			return err
		}
		say.sayf("  %s chain identity: %s", chain, held.ID())
		for _, fingerprint := range held.Fingerprints() {
			say.sayf("    key %s", fingerprint)
		}
	}
	say.sayf("\nWhat is left for you to do by hand:")
	if place.state.Software != "" {
		say.sayf("1. Copy the contents of %s to the server, so it serves them under /.well-known/. The first\n"+
			"   statement of each chain is at %s/ledger/discovery/root/1 and .../publisher/root/1 with its .sig.\n"+
			"2. Keep the key files in %s out of every repository, and back them up; they are the whole defence.\n"+
			"3. Commit the folder %s to its private repository: nothing in it is secret.",
			place.join(publishDirectory), place.path, place.state.Software, place.path)
		return nil
	}
	sources, err := guide.genesisSources(records)
	if err != nil {
		return err
	}
	say.sayf("1. Put the two first statements into internal/core/discovery/suggested.go, then commit them with\n" +
		"   the identities and fingerprints above in the message. In Suggested(), the Genesis line becomes:\n\n" +
		sources[trust.ChainDiscovery] + "\n\n   and in Publisher():\n\n" + sources[trust.ChainPublisher])
	say.sayf("\n2. Copy the contents of %s to the server, so it serves them under /.well-known/ (the address\n"+
		"   document, both chains, and the install script), then check the watcher sees them.",
		place.join(publishDirectory))
	say.sayf("3. Put each key where it belongs: carried keys with you, the others at home, the recovery keys one\n" +
		"   at a bank and one at a second site. Write each key's fingerprint on paper with it.")
	say.sayf("4. Commit %s to its private repository (git add -A; git commit): the ledger, the public keys and the\n"+
		"   attestations. Nothing in it is secret; what matters is that it is not lost or changed.", place.path)
	return nil
}

// genesisSources are the Go text of each chain's first statement, as suggested.go holds it.
func (guide *guided) genesisSources(records ledger) (map[trust.ChainKind]string, error) {
	texts := map[trust.ChainKind]string{}
	for _, chain := range []trust.ChainKind{trust.ChainDiscovery, trust.ChainPublisher} {
		statements, err := records.signed(string(chain), kindRoot)
		if err != nil || len(statements) == 0 {
			return nil, errors.Join(err, fmt.Errorf("no %s chain", chain))
		}
		first := statements[0]
		for _, each := range statements {
			if root, parseErr := trust.ParseRoot([]byte(each.Payload)); parseErr == nil && root.Version == 1 {
				first = each
			}
		}
		texts[chain] = "Genesis: trust.Signed{\n\tPayload: `" + first.Payload + "`,\n" +
			"\tSignature: `" + first.Signature + "`,\n},"
	}
	return texts, nil
}

// withLedger is a flag set as run makes one for a command, with its --ledger flag.
func withLedger(name string) *flag.FlagSet {
	flags := flag.NewFlagSet("discovery-sign "+name, flag.ContinueOnError)
	flags.String("ledger", "", "the owner's ledger directory (required)")
	return flags
}
