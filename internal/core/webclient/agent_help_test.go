// SPDX-License-Identifier: AGPL-3.0-or-later

package webclient_test

import (
	"compress/gzip"
	"io"
	"mime"
	"net/http"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/kstenerud/orpheum/internal/core/webclient"
)

// A person's AI agent that fetches the setup help or /llms.txt, which the client's build writes
// beside its files (web/apps/web/src/agent-help.ts), is answered with the file as text, never with
// the client's page as a screen's address is: the help as Markdown, gzipped like the client's other
// text to an agent that accepts it, whatever the host's mime.types says of `.md` or without one,
// since Go's own table has no Markdown (the plan's "Help for a person's own AI agent at
// orpheum.cc").
func TestHandler_ServesTheAgentsHelpAsText(t *testing.T) {
	t.Parallel()
	// A host whose table types Markdown otherwise: the core's answer must not follow it.
	if err := mime.AddExtensionType(".md", "application/x-host-table"); err != nil {
		t.Fatalf("give .md the host's type: %v", err)
	}
	help := "# Setting Up Orpheum\n\n" + strings.Repeat("- **Never ask for a password.**\n", 64)
	llms := "# Orpheum\n\n- [Setup help for an AI agent](/agent-setup.md)\n"
	files := builtClientFS()
	files["agent-setup.md"] = &fstest.MapFile{Data: []byte(help)}
	files["llms.txt"] = &fstest.MapFile{Data: []byte(llms)}
	handler := webclient.Handler(files, "/api/v1")

	for _, accepting := range []string{"", "gzip"} {
		sent := getAccepting(t, handler, "/agent-setup.md", accepting)
		if got := sent.Header().Get("Content-Type"); sent.Code != http.StatusOK ||
			got != "text/markdown; charset=utf-8" {
			t.Fatalf("the help, accepting %q: %d %q; want 200 text/markdown; charset=utf-8", accepting,
				sent.Code, got)
		}
		body := io.Reader(sent.Body)
		if accepting == "gzip" {
			if sent.Header().Get("Content-Encoding") != "gzip" {
				t.Fatalf("the help to an agent accepting gzip was sent as it is; want it gzipped")
			}
			unpacked, err := gzip.NewReader(sent.Body)
			if err != nil {
				t.Fatalf("the gzipped help: %v", err)
			}
			body = unpacked
		}
		if got, err := io.ReadAll(body); err != nil || string(got) != help {
			t.Errorf("the help, accepting %q: %d bytes (%v); want the help itself", accepting, len(got), err)
		}
	}
	sent := get(t, handler, "/llms.txt")
	if got := sent.Header().Get("Content-Type"); sent.Code != http.StatusOK || got != "text/plain; charset=utf-8" ||
		sent.Body.String() != llms {
		t.Errorf("/llms.txt: %d %q %q; want 200 text/plain naming the help", sent.Code, got, sent.Body.String())
	}
}
