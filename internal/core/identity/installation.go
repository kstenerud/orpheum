// SPDX-License-Identifier: AGPL-3.0-or-later

package identity

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/kstenerud/orpheum/internal/core/instancedir"
)

// InstallationFileName is the installation id's file in the instance's persistent directory, beside
// identity.key. Like the key it is never in a backup, so a copy restored on another machine is
// another installation, which a discovery admits only once the last holder of the realm has had its
// window to come back (D437 §5).
const InstallationFileName = "installation-id"

// LoadOrMakeInstallation reads the instance's installation id, making and keeping one the first
// time: random, made once, and kept across the core's restarts, unlike the instance id each run makes
// (D437 §5). A file holding nothing is refused, never replaced.
func LoadOrMakeInstallation(files Files, random io.Reader) (string, error) {
	contents, err := files.ReadPersistentFile(InstallationFileName)
	if err == nil {
		installation := strings.TrimSpace(string(contents))
		if installation == "" {
			return "", errors.New(InstallationFileName + ": holds no installation id")
		}
		return installation, nil
	}
	if !errors.Is(err, instancedir.ErrNotFound) {
		return "", fmt.Errorf("read %s: %w", InstallationFileName, err)
	}
	made := make([]byte, 20)
	if _, err = io.ReadFull(random, made); err != nil {
		return "", fmt.Errorf("make an installation id: %w", err)
	}
	installation := hex.EncodeToString(made)
	if err = files.WritePersistentFile(InstallationFileName, []byte(installation+"\n")); err != nil {
		return "", fmt.Errorf("keep %s: %w", InstallationFileName, err)
	}
	return installation, nil
}
