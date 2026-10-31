// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"slices"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/kstenerud/orpheum/discovery/addressdoc"
	"github.com/kstenerud/orpheum/discovery/release"
	"github.com/kstenerud/orpheum/discovery/trust"
)

// signing is one run: when it is, the keys it may sign with, and where it says what it did.
type signing struct {
	now     time.Time
	agent   func() ([]ssh.Signer, error)
	keyFile string
	stdout  io.Writer
}

// run signs what its arguments name and returns the exit status.
func run(arguments []string, now time.Time, agentKeys func() ([]ssh.Signer, error), stdout, stderr io.Writer) int {
	if len(arguments) == 0 {
		return write(stderr, usage, exitUsage)
	}
	commands := map[string]func(signing, *flag.FlagSet, []string) (bool, error){
		"root-set": signRootSet, "delegation": signDelegation, "document": signDocument, "record": signRecord,
		"seal": seal,
	}
	command, isKnown := commands[arguments[0]]
	if !isKnown {
		return write(stderr, usage, exitUsage)
	}
	flags := flag.NewFlagSet("discovery-sign "+arguments[0], flag.ContinueOnError)
	flags.SetOutput(stderr)
	run := signing{now: now, agent: agentKeys, stdout: stdout}
	flags.StringVar(&run.keyFile, "key-file", "", "a private key file to sign with in place of the ssh-agent's keys")
	isUsage, err := command(run, flags, arguments[1:])
	switch {
	case isUsage:
		return write(stderr, usage, exitUsage)
	case err != nil:
		return write(stderr, fmt.Sprintf("discovery-sign: %v\n", err), exitFailure)
	}
	return exitSuccess
}

// signRootSet writes the next root key set into the chain in --sets: the last set's keys with --add's
// added and --remove's removed, its version one more, signed by a key of the last set, or, for the
// first, by one of its own (D391 §2). The rules for a key removed and for two sets of one version
// are the core's (trust.Pinned.Advance), which the chain written is checked by.
func signRootSet(run signing, flags *flag.FlagSet, arguments []string) (bool, error) {
	setsFile := flags.String("sets", "", "the root key sets' chain, a JSON list, made when absent (required)")
	var added, removed []string
	flags.Func("add", "a key to add: an authorised key's line or a .pub file", appendTo(&added))
	flags.Func("remove", "a key to remove: an authorised key's line or a .pub file", appendTo(&removed))
	if err := flags.Parse(arguments); err != nil || *setsFile == "" || len(added)+len(removed) == 0 {
		return true, err
	}
	sets, err := readSets(*setsFile, false)
	if err != nil {
		return false, err
	}
	last := trust.RootSet{}
	if len(sets) > 0 {
		pinned, chainErr := trust.FirstUse(sets)
		if chainErr != nil {
			return false, fmt.Errorf("the chain in %s: %w", *setsFile, chainErr)
		}
		last = trust.RootSet{Version: pinned.Version, Keys: pinned.Keys}
	}
	next, err := nextSet(last, added, removed, run.now)
	if err != nil {
		return false, err
	}
	signers := last.Keys
	if len(sets) == 0 {
		signers = next.Keys
	}
	signed, err := run.sign(trust.KindRootSet, mustJSON(next), signers)
	if err != nil {
		return false, err
	}
	sets = append(sets, signed)
	pinned, err := trust.FirstUse(sets)
	if err != nil || pinned.Version != next.Version {
		return false, fmt.Errorf("the new chain does not hold: %w", err)
	}
	if writeErr := writeJSON(*setsFile, sets); writeErr != nil {
		return false, writeErr
	}
	_, err = fmt.Fprintf(run.stdout, "%s: root key set %d, of %d keys:\n  %s\n", *setsFile, next.Version,
		len(next.Keys), strings.Join(pinned.Fingerprints(), "\n  "))
	return false, err
}

