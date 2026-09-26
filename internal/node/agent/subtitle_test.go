// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/node/hash"
	"github.com/kstenerud/orpheum/internal/node/subtitle"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// fakeSubtitles answers with text that names what it was given.
type fakeSubtitles struct{}

func (fakeSubtitles) ExtractTrack(_ context.Context, videoPath string, streamIndex int) (subtitle.Text, error) {
	return subtitle.Text{ASS: []byte(videoPath), VTT: []byte(strconv.Itoa(streamIndex))}, nil
}

func (fakeSubtitles) ConvertFile(_ context.Context, source []byte) (subtitle.Text, error) {
	return subtitle.Text{ASS: source, VTT: source}, nil
}

func receiveSubtitle(t *testing.T, core *fakeCore) nodeproto.SubtitleText {
	t.Helper()
	select {
	case answer := <-core.subtitles:
		return answer
	case <-t.Context().Done():
		t.Fatal("no subtitle reached the core")
	}
	return nodeproto.SubtitleText{}
}

func TestAgent_ExtractsATrackOnlyFromTheVideoTheCoreNamedAndASidecarUnderItsRoot(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	root, _ := running(t, socket)
	for name, contents := range map[string]string{"film.mkv": "a film", "film.en.srt": "the sidecar"} {
		if err := os.WriteFile(filepath.Join(root.Path(), name), []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	identity, err := hash.Payload(strings.NewReader("a film"), 6)
	if err != nil {
		t.Fatal(err)
	}
	video := nodeproto.BytesFile{
		Root: root.Path(), Path: []byte("film.mkv"), Hash: hex.EncodeToString(identity.Hash[:]),
	}
	send := func(job int64, extract nodeproto.ExtractSubtitle) nodeproto.SubtitleText {
		extract.Job = job
		core.commands <- nodeproto.Command{Seq: job, Type: nodeproto.CommandExtractSubtitle, ExtractSubtitle: &extract}
		return receiveSubtitle(t, core)
	}

	track := send(1, nodeproto.ExtractSubtitle{Video: &video, StreamIndex: 2})
	if track.Job != 1 || track.Refused != "" || string(track.ASS) != root.Path()+"/film.mkv" ||
		string(track.VTT) != "2" {
		t.Errorf("a track of the named video: %+v", track)
	}
	changed := video
	changed.Hash = strings.Repeat("0", 64)
	refused := send(2, nodeproto.ExtractSubtitle{Video: &changed, StreamIndex: 2})
	if !strings.Contains(refused.Refused, "not the one named") {
		t.Errorf("a track of a changed video: %+v, want refused", refused)
	}

	sidecar := nodeproto.BytesFile{Root: root.Path(), Path: []byte("film.en.srt")}
	converted := send(3, nodeproto.ExtractSubtitle{Sidecar: &sidecar})
	if converted.Refused != "" || string(converted.ASS) != "the sidecar" {
		t.Errorf("a sidecar: %+v", converted)
	}
	outside := nodeproto.BytesFile{Root: t.TempDir(), Path: []byte("film.en.srt")}
	refused = send(4, nodeproto.ExtractSubtitle{Sidecar: &outside})
	if !strings.Contains(refused.Refused, "none of this node's roots") {
		t.Errorf("a sidecar under another root: %+v, want refused", refused)
	}
	escaping := nodeproto.BytesFile{Root: root.Path(), Path: []byte("../film.en.srt")}
	if refused := send(5, nodeproto.ExtractSubtitle{Sidecar: &escaping}); refused.Refused == "" {
		t.Errorf("a sidecar path leaving its root was read: %+v", refused)
	}
	if refused := send(6, nodeproto.ExtractSubtitle{Video: &video, Sidecar: &sidecar}); refused.Refused == "" {
		t.Errorf("a command naming both a track and a sidecar ran: %+v", refused)
	}
}
