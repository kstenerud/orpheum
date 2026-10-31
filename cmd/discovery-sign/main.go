// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"

	"github.com/kstenerud/orpheum/internal/platform/clock"
)

// Exit statuses: a usage error is told apart from a signing refused.
const (
	exitSuccess = 0
	exitFailure = 1
	exitUsage   = 2
)

func main() {
	signers, closeAgent := agentSigners(os.Getenv("SSH_AUTH_SOCK"))
	status := run(os.Args[1:], clock.System{}.Now(), signers, os.Stdout, os.Stderr)
	closeAgent()
	os.Exit(status)
}

// agentSigners are the keys the ssh-agent at socket holds, a YubiKey's resident keys among them once
// `ssh-add -K` loaded them: signing with one asks the key for a touch. With no agent, none.
func agentSigners(socket string) (func() ([]ssh.Signer, error), func()) {
	if socket == "" {
		return func() ([]ssh.Signer, error) { return nil, errNoAgent }, func() {}
	}
	// The owner's own agent, at the socket their session names.
	connection, err := (&net.Dialer{}).DialContext(context.Background(), "unix", socket)
	if err != nil {
		return func() ([]ssh.Signer, error) { return nil, fmt.Errorf("reach the ssh-agent: %w", err) }, func() {}
	}
	return agent.NewClient(connection).Signers, func() { _ = connection.Close() } //nolint:errcheck // exiting
}

// errNoAgent is a run with no ssh-agent to sign with.
var errNoAgent = errors.New("no ssh-agent (SSH_AUTH_SOCK): start one, and load your YubiKey's keys with ssh-add -K")

// write writes text and returns status, or a failure when text could not be written.
func write(writer io.Writer, text string, status int) int {
	if _, err := io.WriteString(writer, text); err != nil {
		return exitFailure
	}
	return status
}

// usage is how the command is run.
const usage = `usage: discovery-sign <what> [flags]

  root-set    --sets <file> [--add <key>]... [--remove <key>]...
              the next root key set, signed by a key of the set before it (by one of its own, the first)
  delegation  --sets <file> --issuer <origin> --key <online key> --for <duration> --out <file>
  document    --sets <file> --issuer <origin> --core-api <url> --client-api <url> --loader <origin>
              --client-origins <pattern> [--account <origin>] [--homepage <url>] [--support <address>]
              [--location <url>]... --out <file>
  record      --sets <file> --record <unsigned record> --out <file>
  seal        --sets <file> --kind <kind> --payload <file> --signature <file.sig> --out <file>

A key is an authorised key's line or a .pub file. Each signs with the first root key the ssh-agent
holds, or a --key-file names: plug in any one of your YubiKeys and load it with ssh-add -K.
`
