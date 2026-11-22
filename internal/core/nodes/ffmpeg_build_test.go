// SPDX-License-Identifier: AGPL-3.0-or-later

package nodes

import (
	"context"
	"encoding/json"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store/rowstest"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// forgottenOfFiles records the files a node's new FFmpeg asked to forget the failures of: those whose
// subtitle texts, those forgotten in the write that keeps the build, and how often the passes were
// asked to look again.
type forgottenOfFiles struct {
	subtitles, inWrite [][]int64
	sweeps             int
}

func (forgotten *forgottenOfFiles) ForgetFFmpegSubtitleFailures(_ context.Context, fileIDs []int64) error {
	forgotten.subtitles = append(forgotten.subtitles, fileIDs)
	return nil
}

func (forgotten *forgottenOfFiles) ForgetFFmpegFailuresOfFiles(
	_ context.Context, _ *sqlcgen.Queries, fileIDsJSON string,
) (int64, error) {
	var ids []int64
	if err := json.Unmarshal([]byte(fileIDsJSON), &ids); err != nil {
		return 0, err
	}
	forgotten.inWrite = append(forgotten.inWrite, ids)
	return int64(len(ids)), nil
}

// ffmpegBench is a node channel that forgets into a record, the node "nas" enrolled by a hello with
// FFmpeg build "build-a", holding files 11 and 12 at live places and file 13 at a removed one, and
// the node "attic", holding file 21.
type ffmpegBench struct {
	*fixture
	forgotten *forgottenOfFiles
}

func newFFmpegBench(t *testing.T) ffmpegBench {
	t.Helper()
	bench := ffmpegBench{fixture: newFixture(t, time.Millisecond), forgotten: &forgottenOfFiles{}}
	bench.service.SetFFmpegFailures(bench.forgotten, func() { bench.forgotten.sweeps++ })
	nas := bench.hello(t, nodeproto.Hello{Name: "nas", FFmpegSha256: "build-a"}).NodeID
	attic := bench.hello(t, nodeproto.Hello{Name: "attic", FFmpegSha256: "build-a"}).NodeID
	storetest.ExecRaw(t, bench.database.Path(),
		rowstest.File(rowstest.FileSpec{ID: 11, NodeID: nas, Path: "/m/a.mkv"}),
		rowstest.File(rowstest.FileSpec{ID: 12, NodeID: nas, Path: "/m/b.mkv"}),
		rowstest.File(rowstest.FileSpec{ID: 13, NodeID: nas, Path: "/m/c.mkv", RemovedAt: 5}),
		rowstest.File(rowstest.FileSpec{ID: 21, NodeID: attic, Path: "/m/d.mkv"}))
	if bench.forgotten.inWrite != nil || bench.forgotten.subtitles != nil || bench.forgotten.sweeps != 0 {
		t.Fatalf("the nodes' first hellos forgot %+v; want nothing", *bench.forgotten)
	}
	return bench
}

// keptBuild is the FFmpeg build kept for the node called name.
func (bench ffmpegBench) keptBuild(t *testing.T, name string) []string {
	t.Helper()
	return storetest.QueryRaw(t, bench.database.Path(),
		"SELECT coalesce(ffmpeg_sha256, 'none') FROM node WHERE name = '"+name+"'")
}

// D535 §9: a node that says hello with another FFmpeg build than it last said forgets the failures
// FFmpeg made of the files at a live place on it, its subtitle texts' and the rest in the write that
// keeps the new build, and asks the passes to try each again; another node's files are not its.
func TestHello_AnotherFFmpegForgetsTheFailuresOfTheNodesFilesAlone(t *testing.T) {
	t.Parallel()
	bench := newFFmpegBench(t)
	bench.hello(t, nodeproto.Hello{Name: "nas", FFmpegSha256: "build-b"})
	want := [][]int64{{11, 12}}
	if !slices.EqualFunc(bench.forgotten.inWrite, want, slices.Equal) ||
		!slices.EqualFunc(bench.forgotten.subtitles, want, slices.Equal) {
		t.Errorf("another FFmpeg forgot, in its write, the failures of %v, and the subtitle texts of %v; want "+
			"%v of each: the files at a live place on the node, no other node's", bench.forgotten.inWrite,
			bench.forgotten.subtitles, want)
	}
	if bench.forgotten.sweeps != 1 {
		t.Errorf("the passes were asked to look again %d times; want once", bench.forgotten.sweeps)
	}
	if kept := bench.keptBuild(t, "nas"); !slices.Equal(kept, []string{"build-b"}) {
		t.Errorf("the node's kept build is %v; want build-b", kept)
	}
	if kept := bench.keptBuild(t, "attic"); !slices.Equal(kept, []string{"build-a"}) {
		t.Errorf("the other node's kept build is %v; want build-a, its own", kept)
	}
}

// D535 §9: the same build said again, a hello that says none, and a node's first build forget nothing;
// one that says none keeps the build last said, so the same build after it forgets nothing either.
func TestHello_TheSameFFmpegNoneOrAFirstForgetsNothing(t *testing.T) {
	t.Parallel()
	bench := newFFmpegBench(t)
	bench.hello(t, nodeproto.Hello{Name: "nas", FFmpegSha256: "build-a"})
	bench.hello(t, nodeproto.Hello{Name: "nas"})
	if kept := bench.keptBuild(t, "nas"); !slices.Equal(kept, []string{"build-a"}) {
		t.Errorf("after a hello that says no build, the node's kept build is %v; want build-a", kept)
	}
	bench.hello(t, nodeproto.Hello{Name: "nas", FFmpegSha256: "build-a"})
	storetest.ExecRaw(t, bench.database.Path(), rowstest.Node(rowstest.NodeSpec{Name: "den"}))
	den, err := strconv.ParseInt(
		storetest.QueryRaw(t, bench.database.Path(), "SELECT id FROM node WHERE name = 'den'")[0], 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	storetest.ExecRaw(t, bench.database.Path(),
		rowstest.File(rowstest.FileSpec{ID: 31, NodeID: den, Path: "/m/e.mkv"}))
	bench.hello(t, nodeproto.Hello{Name: "den", FFmpegSha256: "build-c"})
	if bench.forgotten.inWrite != nil || bench.forgotten.subtitles != nil || bench.forgotten.sweeps != 0 {
		t.Errorf("the same build, none and a first build forgot %+v; want nothing", *bench.forgotten)
	}
	if kept := bench.keptBuild(t, "den"); !slices.Equal(kept, []string{"build-c"}) {
		t.Errorf("a node's first build kept is %v; want build-c", kept)
	}
}