// nextSet is the set after last, one version on: its keys and those of added, but for removed's.
func nextSet(last trust.RootSet, added, removed []string, now time.Time) (trust.RootSet, error) {
	keys := slices.Clone(last.Keys)
	for _, each := range added {
		line, err := keyLine(each)
		if err != nil {
			return trust.RootSet{}, err
		}
		if !slices.Contains(keys, line) {
			keys = append(keys, line)
		}
	}
	for _, each := range removed {
		line, err := keyLine(each)
		if err != nil {
			return trust.RootSet{}, err
		}
		if !slices.Contains(keys, line) {
			return trust.RootSet{}, fmt.Errorf("%s is not in the set", line)
		}
		keys = slices.DeleteFunc(keys, func(kept string) bool { return kept == line })
	}
	if len(keys) == 0 {
		return trust.RootSet{}, trust.ErrNoRootKeys
	}
	return trust.RootSet{Version: last.Version + 1, IssuedAt: now.UnixMilli(), Keys: keys}, nil
}

// signDelegation writes a root key's delegation of an online key, for one issuer and a time from now.
func signDelegation(run signing, flags *flag.FlagSet, arguments []string) (bool, error) {
	setsFile := flags.String("sets", "", "the root key sets' chain (required)")
	issuer := flags.String("issuer", "", "the issuer whose documents the online key signs (required)")
	key := flags.String("key", "", "the online key: an authorised key's line or a .pub file (required)")
	lasts := flags.Duration("for", 0, "how long the delegation lasts from now, 2160h (required)")
	out := flags.String("out", "", "where the signed delegation is written (required)")
	if err := flags.Parse(arguments); err != nil || *setsFile == "" || *issuer == "" || *key == "" || *lasts <= 0 ||
		*out == "" {
		return true, err
	}
	pinned, err := chainOf(*setsFile)
	if err != nil {
		return false, err
	}
	online, err := keyLine(*key)
	if err != nil {
		return false, err
	}
	origin, err := addressdoc.Issuer(*issuer)
	if err != nil {
		return false, err
	}
	signed, err := run.sign(trust.KindDelegation, mustJSON(trust.Delegation{
		Key: online, Issuer: origin, NotBefore: run.now.UnixMilli(), NotAfter: run.now.Add(*lasts).UnixMilli(),
	}), pinned.Keys)
	if err != nil {
		return false, err
	}
	return false, writeJSON(*out, signed)
}

// signDocument writes the address document the flags name, signed by a root key, with the chain of
// sets beside it: what a discovery serves at addressdoc.WellKnownPath.
func signDocument(run signing, flags *flag.FlagSet, arguments []string) (bool, error) {
	setsFile := flags.String("sets", "", "the root key sets' chain (required)")
	out := flags.String("out", "", "where the served document is written (required)")
	var document addressdoc.Document
	flags.StringVar(&document.Issuer, "issuer", "", "the discovery's issuer, its typed origin (required)")
	flags.StringVar(&document.CoreAPI, "core-api", "", "the core API's base URL (required)")
	flags.StringVar(&document.ClientAPI, "client-api", "", "the client API's base URL (required)")
	flags.StringVar(&document.LoaderOrigin, "loader", "", "the loader's origin (required)")
	flags.StringVar(&document.ClientOrigins, "client-origins", "", "the client origins' pattern, {core} in a label")
	flags.StringVar(&document.AccountOrigin, "account", "", "the account pages' origin")
	flags.StringVar(&document.Homepage, "homepage", "", "what the discovery is, for a stranger")
	flags.StringVar(&document.Support, "support", "", "the discovery's address for help")
	flags.Func("location", "a further place the document is served under; repeated", appendTo(&document.Locations))
	if err := flags.Parse(arguments); err != nil || *setsFile == "" || *out == "" {
		return true, err
	}
	sets, err := readSets(*setsFile, true)
	if err != nil {
		return false, err
	}
	pinned, err := trust.FirstUse(sets)
	if err != nil {
		return false, err
	}
	if document.Issuer, err = addressdoc.Issuer(document.Issuer); err != nil {
		return false, err
	}
	document.IssuedAt = run.now.UnixMilli()
	if checkErr := document.Check(document.Issuer); checkErr != nil {
		return false, checkErr
	}
	signed, err := run.sign(trust.KindAddressDocument, mustJSON(document), pinned.Keys)
	if err != nil {
		return false, err
	}
	served := addressdoc.Served{Document: signed, RootSets: sets}
	if _, _, err = addressdoc.Open(served, pinned, document.Issuer, run.now); err != nil {
		return false, fmt.Errorf("the signed document does not hold: %w", err)
	}
	return false, writeJSON(*out, served)
}

