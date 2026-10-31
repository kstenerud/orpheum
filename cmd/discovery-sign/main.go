// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"io"
	"os"

	"github.com/kstenerud/orpheum/internal/platform/clock"
)

// Exit statuses: a usage error is told apart from a statement refused.
const (
	exitSuccess = 0
	exitFailure = 1
	exitUsage   = 2
)

func main() {
	os.Exit(run(os.Args[1:], clock.System{}.Now(), os.Stdout, os.Stderr))
}

// write writes text and returns status, or a failure when text could not be written.
func write(writer io.Writer, text string, status int) int {
	if _, err := io.WriteString(writer, text); err != nil {
		return exitFailure
	}
	return status
}

// usage is how the command is run.
const usage = `usage: discovery-sign <what> --ledger <dir> [flags]

Each command but check and publish writes a payload to sign, shows it, and says the ssh-keygen command
that signs it with a root key's stub (ssh-keygen -K writes it from the YubiKey). Read every line, then
run that command (one touch, one PIN), then check: it checks the signature by the code a core uses,
keeps both in the ledger, and writes the files to publish.

  genesis   --chain discovery|publisher --everyday <key.pub>... --recovery <key.pub>... [--location <url>]...
  root      --chain discovery|publisher [--add-everyday <key.pub>]... [--add-recovery <key.pub>]...
            [--remove <key.pub>]... [--skip <n>] [--threshold <role>=<n>]... [--location <url>]...
  address   --issuer <origin> --name <name> --core-api <url> --client-api <url> --loader-origin <origin>
            --client-origins <pattern> [--account-origin <origin>] [--homepage <url>] [--support <address>]
            [--location <url>]... [--lasts <duration>]
  drill     --chain discovery|publisher
  check     --kind root|address|release|drill [--chain discovery|publisher] --payload <file> [--publish <dir>]
  publish   --out <dir>
  bootstrap --out <file> [--location <origin>]...
`
