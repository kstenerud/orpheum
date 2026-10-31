// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/kstenerud/orpheum/discovery/trust"
)

// testKey makes (or takes) a key, has it sign a throwaway statement, and says whether its signatures
// carry the user-verified flag (FIDO's UV, set when the PIN or Touch ID was given), which a core demands
// of a key marked uv (D393 §3). It is the owner's test of whether the Mac's Secure Enclave may be an
// everyday key (D394 §7).
func (guide *guided) testKey(arguments []string) error {
	flags := flag.NewFlagSet("discovery-sign test-key", flag.ContinueOnError)
	flags.SetOutput(guide.err)
	where := flags.String("dir", "", "where the test key is made (a new temporary directory when not given)")
	if flags.Parse(arguments) != nil || flags.NArg() != 0 {
		return flag.ErrHelp
	}
	say := guide.say
	say.sayf("Key test. I make a key (or use one you have), sign a throwaway statement with it, and check the\n" +
		"signature as a core would. A core refuses a hardware key's signature unless it carries the\n" +
		"\"user-verified\" mark, which says the PIN or your fingerprint was really asked for. This test finds\n" +
		"out whether a key, such as this Mac's Secure Enclave, makes that mark. Nothing here is published.")
	what, err := say.choose("Which key do you want to test?", []string{kindMac, kindYubiKey, "file"}, kindMac)
	if err != nil {
		return err
	}
	directory := *where
	if directory == "" {
		if directory, err = os.MkdirTemp("", "orpheum-test-key-"); err != nil {
			return err
		}
	} else if mkdirErr := os.MkdirAll(directory, 0o700); mkdirErr != nil {
		return mkdirErr
	}
	file, kind := filepath.Join(directory, "test-key"), what
	if what == "file" {
		if file, err = say.ask("The key's file (its .pub must be beside it)?", ""); err != nil {
			return err
		}
		if kind, err = say.choose("Is it a Mac key, a YubiKey or a software key?",
			[]string{kindMac, kindYubiKey, kindSoftware}, kindYubiKey); err != nil {
			return err
		}
	} else {
		say.sayf("Making a test key in %s. %s", directory, touchHint(kind))
		spec := keySpec{Kind: kind, File: file, Application: "ssh:orpheum-test", Label: "test-key"}
		if makeErr := guide.keys.Make(spec); makeErr != nil {
			return makeErr
		}
	}
	key, err := readKey(file + ".pub")
	if err != nil {
		return err
	}
	return guide.judgeKey(directory, file, kind, key)
}

// judgeKey signs a throwaway statement, which lists the key as a core's set lists the paid service's,
// and reports whether a core would take the signature.
func (guide *guided) judgeKey(directory, file, kind string, key trust.RootKey) error {
	say := guide.say
	if key.Public.Type() != ssh.KeyAlgoSKED25519 && key.Public.Type() != ssh.KeyAlgoSKECDSA256 {
		say.sayf("Result: this is a software key. It has no PIN or touch to mark, so it can only be listed\n" +
			"without \"uv\": fine for staging or a self-hoster, not for the paid service's root (D394 §1).")
		return nil
	}
	payload, err := testRoot(guide.now, key).Payload()
	if err != nil {
		return err
	}
	payloadFile := filepath.Join(directory, "test-statement")
	if writeErr := os.WriteFile(payloadFile, payload, 0o600); writeErr != nil {
		return writeErr
	}
	say.sayf("Signing a throwaway statement. %s", touchHint(kind))
	spec := signSpec{Namespace: trust.NamespaceDiscoveryRoot, KeyFile: file, Payload: payloadFile, Kind: kind}
	if signErr := guide.keys.Sign(spec); signErr != nil {
		return signErr
	}
	signature, err := os.ReadFile(payloadFile + ".sig") //nolint:gosec // as above
	if err != nil {
		return fmt.Errorf("ssh-keygen wrote no signature: %w", err)
	}
	blocks, err := trust.Inspect(string(signature))
	if err != nil || len(blocks) != 1 {
		return errors.Join(err, errors.New("ssh-keygen wrote something that is not one signature"))
	}
	say.sayf("The signature carries the flags %#02x and the counter %d.", blocks[0].Flags, blocks[0].Counter)
	_, err = trust.Genesis(trust.ChainDiscovery, trust.Signed{Payload: string(payload), Signature: string(signature)})
	switch {
	case err == nil:
		say.sayf("Result: this key's signatures DO carry the user-verified flag. A core accepts the key as an\n" +
			"everyday key (D394 §7), as it accepts a YubiKey whose PIN was given.")
	case errors.Is(err, trust.ErrNotUserVerified):
		say.sayf("Result: this key's signatures do NOT carry the user-verified flag, so a core would refuse them\n" +
			"for a key listed as the paid service's keys are. Do not use it as an everyday key; use a\n" +
			"YubiKey instead (D394 §7).")
	default:
		return fmt.Errorf("the signature does not hold, so nothing is learned: %w", err)
	}
	return nil
}

// testRoot is a throwaway discovery genesis listing key in both tiers, marked uv as a hardware key of
// the paid service is.
func testRoot(now time.Time, key trust.RootKey) trust.Root {
	everyday, recovery := key, key
	everyday.Role, recovery.Role = trust.RoleEveryday, trust.RoleRecovery
	return trust.Root{
		Chain: trust.ChainDiscovery, Version: 1, Issued: now, Keys: []trust.RootKey{recovery, everyday},
		Thresholds: map[trust.Role]int{trust.RoleEveryday: 1, trust.RoleRecovery: 1},
	}
}
