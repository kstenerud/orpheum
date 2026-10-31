// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/kstenerud/orpheum/discovery/addressdoc"
	"github.com/kstenerud/orpheum/discovery/release"
	"github.com/kstenerud/orpheum/discovery/trust"
)

// documentLasts is how long an address document is current unless --lasts says: a year, re-signed at
// the yearly drill (the audit's §4 step 12).
const documentLasts = 365 * 24 * time.Hour

// The kinds of object the tool checks, and the ledger's directories they are kept in.
const (
	kindRoot    = "root"
	kindAddress = "address"
	kindRelease = "release"
	kindDrill   = "drill"
)

// signing is one run: when it is, its ledger, and where it says what it did.
type signing struct {
	now    time.Time
	ledger ledger
	stdout io.Writer
}

// command is one of the tool's commands: its flags added to flags, it runs with arguments and says
// whether they were a usage error.
type command func(run signing, flags *flag.FlagSet, arguments []string) (bool, error)

// run does what its arguments name and returns the exit status.
func run(arguments []string, now time.Time, stdout, stderr io.Writer) int {
	commands := map[string]command{
		"genesis": genesis, kindRoot: nextRoot, kindAddress: address, kindDrill: drill, "check": check,
		"publish":   publish,
		"bootstrap": bootstrap,
	}
	if len(arguments) == 0 || commands[arguments[0]] == nil {
		return write(stderr, usage, exitUsage)
	}
	flags := flag.NewFlagSet("discovery-sign "+arguments[0], flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.String("ledger", "", "the owner's ledger directory (required)")
	isUsage, err := commands[arguments[0]](signing{now: now, stdout: stdout}, flags, arguments[1:])
	switch {
	case isUsage:
		return write(stderr, usage, exitUsage)
	case err != nil:
		return write(stderr, fmt.Sprintf("discovery-sign: %v\n", err), exitFailure)
	}
	return exitSuccess
}

// parsed parses arguments into flags and sets run's ledger; false is a usage error.
func parsed(run *signing, flags *flag.FlagSet, arguments []string) bool {
	if flags.Parse(arguments) != nil || flags.NArg() != 0 {
		return false
	}
	dir := flags.Lookup("ledger").Value.String()
	run.ledger = ledger(dir)
	return dir != ""
}

// genesis writes a chain's first statement: its keys in their roles, thresholds 1, unsigned.
func genesis(run signing, flags *flag.FlagSet, arguments []string) (bool, error) {
	kind := flags.String("chain", "", "discovery or publisher (required)")
	var everyday, recovery, locations []string
	flags.Func("everyday", "an everyday key's .pub file; repeated", appendTo(&everyday))
	flags.Func("recovery", "a recovery key's .pub file; repeated", appendTo(&recovery))
	flags.Func("location", "where the chain is served; repeated", appendTo(&locations))
	if !parsed(&run, flags, arguments) || len(everyday) == 0 || len(recovery) == 0 || !isChain(*kind) {
		return true, nil
	}
	if _, isHeld, err := run.ledger.chain(trust.ChainKind(*kind)); err != nil || isHeld {
		return false, errors.Join(err, errHasGenesis)
	}
	root := trust.Root{
		Chain: trust.ChainKind(*kind), Version: 1, Issued: run.now, Locations: locations,
		Thresholds: map[trust.Role]int{trust.RoleEveryday: 1, trust.RoleRecovery: 1},
	}
	if err := addKeys(&root, trust.RoleRecovery, recovery); err != nil {
		return false, err
	}
	if err := addKeys(&root, trust.RoleEveryday, everyday); err != nil {
		return false, err
	}
	payload, err := root.Payload()
	if err != nil {
		return false, err
	}
	return false, run.pending(*kind+"-root-1", root.Chain.RootNamespace(), payload, "",
		"any key of the set (every key, to prove each works and its PIN is known)")
}

// errHasGenesis is a genesis for a chain the ledger holds already.
var errHasGenesis = errors.New("the ledger holds this chain already: a new genesis is a new identity, " +
	"for when every key is lost")

// nextRoot writes the next root statement from the ledger's chain, with the change its flags name.
func nextRoot(run signing, flags *flag.FlagSet, arguments []string) (bool, error) {
	kind := flags.String("chain", "", "discovery or publisher (required)")
	var change rootChange
	flags.Int64Var(&change.skip, "skip", 1,
		"versions to move on: more than 1 only a recovery key signs, past a thief's statements")
	flags.Func("add-everyday", "an everyday key's .pub file to add; repeated", appendTo(&change.addEveryday))
	flags.Func("add-recovery", "a recovery key's .pub file to add; repeated", appendTo(&change.addRecovery))
	flags.Func("remove", "a key's .pub file, or its SHA256: fingerprint, to remove; repeated",
		appendTo(&change.removed))
	flags.Func("suspend", "a key's .pub file, or its SHA256: fingerprint, to suspend; repeated",
		appendTo(&change.suspended))
	flags.Func("resume", "a suspended key's .pub file, or its SHA256: fingerprint, to resume; repeated",
		appendTo(&change.resumed))
	flags.Func("threshold", "<role>=<n>; repeated", appendTo(&change.thresholds))
	flags.Func("location", "where the chain is served, in place of those it names; repeated",
		appendTo(&change.locations))
	if !parsed(&run, flags, arguments) || !isChain(*kind) || change.skip < 1 {
		return true, nil
	}
	chain, isHeld, err := run.ledger.chain(trust.ChainKind(*kind))
	if err != nil || !isHeld {
		return false, errors.Join(err, errors.New("the ledger holds no such chain: make its genesis first"))
	}
	next, isRecovery, err := change.next(chain, run.now)
	if err != nil {
		return false, err
	}
	payload, err := next.Payload()
	if err != nil {
		return false, err
	}
	signers := "an everyday or a recovery key of the current set"
	if isRecovery {
		signers = "a recovery key of the current set"
	}
	return false, run.pending(rootName(chain.Kind(), next.Version), chain.Kind().RootNamespace(), payload,
		currentPayload(chain),
		signers+"; if that key leaves the set or is suspended, a key of the new set of the same tier signs too")
}

// rootChange is a change to a chain's keys: keys added and removed, every key removed named removed for
// good, keys suspended and resumed (D394 §4), thresholds and locations changed, versions skipped (§3.6).
// Keys are named by .pub file or SHA256: fingerprint.
type rootChange struct {
	skip                        int64
	addEveryday, addRecovery    []string
	removed, suspended, resumed []string
	thresholds, locations       []string
}

// next is the statement after chain's current one with change made, issued at now, and whether only a
// recovery key may sign it.
func (change rootChange) next(chain trust.Chain, now time.Time) (trust.Root, bool, error) {
	skip := max(change.skip, 1)
	current := chain.Current()
	next := current
	next.ID, next.Version, next.Issued = chain.ID(), current.Version+skip, now
	next.Keys, next.Removed = slices.Clone(current.Keys), slices.Clone(current.Removed)
	next.Suspended, next.Thresholds = slices.Clone(current.Suspended), maps.Clone(current.Thresholds)
	isRecovery, err := changeStanding(&next, change.removed, change.suspended, change.resumed)
	if err != nil {
		return trust.Root{}, false, err
	}
	isRecovery = isRecovery || skip > 1 || len(change.addRecovery) > 0 || len(change.thresholds) > 0
	if changeErr := errors.Join(addKeys(&next, trust.RoleRecovery, change.addRecovery),
		addKeys(&next, trust.RoleEveryday, change.addEveryday),
		setThresholds(&next, change.thresholds)); changeErr != nil {
		return trust.Root{}, false, changeErr
	}
	if len(change.locations) > 0 {
		next.Locations = change.locations
	}
	return next, isRecovery, nil
}

// rootName is the ledger's name for chain's statement at version.
func rootName(chain trust.ChainKind, version int64) string {
	return fmt.Sprintf("%s-root-%d", chain, version)
}

// currentPayload is the payload of chain's current statement.
func currentPayload(chain trust.Chain) string {
	statements := chain.Statements()
	return statements[len(statements)-1].Payload
}

// address writes the next address document, under the discovery chain's version, its sequence one
// more than the ledger's newest.
func address(run signing, flags *flag.FlagSet, arguments []string) (bool, error) {
	var document addressdoc.Document
	lasts := flags.Duration("lasts", documentLasts, "how long it is current")
	for name, value := range map[string]*string{
		"issuer": &document.Issuer, "name": &document.Name, "core-api": &document.CoreAPI,
		"client-api": &document.ClientAPI, "loader-origin": &document.LoaderOrigin,
		"client-origins": &document.ClientOrigins, "account-origin": &document.AccountOrigin,
		"homepage": &document.Homepage, "support": &document.Support,
	} {
		flags.StringVar(value, name, "", "the document's "+name)
	}
	flags.Func("location", "where the document and chain are served; repeated", appendTo(&document.Locations))
	if !parsed(&run, flags, arguments) || document.Issuer == "" {
		return true, nil
	}
	chain, isHeld, err := run.ledger.chain(trust.ChainDiscovery)
	if err != nil || !isHeld {
		return false, errors.Join(err, errors.New("the ledger holds no discovery chain"))
	}
	previous, err := run.ledger.newest(string(trust.ChainDiscovery), kindAddress)
	if err != nil {
		return false, err
	}
	sequence := previous.sequence
	if previous.rootVersion != chain.Version() {
		sequence = 0
	}
	document.ID, document.RootVersion, document.Sequence = chain.ID(), chain.Version(), sequence+1
	document.Issued, document.Expires = run.now, run.now.Add(*lasts)
	if document.Issuer, err = addressdoc.Issuer(document.Issuer); err != nil {
		return false, err
	}
	if checkErr := document.Check(document.Issuer); checkErr != nil {
		return false, checkErr
	}
	payload, err := document.Payload()
	if err != nil {
		return false, err
	}
	return false, run.pending(fmt.Sprintf("address-%d-%d", document.RootVersion, document.Sequence),
		trust.NamespaceDiscoveryAddress, payload, previous.signed.Payload,
		"an everyday or a recovery key of the discovery chain")
}

// drill writes a drill statement, which a key signs once a year to prove it and its PIN still work;
// nothing accepts it (the audit's §4 steps 8 and 12).
func drill(run signing, flags *flag.FlagSet, arguments []string) (bool, error) {
	kind := flags.String("chain", "", "the chain whose keys are drilled")
	if !parsed(&run, flags, arguments) || !isChain(*kind) {
		return true, nil
	}
	payload := []byte("orpheum-signed drill 1\nchain " + *kind + "\nissued " + trust.TimeText(run.now) + "\n")
	return false, run.pending("drill-"+strconv.FormatInt(run.now.Unix(), 10), trust.NamespaceDrill, payload, "",
		"each key of the set in turn")
}

// check checks a payload's signature by the code a core uses, keeps both in the ledger, and publishes.
func check(run signing, flags *flag.FlagSet, arguments []string) (bool, error) {
	kind := flags.String("kind", "", "root, address, release or drill (required)")
	chainKind := flags.String("chain", "discovery", "the chain, for root and drill")
	payloadFile := flags.String("payload", "", "the payload signed; its signature beside it, .sig (required)")
	out := flags.String("publish", "", "the directory the published files are written to")
	if !parsed(&run, flags, arguments) || *payloadFile == "" || !isChain(*chainKind) {
		return true, nil
	}
	signed, err := readSigned(*payloadFile)
	if err != nil {
		return false, err
	}
	var where []string
	switch *kind {
	case kindRoot:
		where, err = run.checkRoot(trust.ChainKind(*chainKind), signed)
	case kindAddress:
		where, err = run.checkAddress(signed)
	case kindRelease:
		where, err = run.checkRecord(signed)
	case kindDrill:
		where, err = run.checkDrill(trust.ChainKind(*chainKind), signed)
	default:
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if keepErr := run.ledger.keep(signed, where...); keepErr != nil {
		return false, keepErr
	}
	fmt.Fprintf(run.stdout, "checked and kept: %s\n", filepath.Join(where...)) //nolint:errcheck // a terminal
	if *out == "" {
		return false, nil
	}
	if publishErr := run.ledger.publish(*out); publishErr != nil {
		return false, publishErr
	}
	_, err = fmt.Fprintf(run.stdout, "published to %s: copy it to every place, then check the watcher\n", *out)
	return false, err
}

// checkRoot is where a root statement is kept, once the ledger's chain takes it (or it is a genesis).
func (run signing) checkRoot(kind trust.ChainKind, signed trust.Signed) ([]string, error) {
	root, err := trust.ParseRoot([]byte(signed.Payload))
	if err != nil {
		return nil, err
	}
	chain, isHeld, err := run.ledger.chain(kind)
	switch {
	case err != nil:
		return nil, err
	case !isHeld:
		_, err = trust.Genesis(kind, signed)
	default:
		_, err = chain.Accept(signed)
	}
	if err != nil {
		return nil, err
	}
	return []string{string(kind), kindRoot, strconv.FormatInt(root.Version, 10)}, nil
}

// checkAddress is where an address document is kept, once it holds under the discovery chain and is
// newer than the ledger's newest.
func (run signing) checkAddress(signed trust.Signed) ([]string, error) {
	chain, isHeld, err := run.ledger.chain(trust.ChainDiscovery)
	if err != nil || !isHeld {
		return nil, errors.Join(err, errors.New("the ledger holds no discovery chain"))
	}
	parsed, err := addressdoc.Parse([]byte(signed.Payload))
	if err != nil {
		return nil, err
	}
	document, err := addressdoc.Open(signed, chain, parsed.Issuer)
	if err != nil {
		return nil, err
	}
	previous, err := run.ledger.newest(string(trust.ChainDiscovery), kindAddress)
	if err != nil {
		return nil, err
	}
	if !document.IsNewerThan(addressdoc.Document{RootVersion: previous.rootVersion, Sequence: previous.sequence}) {
		return nil, errors.New("the document is not newer than the ledger's newest: cores would not take it")
	}
	return []string{
		string(trust.ChainDiscovery), kindAddress,
		fmt.Sprintf("%d-%d", document.RootVersion, document.Sequence),
	}, nil
}

// checkRecord is where a release record is kept, once it holds under the publisher chain, is newer
// than the ledger's newest, and changes no released version's digest (the audit's F11).
func (run signing) checkRecord(signed trust.Signed) ([]string, error) {
	chain, isHeld, err := run.ledger.chain(trust.ChainPublisher)
	if err != nil || !isHeld {
		return nil, errors.Join(err, errors.New("the ledger holds no publisher chain"))
	}
	record, err := release.Open(signed, chain)
	if err != nil {
		return nil, err
	}
	previous, err := run.ledger.newest(string(trust.ChainPublisher), kindRelease)
	if err != nil {
		return nil, err
	}
	if previous.signed.Payload != "" {
		before, parseErr := release.Parse([]byte(previous.signed.Payload))
		if parseErr != nil {
			return nil, parseErr
		}
		if tag, isChanged := record.ChangedDigest(before.Images); isChanged {
			return nil, fmt.Errorf("%s: %w", tag, release.ErrDigestChanged)
		}
	}
	if !record.IsNewerThan(release.Record{RootVersion: previous.rootVersion, Sequence: previous.sequence}) {
		return nil, errors.New("the record is not newer than the ledger's newest: cores would not take it")
	}
	return []string{
		string(trust.ChainPublisher), kindRelease,
		fmt.Sprintf("%d-%d", record.RootVersion, record.Sequence),
	}, nil
}

// checkDrill is where a drill is kept, once a key of the chain's set signed it; it names the keys.
func (run signing) checkDrill(kind trust.ChainKind, signed trust.Signed) ([]string, error) {
	chain, isHeld, err := run.ledger.chain(kind)
	if err != nil || !isHeld {
		return nil, errors.Join(err, errors.New("the ledger holds no such chain"))
	}
	signers, err := chain.VerifyAny(trust.NamespaceDrill, signed)
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(run.stdout, "drilled: %s\n", strings.Join(signers, ", ")) //nolint:errcheck // a terminal
	return []string{kindDrill, trust.PayloadHash(signed.Payload)[:16]}, nil
}

// bootstrapScript is the install's bootstrap, which bootstrap fills in with the publisher's keys and
// the places the record is served.
//
//go:embed bootstrap.sh
var bootstrapScript string

// bootstrap writes the install's bootstrap script (the audit's §3.9): the publisher chain's current
// keys, of both tiers, as ssh-keygen's allowed signers for release records alone, and the places the
// record is served; it says the script's SHA-256, which the documentation and release page print.
func bootstrap(run signing, flags *flag.FlagSet, arguments []string) (bool, error) {
	out := flags.String("out", "", "the script written (required)")
	var locations []string
	flags.Func("location", "a place the release record is served; repeated; none takes the chain's",
		appendTo(&locations))
	if !parsed(&run, flags, arguments) || *out == "" {
		return true, nil
	}
	chain, isHeld, err := run.ledger.chain(trust.ChainPublisher)
	if err != nil || !isHeld {
		return false, errors.Join(err, errors.New("the ledger holds no publisher chain"))
	}
	if len(locations) == 0 {
		locations = chain.Current().Locations
	}
	script, err := renderBootstrap(chain.Current(), locations)
	if err != nil {
		return false, err
	}
	if err = os.WriteFile(*out, []byte(script), 0o644); err != nil { //nolint:gosec // published, read by all
		return false, fmt.Errorf("write the bootstrap: %w", err)
	}
	_, err = fmt.Fprintf(run.stdout, "%s: SHA-256 %s\n", *out, trust.PayloadHash(script))
	return false, err
}

// renderBootstrap is the bootstrap for the publisher set root, fetching the record from locations.
func renderBootstrap(root trust.Root, locations []string) (string, error) {
	if len(locations) == 0 {
		return "", errors.New("name at least one --location the release record is served at")
	}
	var signers []string
	for _, key := range root.Keys {
		if key.Public != nil && !slices.Contains(root.Removed, key.Fingerprint()) && !root.IsSuspended(key) {
			signers = append(signers, `orpheum namespaces="`+trust.NamespaceRelease+`" `+key.Text)
		}
	}
	origins := make([]string, 0, len(locations))
	for _, location := range locations {
		origin, err := addressdoc.Issuer(location)
		if err != nil {
			return "", err
		}
		origins = append(origins, origin)
	}
	script := strings.Replace(bootstrapScript, "{{SIGNERS}}", strings.Join(signers, "\n"), 1)
	return strings.Replace(script, "{{LOCATIONS}}", strings.Join(origins, " "), 1), nil
}

// publish writes the files every place serves from the ledger.
func publish(run signing, flags *flag.FlagSet, arguments []string) (bool, error) {
	out := flags.String("out", "", "the directory written (required)")
	if !parsed(&run, flags, arguments) || *out == "" {
		return true, nil
	}
	return false, run.ledger.publish(*out)
}

// pending writes payload to sign into the ledger's pending directory, shows it and what changed from
// before, and says the command that signs it, and by whom.
func (run signing) pending(name, namespace string, payload []byte, before, signers string) error {
	path := run.ledger.path("pending", name)
	// gosec's taint check (G703) flags this line in some runs and not in others, so the suppression is
	// also kept when unused (idiosyncrasies, "gosec's G703 flags the same line in one run and not the next").
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil { //nolint:gosec,nolintlint // the owner's own ledger
		return fmt.Errorf("write the payload: %w", err)
	}
	if err := os.WriteFile(path, payload, 0o600); err != nil { //nolint:gosec // the owner's own ledger
		return fmt.Errorf("write the payload: %w", err)
	}
	var shown strings.Builder
	fmt.Fprintf(&shown, "%s\n%s", path, payload)
	if before != "" {
		shown.WriteString("\nchanged from the current one:\n" + difference(before, string(payload)))
	}
	fmt.Fprintf(&shown, "\nRead every line. Sign it with %s:\n  ssh-keygen -Y sign -n %s -f <the key's stub> %s\n"+
		"then check it:\n  go run ./cmd/discovery-sign check --ledger %s --payload %s --publish <dir> ...\n",
		signers, namespace, path, run.ledger, path)
	_, err := io.WriteString(run.stdout, shown.String())
	return err
}

// difference is the lines before has that after has not ("- "), and the reverse ("+ ").
func difference(before, after string) string {
	old, now := strings.Split(before, "\n"), strings.Split(after, "\n")
	var shown strings.Builder
	for _, line := range old {
		if line != "" && !slices.Contains(now, line) {
			shown.WriteString("- " + line + "\n")
		}
	}
	for _, line := range now {
		if line != "" && !slices.Contains(old, line) {
			shown.WriteString("+ " + line + "\n")
		}
	}
	return shown.String()
}

// addKeys adds each .pub file's key to root in role, a hardware key marked uv.
func addKeys(root *trust.Root, role trust.Role, files []string) error {
	for _, file := range files {
		key, err := readKey(file)
		if err != nil {
			return err
		}
		key.Role = role
		root.Keys = append(root.Keys, key)
	}
	return nil
}

// removeKey takes named, a .pub file or a fingerprint, out of root's keys and names it removed, for
// good, suspended or not; it returns the role it had.
func removeKey(root *trust.Root, named string) (trust.Role, error) {
	fingerprint, index, err := heldKey(*root, named)
	if err != nil {
		return "", err
	}
	role := root.Keys[index].Role
	root.Keys = slices.Delete(root.Keys, index, index+1)
	root.Removed = append(root.Removed, fingerprint)
	root.Suspended = slices.DeleteFunc(root.Suspended, func(held string) bool { return held == fingerprint })
	return role, nil
}

// changeStanding removes, suspends and resumes the keys named in root, and says whether only a
// recovery key may sign the change: a recovery key removed or suspended, or any key resumed, so a
// thief holding an everyday key cannot undo its suspension (D394 §4).
func changeStanding(root *trust.Root, removed, suspended, resumed []string) (bool, error) {
	isRecovery := len(resumed) > 0
	for _, each := range removed {
		role, err := removeKey(root, each)
		if err != nil {
			return false, err
		}
		isRecovery = isRecovery || role == trust.RoleRecovery
	}
	for _, each := range suspended {
		role, err := suspendKey(root, each, true)
		if err != nil {
			return false, err
		}
		isRecovery = isRecovery || role == trust.RoleRecovery
	}
	for _, each := range resumed {
		if _, err := suspendKey(root, each, false); err != nil {
			return false, err
		}
	}
	return isRecovery, nil
}

// suspendKey suspends named, a .pub file or a fingerprint of a key root holds, or resumes it when
// isSuspending is false; it returns the key's role.
func suspendKey(root *trust.Root, named string, isSuspending bool) (trust.Role, error) {
	fingerprint, index, err := heldKey(*root, named)
	if err != nil {
		return "", err
	}
	isSuspended := root.IsSuspended(root.Keys[index])
	switch {
	case isSuspending && isSuspended:
		return "", fmt.Errorf("%s is suspended already", fingerprint)
	case !isSuspending && !isSuspended:
		return "", fmt.Errorf("%s is not suspended", fingerprint)
	case isSuspending:
		root.Suspended = append(root.Suspended, fingerprint)
	default:
		root.Suspended = slices.DeleteFunc(root.Suspended, func(held string) bool { return held == fingerprint })
	}
	return root.Keys[index].Role, nil
}

// heldKey is named's fingerprint, a .pub file's or named itself when it is one, and its index among
// root's keys.
func heldKey(root trust.Root, named string) (string, int, error) {
	fingerprint := named
	if !strings.HasPrefix(named, "SHA256:") {
		key, err := readKey(named)
		if err != nil {
			return "", 0, err
		}
		fingerprint = key.Fingerprint()
	}
	index := slices.IndexFunc(root.Keys, func(key trust.RootKey) bool { return key.Fingerprint() == fingerprint })
	if index < 0 {
		return "", 0, fmt.Errorf("%s is not in the set", fingerprint)
	}
	return fingerprint, index, nil
}

// setThresholds sets each "<role>=<n>".
func setThresholds(root *trust.Root, thresholds []string) error {
	for _, each := range thresholds {
		role, count, isCut := strings.Cut(each, "=")
		number, err := strconv.Atoi(count)
		if !isCut || err != nil || number < 1 {
			return fmt.Errorf("threshold %q: <role>=<n>", each)
		}
		root.Thresholds[trust.Role(role)] = number
	}
	return nil
}

// readKey is the key a .pub file holds, its comment its label, a hardware key marked uv: the owner's
// keys are made with -O verify-required (D393 §3).
func readKey(file string) (trust.RootKey, error) {
	contents, err := os.ReadFile(file) //nolint:gosec // the owner's own key file
	if err != nil {
		return trust.RootKey{}, fmt.Errorf("read the key %s: %w", file, err)
	}
	public, comment, _, _, err := ssh.ParseAuthorizedKey(contents)
	if err != nil {
		return trust.RootKey{}, fmt.Errorf("read the key %s: %w", file, err)
	}
	text := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(public)))
	if _, err = trust.ParseKey(text); err != nil {
		return trust.RootKey{}, err
	}
	label := strings.Join(strings.Fields(comment), "-")
	isHardware := public.Type() == ssh.KeyAlgoSKED25519 || public.Type() == ssh.KeyAlgoSKECDSA256
	return trust.RootKey{Public: public, MustVerifyUV: isHardware, Text: text, Label: label}, nil
}

// isChain is whether kind names a chain.
func isChain(kind string) bool {
	return kind == string(trust.ChainDiscovery) || kind == string(trust.ChainPublisher)
}

// appendTo is a flag that appends each value it is given to values.
func appendTo(values *[]string) func(string) error {
	return func(value string) error {
		*values = append(*values, value)
		return nil
	}
}
