// SPDX-License-Identifier: GPL-3.0-or-later

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
