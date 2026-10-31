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
	now := clock.System{}.Now()
	if status, isGuided := runGuided(os.Args[1:], now, os.Stdin, os.Stdout, os.Stderr, sshKeygen{}); isGuided {
		os.Exit(status)
	}
	os.Exit(run(os.Args[1:], now, os.Stdout, os.Stderr))
}

// write writes text and returns status, or a failure when text could not be written.
func write(writer io.Writer, text string, status int) int {
	if _, err := io.WriteString(writer, text); err != nil {
		return exitFailure
	}
	return status
}

// usage is how the command is run.
const usage = `usage: discovery-sign ceremony [--folder <dir>] [--software <keysdir>]
       discovery-sign test-key [--dir <dir>]
       discovery-sign sign-release --folder <dir> --record <unsigned record file>
       discovery-sign <what> --ledger <dir> [flags]

ceremony asks plain questions and does day one for both chains (run it again to carry on if stopped);
test-key says whether a key's signatures carry the user-verified flag; sign-release signs a release.
The commands below are what they use, for the cases they do not cover yet.

Each command but check and publish writes a payload to sign, shows it, and says the ssh-keygen command
that signs it with a root key's stub (ssh-keygen -K writes it from the YubiKey). Read every line, then
run that command (one touch, one PIN), then check: it checks the signature by the code a core uses,
keeps both in the ledger, and writes the files to publish.

  genesis   --chain discovery|publisher --everyday <key.pub>... --recovery <key.pub>... [--location <url>]...
  root      --chain discovery|publisher [--add-everyday <key.pub>]... [--add-recovery <key.pub>]...
            [--remove <key.pub>]... [--suspend <key.pub>]... [--resume <key.pub>]... [--skip <n>]
            [--threshold <role>=<n>]... [--location <url>]...
            (an everyday key suspends an everyday key; a recovery key suspends any key, and alone resumes one)
  address   --issuer <origin> --name <name> --core-api <url> --client-api <url> --loader-origin <origin>
            --client-origins <pattern> [--account-origin <origin>] [--homepage <url>] [--support <address>]
            [--location <url>]... [--lasts <duration>]
  drill     --chain discovery|publisher
  check     --kind root|address|release|drill [--chain discovery|publisher] --payload <file> [--publish <dir>]
  publish   --out <dir>
  bootstrap --out <file> [--location <origin>]...
`
