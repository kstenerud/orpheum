// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

// This file is the one place the tool starts a process (D394 §5; the lint's exception names this file):
// ssh-keygen, given the owner's own terminal and environment, since a hardware key asks for its touch
// and PIN through them. Everything else asks a keygen, so a test puts a fake in its place and never
// runs ssh-keygen or touches a key.

// The kinds of key the ceremony makes.
const (
	kindYubiKey  = "yubikey"
	kindMac      = "mac"
	kindSoftware = "software"
)

// macProvider is the library that lets OpenSSH use the Mac's Secure Enclave as a security key.
//
// Not confirmed on a Mac (the tool was written in a sandbox with no Mac): that Apple ships this path
// on the owner's macOS (it is documented for macOS 13 and later), that its ssh-keygen accepts -w for
// "ecdsa-sk" and "-O application", that it accepts neither "resident" nor "verify-required" (so this
// tool asks for neither), and that its signatures carry the user-verified flag. The last is what
// `discovery-sign test-key` finds out (D394 §7).
const macProvider = "/usr/lib/ssh-keychain.dylib"

// keySpec is a key to make.
type keySpec struct {
	// Kind is kindYubiKey, kindMac or kindSoftware.
	Kind string
	// File is where the private key (for a hardware key, its stub) is written; File + ".pub" beside it.
	File string
	// Application is a hardware key's application name, one per key.
	Application string
	// Label is the key's comment, which names it in the statements.
	Label string
	// Attestation is where a YubiKey's attestation is written; empty for the others.
	Attestation string
}

// signSpec is a payload to sign.
type signSpec struct {
	Namespace string
	// KeyFile is the private key or stub; Payload the file signed, whose signature is written beside it
	// at Payload + ".sig".
	KeyFile string
	Payload string
	// Kind says whether the Mac's provider is needed to sign.
	Kind string
}

// keygen makes keys and signs with them.
type keygen interface {
	Make(spec keySpec) error
	Sign(spec signSpec) error
}

// sshKeygen is OpenSSH's ssh-keygen, run on the owner's terminal.
type sshKeygen struct{}

// makeArguments are ssh-keygen's arguments to make spec.
//
// A YubiKey's: ed25519-sk (OpenSSH 8.2 and later); "resident" so ssh-keygen -K gets the stub back on
// any computer; "verify-required" so every signature needs the PIN and carries the user-verified flag
// a core checks (D393 §3); "application" one name per key (D393 §2); "write-attestation" (OpenSSH 8.3
// and later) the device's proof it made the key; -N "" since the PIN protects it, not a passphrase.
// Not confirmed against a real YubiKey: the exact option spelling "user=" for resident keys, which
// this tool takes from the OpenSSH manual.
func makeArguments(spec keySpec) []string {
	switch spec.Kind {
	case kindYubiKey:
		arguments := []string{
			"-t", "ed25519-sk", "-O", "resident", "-O", "verify-required", "-O", "application=" + spec.Application,
			"-O", "user=" + spec.Label,
		}
		if spec.Attestation != "" {
			arguments = append(arguments, "-O", "write-attestation="+spec.Attestation)
		}
		return append(arguments, "-N", "", "-C", spec.Label, "-f", spec.File)
	case kindMac:
		return []string{
			"-t", "ecdsa-sk", "-w", macProvider, "-O", "application=" + spec.Application,
			"-N", "", "-C", spec.Label, "-f", spec.File,
		}
	default:
		// ssh-keygen asks for a passphrase itself.
		return []string{"-t", "ed25519", "-a", "100", "-C", spec.Label, "-f", spec.File}
	}
}

// signArguments are ssh-keygen's arguments to sign (OpenSSH 8.0 and later).
func signArguments(spec signSpec) []string {
	return []string{"-Y", "sign", "-n", spec.Namespace, "-f", spec.KeyFile, spec.Payload}
}

// Make runs ssh-keygen to make spec's key.
func (sshKeygen) Make(spec keySpec) error {
	return runKeygen(nil, makeArguments(spec))
}

// Sign runs ssh-keygen to sign spec's payload; the Mac's provider is named in the environment, which
// is how ssh-keygen -Y sign is told which library holds a security key.
func (sshKeygen) Sign(spec signSpec) error {
	var environment []string
	if spec.Kind == kindMac {
		environment = []string{"SSH_SK_PROVIDER=" + macProvider}
	}
	return runKeygen(environment, signArguments(spec))
}

// runKeygen runs ssh-keygen with the owner's terminal and environment, extra added.
func runKeygen(extra, arguments []string) error {
	//nolint:gosec // a fixed program with the owner's own arguments
	command := exec.CommandContext(context.Background(), "ssh-keygen", arguments...)
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	command.Env = append(os.Environ(), extra...)
	if err := command.Run(); err != nil {
		return fmt.Errorf("ssh-keygen did not finish: %w", err)
	}
	return nil
}
