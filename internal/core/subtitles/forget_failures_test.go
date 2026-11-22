// SPDX-License-Identifier: AGPL-3.0-or-later

package subtitles

import (
	"encoding/hex"
	"slices"
	"strconv"
	"testing"

	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// D535 §8: every rescan forgets why the text of a subtitle of its files could not be made, a track of
// its own and one fetched for it; a text that was made stays.
func TestForgetTextFailures_ForgetsEveryFailureOfTheFilesAndKeepsWhatWasMade(t *testing.T) {
	t.Parallel()
	bench, _, fetched := fetchedAndConverted(t)
	path := bench.database.Path()
	ofFile := `(SELECT payload_hash FROM file WHERE id = ` + strconv.FormatInt(bareVideoFileID, 10) + `)`
	storetest.ExecRaw(t, path, `UPDATE subtitle_text SET ass = NULL, vtt = NULL, failure = 'it would not convert'
		WHERE source_hash = X'`+hex.EncodeToString(fetched.SourceHash)+`'`)
	storetest.ExecRaw(t, path, `INSERT INTO subtitle_text (source_hash, stream_index, failure, made_at)
		VALUES (`+ofFile+`, 2, 'ffmpeg exited 1', 1)`)
	storetest.ExecRaw(t, path, `INSERT INTO subtitle_text (source_hash, stream_index, ass, vtt, made_at)
		VALUES (`+ofFile+`, 3, X'01', X'02', 1)`)
	if err := bench.service.ForgetTextFailures(t.Context(), []int64{bareVideoFileID}); err != nil {
		t.Fatal(err)
	}
	if bench.isTextKept(t, fetched) {
		t.Error("the fetched subtitle's failure is kept; want it forgotten")
	}
	kept := storetest.QueryRaw(t, path, `SELECT stream_index FROM subtitle_text WHERE source_hash = `+ofFile+
		` ORDER BY stream_index`)
	if !slices.Equal(kept, []string{"3"}) {
		t.Errorf("the file's own tracks' texts kept are of streams %v; want only 3's, the one made", kept)
	}
}
