// SPDX-License-Identifier: AGPL-3.0-or-later

package webclient

import (
	"testing"
	"testing/fstest"
)

func TestBuiltClient_IsTheClientOnlyWhenItHasAnIndex(t *testing.T) {
	t.Parallel()
	placeholder := fstest.MapFS{"dist/.keep": {}}
	if _, found := builtClient(placeholder); found {
		t.Error("a dist holding only the placeholder counts as a built client")
	}
	built := fstest.MapFS{"dist/.keep": {}, "dist/index.html": {Data: []byte("<!doctype html>")}}
	files, found := builtClient(built)
	if !found {
		t.Fatal("a dist with index.html does not count as a built client")
	}
	if _, err := files.Open("index.html"); err != nil {
		t.Errorf("the client's files are not rooted at dist: %v", err)
	}
}

// The client's own script is named from its index.html, so a page running another can tell it is
// out of date; a client without one has no name.
func TestScriptName_IsTheBuiltClientsOwnScript(t *testing.T) {
	t.Parallel()
	built := fstest.MapFS{"index.html": {Data: []byte(
		`<head><script type="module" crossorigin src="/assets/index-DCLi77_b.js"></script>` +
			`<link rel="stylesheet" href="/assets/index-u_38TrwO.css"></head>`,
	)}}
	if name := ScriptName(built); name != "index-DCLi77_b.js" {
		t.Errorf("the script is named %q, want index-DCLi77_b.js", name)
	}
	if name := ScriptName(fstest.MapFS{"index.html": {Data: []byte("<!doctype html>")}}); name != "" {
		t.Errorf("a page with no script names %q, want none", name)
	}
	if name := ScriptName(fstest.MapFS{}); name != "" {
		t.Errorf("no index.html names %q, want none", name)
	}
}
