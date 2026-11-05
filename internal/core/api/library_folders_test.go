// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// ListFolders answers as a node's byte socket would: the folders in the folder asked for, as the
// fake's files hold them, a folder per name under it.
func (fake *fakeNodeBytes) ListFolders(_ context.Context, _ int64, path string) (nodeproto.FolderListing, error) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	listing := nodeproto.FolderListing{Path: path, State: nodeproto.ProbeReadable, Folders: []string{}}
	for target := range fake.files {
		if name, isFolder := strings.CutPrefix(target, "folder:"+path+"/"); isFolder && !strings.Contains(name, "/") {
			listing.Folders = append(listing.Folders, name)
		}
	}
	slices.Sort(listing.Folders)
	return listing, nil
}

// An admin browses a node's folders inside its configured roots, and nothing outside them; a person
// who is not an admin browses none (D359 §2).
func TestLibraries_AnAdminBrowsesTheFoldersInsideANodesRoots(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.connectNode(t, "/media")
	harness.nodeBytes.mutex.Lock()
	harness.nodeBytes.files["folder:/media/video"] = ""
	harness.nodeBytes.files["folder:/media/video/movies"] = ""
	harness.nodeBytes.files["folder:/media/video/shows"] = ""
	harness.nodeBytes.mutex.Unlock()
	folders := func(path string) string {
		return "/nodes/local/folders?limit=500&" + url.Values{"path": {path}}.Encode()
	}
	listed := harness.do(t, http.MethodGet, folders("/media/video"), admin, nil)
	if listed.status != http.StatusOK ||
		!strings.Contains(listed.body, `"state":"readable","folders":["movies","shows"]`) {
		t.Errorf("GET the folders in /media/video = %d %s; want movies and shows", listed.status, listed.body)
	}
	var paged []string
	cursor := ""
	for range 4 {
		var page api.NodeFoldersOutput
		harness.readJSON(t, "/nodes/local/folders?limit=1&path=/media/video&cursor="+url.QueryEscape(cursor),
			admin, &page.Body)
		paged = append(paged, page.Body.Folders...)
		if cursor = page.Body.NextCursor; cursor == "" {
			break
		}
	}
	if !slices.Equal(paged, []string{"movies", "shows"}) || cursor != "" {
		t.Errorf("the folders in /media/video a page of one at a time = %v, then %q; want movies and shows",
			paged, cursor)
	}
	for _, outside := range []string{"/etc", "/media/../etc"} {
		wantInvalidAt(t, harness.do(t, http.MethodGet, folders(outside), admin, nil),
			http.StatusUnprocessableEntity, "query.path")
	}
	if _, err := harness.authService.CreateUser(t.Context(),
		auth.NewUser{Name: "partner", Password: "partner-password", Role: auth.RoleUser}); err != nil {
		t.Fatal(err)
	}
	wantError(t, harness.do(t, http.MethodGet, folders("/media"), harness.signIn(t, "partner", "partner-password"),
		nil), http.StatusForbidden, api.CodeForbidden)
}

// A library is made over a folder inside a node's root, and a folder is added to it, which is
// scanned; one beneath a folder of the same library, or outside every root, is refused (D359 §1,
// §3); another library may cover the same folders (D363 §1).
func TestLibraries_AnAdminAddsAFolderInsideANodesRootToALibrary(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.connectNode(t, "/media")
	created := harness.do(t, http.MethodPost, "/libraries", admin, newShows("/media/video/shows"))
	if created.status != http.StatusCreated {
		t.Fatalf("POST /libraries over a folder inside a root = %d %s", created.status, created.body)
	}
	added := harness.do(t, http.MethodPost, "/libraries/1/roots", admin,
		map[string]any{"nodeName": "local", "path": "/media/video/anime"})
	if added.status != http.StatusAccepted {
		t.Fatalf("POST /libraries/1/roots = %d %s", added.status, added.body)
	}
	if want := []string{"/media/video/shows", "/media/video/anime"}; !slices.Equal(harness.nodes.scanned, want) {
		t.Errorf("scans asked for of %v, want %v", harness.nodes.scanned, want)
	}
	for _, refused := range []string{"/media/video/shows/Dark", "/etc"} {
		wantInvalidAt(t, harness.do(t, http.MethodPost, "/libraries/1/roots", admin,
			map[string]any{"nodeName": "local", "path": refused}), http.StatusUnprocessableEntity, "body.path")
	}
	overlapping := harness.do(t, http.MethodPost, "/libraries", admin, map[string]any{
		"name": "Films", "kind": "movie", "metadataSource": "tmdb", "changeDetection": "manual",
		"isWritable": false, "nodeName": "local", "rootPaths": []string{"/media/video"},
	})
	if overlapping.status != http.StatusCreated {
		t.Errorf("POST /libraries over the folder the first library's are in = %d %s; want it made",
			overlapping.status, overlapping.body)
	}
	wantError(t, harness.do(t, http.MethodPost, "/libraries/9/roots", admin,
		map[string]any{"nodeName": "local", "path": "/media/music"}), http.StatusNotFound, api.CodeNotFound)
	listed := harness.do(t, http.MethodGet, "/libraries", admin, nil)
	if !strings.Contains(listed.body, `"rootPath":"/media/video/anime"`) {
		t.Errorf("GET /libraries = %s; want the folder added among the library's", listed.body)
	}
}
