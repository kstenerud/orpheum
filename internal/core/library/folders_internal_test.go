// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"fmt"
	"testing"

	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// A folder's page holds its folders first and then its videos, from the offset, at most the limit of
// them across both, every one from the offset when the limit is 0 (D432 §3).
func TestFolderPage_RunsOnFromTheFoldersIntoTheVideos(t *testing.T) {
	t.Parallel()
	subfolders := []Subfolder{{Name: "a"}, {Name: "b"}, {Name: "c"}}
	videos := []placedVideo{
		{row: sqlcgen.ListOtherVideosOfLibraryRow{WorkID: 1}}, {row: sqlcgen.ListOtherVideosOfLibraryRow{WorkID: 2}},
	}
	for _, each := range []struct {
		offset, limit int64
		want          string
	}{
		{0, 2, "[a b] []"},
		{2, 2, "[c] [1]"},
		{4, 2, "[] [2]"},
		{6, 2, "[] []"},
		{1, 0, "[b c] [1 2]"},
		{0, 500, "[a b c] [1 2]"},
	} {
		folders, page := folderPage(subfolders, videos, each.offset, each.limit)
		names, ids := make([]string, 0, len(folders)), make([]int64, 0, len(page))
		for _, folder := range folders {
			names = append(names, folder.Name)
		}
		for _, video := range page {
			ids = append(ids, video.row.WorkID)
		}
		if got := fmt.Sprint(names, " ", ids); got != each.want {
			t.Errorf("the page from %d of %d = %s; want %s", each.offset, each.limit, got, each.want)
		}
	}
}
