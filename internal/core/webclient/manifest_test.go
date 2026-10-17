// SPDX-License-Identifier: AGPL-3.0-or-later

package webclient

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"testing"
	"testing/fstest"
)

// The manifest names the build, the client's script, and every file with its size and SHA-256, in
// path order, the same bytes each time (D308 §6).
func TestManifestOf_NamesEveryFileByItsHashInPathOrder(t *testing.T) {
	t.Parallel()
	files := fstest.MapFS{
		"index.html":                {Data: []byte(`<script type="module" src="/assets/index-AbC_1.js"></script>`)},
		"assets/index-AbC_1.js":     {Data: []byte("console.log(1)")},
		"assets/worker-Zz.js":       {Data: []byte("w")},
		"sw.js":                     {Data: []byte("sw")},
		"assets/not-a-file/.keep/x": {Data: []byte("x")},
	}
	encoded, err := ManifestOf(files, "v0.2.0-nas.9")
	if err != nil {
		t.Fatal(err)
	}
	again, err := ManifestOf(files, "v0.2.0-nas.9")
	if err != nil || string(again) != string(encoded) {
		t.Fatalf("a second manifest of the same client is %s (%v); want the same bytes", again, err)
	}
	var manifest Manifest
	if err := json.Unmarshal(encoded, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Version != "v0.2.0-nas.9" || manifest.Script != "index-AbC_1.js" || len(manifest.Files) != 5 {
		t.Fatalf("manifest = %+v; want the build, the script and five files", manifest)
	}
	want := []string{"assets/index-AbC_1.js", "assets/not-a-file/.keep/x", "assets/worker-Zz.js", "index.html", "sw.js"}
	for index, file := range manifest.Files {
		digest := sha256.Sum256(files[file.Path].Data)
		if file.Path != want[index] || file.SizeBytes != len(files[file.Path].Data) ||
			file.SHA256 != base64.RawURLEncoding.EncodeToString(digest[:]) {
			t.Errorf("file %d = %+v; want %s with its size and SHA-256", index, file, want[index])
		}
	}
}
