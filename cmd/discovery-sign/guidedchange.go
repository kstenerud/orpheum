// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/kstenerud/orpheum/discovery/trust"
)

// The guided changes to a key's standing (D394 §4). Each names a device by the name the ceremony gave
// it, and changes its key in every chain it has one in, one chain after another: a lost YubiKey holds a
// key of each chain.
const (
	changeSuspend = "suspend"
	changeResume  = "resume"
	changeAdd     = "add"
	changeRemove  = "remove"
)

// changeCommands are the guided changes, as commands.
func (guide *guided) changeCommands() map[string]func(arguments []string) error {
	commands := map[string]func([]string) error{}
	for _, change := range []string{changeSuspend, changeResume, changeAdd, changeRemove} {
		commands[change] = func(arguments []string) error { return guide.changeKey(change, arguments) }
	}
	return commands
}

// changeKey suspends, resumes, adds or removes the device --key names, in each chain: it shows each new
// statement as a change, has a key allowed to sign it do so, checks it as a core does, keeps it, and
// writes the files to publish.
func (guide *guided) changeKey(change string, arguments []string) error {
	flags := flag.NewFlagSet("discovery-sign "+change, flag.ContinueOnError)
	flags.SetOutput(guide.err)
	where := flags.String("folder", "", "the records folder (required)")
	name := flags.String("key", "", "the key's name, as the ceremony gave it (asked when not given)")
	if flags.Parse(arguments) != nil || flags.NArg() != 0 || *where == "" {
		return flag.ErrHelp
	}
	place, err := openFolder(*where)
	if err != nil {
		return err
	}
	if len(place.state.Keys) == 0 {
		return errors.New("the folder holds no keys: run the ceremony first")
	}
	if *name == "" {
		question := map[string]string{
			changeSuspend: "Which key is lost or out of your hands?", changeResume: "Which key is back?",
			changeAdd:    "What is the new key's name? (Put a sticker on it with that name.)",
			changeRemove: "Which key leaves for good?",
		}[change]
		if *name, err = guide.say.ask(question, ""); err != nil {
			return err
		}
	}
	if change == changeAdd {
		if stepErr := guide.makeDevice(place, *name); stepErr != nil {
			return stepErr
		}
	}
	guide.say.sayf("%s", changeIntro(change, *name))
	for _, chain := range []trust.ChainKind{trust.ChainDiscovery, trust.ChainPublisher} {
		key, isHeld := place.key(chain, *name)
		if !isHeld {
			return fmt.Errorf("the folder holds no %s key named %q", chain, *name)
		}
		if stepErr := guide.changeChain(place, chain, change, key); stepErr != nil {
			return stepErr
		}
	}
	if stepErr := guide.refreshSigned(place); stepErr != nil {
		return stepErr
	}
	if stepErr := guide.publishFiles(place); stepErr != nil {
		return stepErr
	}
	guide.say.sayf("Copy them to the server, then commit %s to its private repository.", place.path)
	return guide.say.wrote
}

// changeIntro says what the change does, before anything is signed.
func changeIntro(change, name string) string {
	return map[string]string{
		changeSuspend: fmt.Sprintf("Suspending %q: its signatures stop counting until a recovery key resumes it.\n"+
			"Another key signs the suspension; %q cannot sign its own.", name, name),
		changeResume: fmt.Sprintf("Resuming %q: its signatures count again. Only a recovery key may sign this, so a\n"+
			"thief holding an everyday key cannot undo its suspension.", name),
		changeAdd: fmt.Sprintf("Adding %q to both chains. A key already in the set signs each addition.", name),
		changeRemove: fmt.Sprintf("Removing %q for good: it can never be added back. If it may turn up again,\n"+
			"suspend it instead.", name),
	}[change]
}

// makeDevice asks what the new key is and makes its key for each chain.
func (guide *guided) makeDevice(place *folder, name string) error {
	if slices.ContainsFunc(place.state.Devices, func(each device) bool { return each.Name == name }) {
		return fmt.Errorf("a key named %q is in the folder already", name)
	}
	role, err := guide.say.choose(fmt.Sprintf("Is %s an everyday key or a recovery key?", name),
		[]string{roleEveryday, roleRecovery}, roleEveryday)
	if err != nil {
		return err
	}
	kind := kindSoftware
	if place.state.Software == "" {
		kind = kindYubiKey
		plug := fmt.Sprintf("Plug in the key named %q (only that one, and no other security key).", name)
		if stepErr := guide.say.press(plug); stepErr != nil {
			return stepErr
		}
	}
	each := device{Name: name, Role: role, Kind: kind}
	place.state.Devices = append(place.state.Devices, each)
	for _, chain := range []trust.ChainKind{trust.ChainDiscovery, trust.ChainPublisher} {
		if stepErr := guide.makeKey(place, each, chain); stepErr != nil {
			return stepErr
		}
	}
	return place.save()
}

// key is the folder's key of chain named name.
func (place *folder) key(chain trust.ChainKind, name string) (keyEntry, bool) {
	index := slices.IndexFunc(place.state.Keys, func(key keyEntry) bool {
		return key.Chain == string(chain) && key.Name == name
	})
	if index < 0 {
		return keyEntry{}, false
	}
	return place.state.Keys[index], true
}