// signRecord signs the unsigned release record's exact bytes with a root key (D391 §6).
func signRecord(run signing, flags *flag.FlagSet, arguments []string) (bool, error) {
	setsFile := flags.String("sets", "", "the root key sets' chain (required)")
	recordFile := flags.String("record", "", "the unsigned release record, as release-record wrote it (required)")
	out := flags.String("out", "", "where the signed record is written (required)")
	if err := flags.Parse(arguments); err != nil || *setsFile == "" || *recordFile == "" || *out == "" {
		return true, err
	}
	pinned, err := chainOf(*setsFile)
	if err != nil {
		return false, err
	}
	payload, err := os.ReadFile(*recordFile) //nolint:gosec // the owner's own file
	if err != nil {
		return false, fmt.Errorf("read the record: %w", err)
	}
	signed, err := run.sign(trust.KindReleaseRecord, payload, pinned.Keys)
	if err != nil {
		return false, err
	}
	if _, err = release.Open(signed, pinned); err != nil {
		return false, fmt.Errorf("the signed record does not hold: %w", err)
	}
	return false, writeJSON(*out, signed)
}

// seal puts a payload and the signature `ssh-keygen -Y sign` made over it by hand together, checked
// by the chain of sets: a root key's, for a delegation or a release record.
func seal(_ signing, flags *flag.FlagSet, arguments []string) (bool, error) {
	setsFile := flags.String("sets", "", "the root key sets' chain, which the signature is checked by (required)")
	kind := flags.String("kind", "", "what was signed: the namespace given to ssh-keygen -n (required)")
	payloadFile := flags.String("payload", "", "the file that was signed (required)")
	signatureFile := flags.String("signature", "", "its signature, the .sig ssh-keygen wrote (required)")
	out := flags.String("out", "", "where the signed object is written (required)")
	if err := flags.Parse(arguments); err != nil || *setsFile == "" || *kind == "" || *payloadFile == "" ||
		*signatureFile == "" || *out == "" {
		return true, err
	}
	pinned, err := chainOf(*setsFile)
	if err != nil {
		return false, err
	}
	payload, err := os.ReadFile(*payloadFile) //nolint:gosec // the owner's own file
	if err != nil {
		return false, fmt.Errorf("read the payload: %w", err)
	}
	signature, err := os.ReadFile(*signatureFile) //nolint:gosec // the owner's own file
	if err != nil {
		return false, fmt.Errorf("read the signature: %w", err)
	}
	signed := trust.Signed{Payload: string(payload), Signature: string(signature)}
	if checkErr := checkSealed(trust.Kind(*kind), signed, pinned); checkErr != nil {
		return false, checkErr
	}
	return false, writeJSON(*out, signed)
}

// checkSealed is whether signed holds as kind under pinned.
func checkSealed(kind trust.Kind, signed trust.Signed, pinned trust.Pinned) error {
	switch kind {
	case trust.KindReleaseRecord:
		_, err := release.Open(signed, pinned)
		return err
	case trust.KindDelegation:
		var delegation trust.Delegation
		if err := json.Unmarshal([]byte(signed.Payload), &delegation); err != nil {
			return fmt.Errorf("not a delegation: %w", err)
		}
		probe := trust.Signed{Delegation: &signed}
		_, err := pinned.OpenDocument(probe, delegation.Issuer, time.UnixMilli(delegation.NotBefore))
		if errors.Is(err, trust.ErrDelegation) {
			return err
		}
		return nil
	case trust.KindRootSet, trust.KindAddressDocument:
	}
	return fmt.Errorf("%q: seal a %s or a %s; a root key set and a document are signed whole by root-set "+
		"and document", kind, trust.KindDelegation, trust.KindReleaseRecord)
}

