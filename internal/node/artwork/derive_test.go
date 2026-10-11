// SPDX-License-Identifier: AGPL-3.0-or-later

package artwork

import (
	"slices"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/testutil"
)

func names(sizes []plannedSize) []nodeproto.ArtSizeName {
	var list []nodeproto.ArtSizeName
	for _, size := range sizes {
		list = append(list, size.name)
	}
	return list
}

func TestPlan_NeverUpscalesAndAlwaysMakesTheSmallest(t *testing.T) {
	t.Parallel()
	all := []nodeproto.ArtSizeName{"portrait-s", "portrait-m", "portrait-l", "landscape-l", "logo-s", "logo-m"}
	for _, check := range []struct {
		what          string
		shape         nodeproto.Shape
		width, height int
		want          []nodeproto.ArtSizeName
	}{
		{
			"a TMDB original poster", nodeproto.ShapePortrait, 2000, 3000,
			[]nodeproto.ArtSizeName{"portrait-s", "portrait-m", "portrait-l"},
		},
		{
			"a 600×900 poster: no 960×1440", nodeproto.ShapePortrait, 600, 900,
			[]nodeproto.ArtSizeName{"portrait-s", "portrait-m"},
		},
		{
			"a thumbnail smaller than every size", nodeproto.ShapePortrait, 100, 150,
			[]nodeproto.ArtSizeName{"portrait-s"},
		},
		{
			"only the sizes asked for, of the shape", nodeproto.ShapeLandscape, 1920, 1080,
			[]nodeproto.ArtSizeName{"landscape-l"},
		},
		{"a small logo", nodeproto.ShapeLogo, 300, 100, []nodeproto.ArtSizeName{"logo-s"}},
	} {
		if got := names(plan(check.shape, all, check.width, check.height)); !slices.Equal(got, check.want) {
			t.Errorf("%s: planned %v, want %v", check.what, got, check.want)
		}
	}
}

func TestDerive_FFmpegReadsTheSourceOnlyFromThePipe(t *testing.T) {
	t.Parallel()
	fake := testutil.InstallFakeFFmpeg(t, testutil.FakeReply{Stdout: `{"streams":[{"width":600,"height":900}]}`})
	runner, err := execx.NewRunner(fake.Directory)
	if err != nil {
		t.Fatal(err)
	}
	deriver := NewDeriver(runner, time.Minute)
	deriver.Derive(t.Context(), nodeproto.Artwork{
		SourceImage: []byte("the source"), Shape: nodeproto.ShapePortrait, MaxSourcePixels: 1 << 30, WebPQuality: 82,
		SizeNames: []nodeproto.ArtSizeName{"portrait-s"},
	})
	calls := fake.Calls(t)
	if len(calls) != 2 {
		t.Fatalf("%d runs, want a probe and one encode", len(calls))
	}
	for _, call := range calls {
		pipeOnly := slices.Contains(call.Arguments, "pipe:0") &&
			slices.Index(call.Arguments, "-protocol_whitelist") == slices.Index(call.Arguments, "pipe")-1
		if !pipeOnly || string(call.Stdin) != "the source" {
			t.Errorf("%s ran with %q and read %q; want the pipe protocol only, fed the source",
				call.Tool, call.Arguments, call.Stdin)
		}
	}
}