// changeChain makes, shows, signs, checks and keeps chain's statement that changes key's standing.
func (guide *guided) changeChain(place *folder, chain trust.ChainKind, change string, key keyEntry) error {
	records := place.ledger()
	held, isHeld, err := records.chain(chain)
	if err != nil || !isHeld {
		return errors.Join(err, fmt.Errorf("the ledger holds no %s chain", chain))
	}
	next, isRecovery, err := standingChange(change, key, place.join(key.Pub)).next(held, guide.now)
	if err != nil {
		return fmt.Errorf("the %s chain: %w", chain, err)
	}
	payload, err := next.Payload()
	if err != nil {
		return err
	}
	path := records.path("pending", rootName(chain, next.Version))
	if stepErr := os.MkdirAll(filepath.Dir(path), 0o700); stepErr != nil {
		return stepErr
	}
	if stepErr := os.WriteFile(path, payload, 0o600); stepErr != nil {
		return stepErr
	}
	guide.say.sayf("\nThe %s chain's next statement (version %d) changes these lines (- gone, + new):\n%s",
		chain, next.Version, difference(currentPayload(held), string(payload)))
	if sure, askErr := guide.say.yes("Sign this change?", true); askErr != nil || !sure {
		return errors.Join(askErr, errors.New("nothing was signed"))
	}
	signer, err := guide.chooseSigner(place, held, key, isRecovery)
	if err != nil {
		return err
	}
	signed, err := guide.signWith(place, chain.RootNamespace(), path, []keyEntry{signer},
		func(one trust.Signed) error {
			_, acceptErr := held.Accept(one)
			return acceptErr
		})
	if err != nil {
		return err
	}
	return guide.accept(signing{now: guide.now, ledger: records, stdout: io.Discard}, string(chain), kindRoot, signed)
}

// refreshSigned signs the address document again when its signature no longer counts (its key
// suspended or removed), and says what to do when the newest release record's no longer does, which
// only a new record from the release workflow can mend.
func (guide *guided) refreshSigned(place *folder) error {
	records := place.ledger()
	discovery, _, err := records.chain(trust.ChainDiscovery)
	if err != nil {
		return err
	}
	document, err := records.newest(string(trust.ChainDiscovery), kindAddress)
	if err != nil {
		return err
	}
	if document.signed.Payload != "" && discovery.Verify(trust.NamespaceDiscoveryAddress, document.signed) != nil {
		guide.say.sayf("\nThe address document was signed by a key whose signatures no longer count, so cores would\n" +
			"refuse it. It is signed again now.")
		signer, chooseErr := guide.chooseSigner(place, discovery, keyEntry{}, false)
		if chooseErr != nil {
			return chooseErr
		}
		if signErr := guide.signAddress(place, signer); signErr != nil {
			return signErr
		}
	}
	publisher, _, err := records.chain(trust.ChainPublisher)
	if err != nil {
		return err
	}
	record, err := records.newest(string(trust.ChainPublisher), kindRelease)
	if err != nil {
		return err
	}
	if record.signed.Payload != "" && publisher.Verify(trust.NamespaceRelease, record.signed) != nil {
		guide.say.sayf("\nThe newest release record was signed by a key whose signatures no longer count, so cores\n"+
			"will refuse it. Make a new unsigned record (release-record --root-version %d) and sign it\n"+
			"with sign-release.", publisher.Version())
	}
	return nil
}

// standingChange is change made to key, whose public half is the file public.
func standingChange(change string, key keyEntry, public string) rootChange {
	named := []string{public}
	switch {
	case change == changeSuspend:
		return rootChange{suspended: named}
	case change == changeResume:
		return rootChange{resumed: named}
	case change == changeRemove:
		return rootChange{removed: named}
	case key.Role == roleRecovery:
		return rootChange{addRecovery: named}
	}
	return rootChange{addEveryday: named}
}

// chooseSigner is the key that signs a change to changed's standing: one whose signatures count in the
// chain now, of the recovery tier when only a recovery key may sign, and not changed itself, whose own
// signature would not carry its suspension or removal.
func (guide *guided) chooseSigner(place *folder, held trust.Chain, changed keyEntry, isRecovery bool,
) (keyEntry, error) {
	counting := held.Fingerprints() // "<role> <fingerprint>", suspended keys left out
	var allowed []keyEntry
	var names []string
	for _, key := range place.state.chainKeys(held.Kind(), "") {
		public, err := readKey(place.join(key.Pub))
		if err != nil {
			return keyEntry{}, err
		}
		if key.Name == changed.Name || !slices.Contains(counting, key.Role+" "+public.Fingerprint()) ||
			(isRecovery && key.Role != roleRecovery) {
			continue
		}
		allowed, names = append(allowed, key), append(names, key.Name)
	}
	tier := "an everyday or a recovery key"
	if isRecovery {
		tier = "a recovery key"
	}
	switch len(allowed) {
	case 0:
		return keyEntry{}, fmt.Errorf("this change needs %s whose signatures count, other than %q, and the folder "+
			"holds none", tier, changed.Name)
	case 1:
		guide.say.sayf("It is signed by %s: %q.", tier, allowed[0].Name)
		return allowed[0], nil
	}
	chosen, err := guide.say.choose("It is signed by "+tier+". Which one?", names, names[0])
	if err != nil {
		return keyEntry{}, err
	}
	return allowed[slices.Index(names, chosen)], nil
}
