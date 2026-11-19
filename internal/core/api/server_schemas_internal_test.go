// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"
)

// A later server serves the document a server built on new schemas serves, byte for byte: its
// operations, its paths and the schemas it validates requests with, opened as the first's are.
func TestNewServer_ALaterServerServesTheDocumentOfOneBuiltAnew(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.DiscardHandler)
	NewServer(logger)
	later, err := json.Marshal(NewServer(logger).OpenAPI())
	if err != nil {
		t.Fatal(err)
	}
	anew, err := json.Marshal(newServer(logger, nil).OpenAPI())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(later, anew) {
		t.Errorf("a later server's document (%d bytes) differs from one built anew (%d bytes)", len(later), len(anew))
	}
}

// Every server in a process shares the schemas the first built, rather than building them again:
// most of what building a server costs (test-weight.md §37).
func TestNewServer_ALaterServerSharesTheFirstsSchemas(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.DiscardHandler)
	ref := schemaRefPrefix + "LibraryBody"
	first := NewServer(logger).OpenAPI().Components.Schemas.SchemaFromRef(ref)
	later := NewServer(logger).OpenAPI().Components.Schemas.SchemaFromRef(ref)
	if first == nil || later != first {
		t.Error("a later server built its schemas again")
	}
}