// sign is payload signed as kind by the first of the run's keys that is one of allowed's: its key
// file's, or the ssh-agent's, a YubiKey's asking for a touch.
func (run signing) sign(kind trust.Kind, payload []byte, allowed []string) (trust.Signed, error) {
	signers, err := run.signers()
	if err != nil {
		return trust.Signed{}, err
	}
	for _, signer := range signers {
		if slices.Contains(allowed, trust.KeyLine(signer.PublicKey())) {
			signed, signErr := trust.Sign(signer, kind, payload)
			if signErr != nil {
				return trust.Signed{}, fmt.Errorf("sign with %s: %w", trust.Fingerprint(signer.PublicKey()), signErr)
			}
			return signed, nil
		}
	}
	return trust.Signed{}, fmt.Errorf("none of the keys at hand is a root key that may sign this; plug in "+
		"one of your YubiKeys and load it (ssh-add -K), or name a --key-file; wanted one of: %s",
		strings.Join(fingerprints(allowed), ", "))
}

// signers are the keys the run may sign with: its key file's, else the ssh-agent's.
func (run signing) signers() ([]ssh.Signer, error) {
	if run.keyFile == "" {
		return run.agent()
	}
	contents, err := os.ReadFile(run.keyFile)
	if err != nil {
		return nil, fmt.Errorf("read the key file: %w", err)
	}
	signer, err := ssh.ParsePrivateKey(contents)
	if err != nil {
		return nil, fmt.Errorf("read the key file %s (a key with a passphrase is signed with through the "+
			"ssh-agent): %w", run.keyFile, err)
	}
	return []ssh.Signer{signer}, nil
}

// chainOf is the set the chain in file reaches.
func chainOf(file string) (trust.Pinned, error) {
	sets, err := readSets(file, true)
	if err != nil {
		return trust.Pinned{}, err
	}
	pinned, err := trust.FirstUse(sets)
	if err != nil {
		return trust.Pinned{}, fmt.Errorf("the chain in %s: %w", file, err)
	}
	return pinned, nil
}

// readSets is the chain of root key sets' statements in file; absent, none, unless it must be there.
func readSets(file string, isRequired bool) ([]trust.Signed, error) {
	contents, err := os.ReadFile(file) //nolint:gosec // the owner's own file
	if errors.Is(err, fs.ErrNotExist) && !isRequired {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read the root key sets: %w", err)
	}
	var sets []trust.Signed
	if err = json.Unmarshal(contents, &sets); err != nil {
		return nil, fmt.Errorf("read the root key sets %s: %w", file, err)
	}
	return sets, nil
}

// keyLine is named as a key a discovery may hold: an authorised key's line, or the .pub file holding one.
func keyLine(named string) (string, error) {
	text := named
	if !strings.Contains(named, " ") {
		contents, err := os.ReadFile(named) //nolint:gosec // the owner's own key file
		if err != nil {
			return "", fmt.Errorf("read the key %s: %w", named, err)
		}
		text = string(bytes.TrimSpace(contents))
	}
	key, err := trust.ParseKey(text)
	if err != nil {
		return "", err
	}
	return trust.KeyLine(key), nil
}

// fingerprints are lines' keys' fingerprints.
func fingerprints(lines []string) []string {
	shown := make([]string, 0, len(lines))
	for _, line := range lines {
		if key, err := trust.ParseKey(line); err == nil {
			shown = append(shown, trust.Fingerprint(key))
		}
	}
	return shown
}

// appendTo is a flag that appends each value it is given to values.
func appendTo(values *[]string) func(string) error {
	return func(value string) error {
		*values = append(*values, value)
		return nil
	}
}

// mustJSON is value as JSON: a payload's exact bytes.
func mustJSON(value any) []byte {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err) // unreachable: the payloads are plain structs
	}
	return encoded
}

// writeJSON writes value to file as indented JSON, readable by all: nothing signed is secret.
func writeJSON(file string, value any) error {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", file, err)
	}
	if err = os.WriteFile(file, append(encoded, '\n'), 0o644); err != nil { //nolint:gosec // public, as said
		return fmt.Errorf("write %s: %w", file, err)
	}
	return nil
}
