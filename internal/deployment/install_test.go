// SPDX-License-Identifier: AGPL-3.0-or-later

package deployment

import (
	"strings"
	"testing"
)

// The script's behaviour is run against fake commands in scripts/tests/test_install_script.py: a
// test here may start no process (depguard's exec-only-in-execx).
func TestInstallScript_IsAShellScriptInstallingTheImageItIsGiven(t *testing.T) {
	t.Parallel()
	script, err := InstallScript("ghcr.io/example/orpheum:v1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	isScript := strings.HasPrefix(script, "#!/bin/sh\n")
	namesImage := strings.Contains(script, `image="ghcr.io/example/orpheum:v1.2.3"`)
	if !isScript || !namesImage || strings.Contains(script, "{{") {
		t.Errorf("the install script begins %.120q; want a shell script naming the image, no template left", script)
	}
}
