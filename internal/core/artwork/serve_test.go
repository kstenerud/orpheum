// SPDX-License-Identifier: GPL-3.0-or-later

package artwork

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// stored gives the bench a poster stored at the sizes named, from source, as the item's poster.
func (bench *bench) stored(t *testing.T, source []byte, sizes ...nodeproto.ArtSizeName) string {
	t.Helper()
	bench.node.answer = func(command nodeproto.Artwork) nodeproto.ArtworkResult {
		var images []nodeproto.ArtImage
		for _, size := range sizes {
			images = append(images, nodeproto.ArtImage{SizeName: size, WebP: ourWebP})
		}
		return nodeproto.ArtworkResult{JobID: command.JobID, Images: images}
	}
	bench.handle(t, owedRef{Kind: identify.ArtPoster, Ref: "/poster.jpg"})
	return Received(source).hash()
}

func TestServe_AStoredSizeIsServedAndAnythingElseIsNoArt(t *testing.T) {
	t.Parallel()
	source := []byte("poster")
	bench := newBench(t, fetched{"/poster.jpg": source})
	bench.service.parts.Queue = &queued{}
	hash := bench.stored(t, source, nodeproto.PortraitS)
	served, err := bench.service.Serve(t.Context(), hash, nodeproto.PortraitS, FormatWebP)
	if err != nil || string(served.Bytes) != string(ourWebP) || served.Redirect != "" {
		t.Errorf("a stored size: %+v, %v", served, err)
	}
	for _, request := range []struct {
		hash   string
		size   nodeproto.ArtSizeName
		format string
	}{
		{hash, "portrait-xxl", FormatWebP},
		{hash, nodeproto.PortraitS, "png"},
		{"../state", nodeproto.PortraitS, FormatWebP},
		{strings.Repeat("0", 64), nodeproto.PortraitS, FormatWebP},
		{hash, nodeproto.LandscapeS, FormatWebP}, // no size of that shape
	} {
		if _, err := bench.service.Serve(t.Context(), request.hash, request.size, request.format); !errors.Is(err,
			ErrNoArt) {
			t.Errorf("Serve(%+v) = %v, want ErrNoArt", request, err)
		}
	}
}

func TestServe_ASizeNotMadeRedirectsToTheNearestAndOnlyAnOnRequestSizeIsQueued(t *testing.T) {
	t.Parallel()
	source := []byte("poster")
	bench := newBench(t, fetched{"/poster.jpg": source})
	queue := &queued{}
	bench.service.parts.Queue = queue
	hash := bench.stored(t, source, nodeproto.PortraitS, nodeproto.PortraitM)

	served, err := bench.service.Serve(t.Context(), hash, nodeproto.PortraitL, FormatWebP)
	if err != nil || served.Redirect != "portrait-m.webp" {
		t.Fatalf("portrait-l: %+v, %v; want a redirect to portrait-m", served, err)
	}
	if len(*queue) != 1 || (*queue)[0].Subject != bench.itemID || (*queue)[0].Kind != jobs.KindArtwork {
		t.Fatalf("queued %+v; want one job to make portrait-l", *queue)
	}
	var args owed
	if err := json.Unmarshal([]byte((*queue)[0].Args), &args); err != nil || args.Hash != hash ||
		!slices.Equal(args.Sizes, []nodeproto.ArtSizeName{nodeproto.PortraitL}) || args.Refs[0].Ref != "/poster.jpg" {
		t.Errorf("the job's args %+v (%v)", args, err)
	}

	// A JPEG of a size not made goes to the nearest size's JPEG.
	if served, err := bench.service.Serve(t.Context(), hash, nodeproto.PortraitL, FormatJPEG); err != nil ||
		served.Redirect != "portrait-m.jpeg" {
		t.Errorf("portrait-l JPEG: %+v, %v", served, err)
	}
}

func TestServe_NothingIsQueuedForASizeTheSourceCannotMake(t *testing.T) {
	t.Parallel()
	source := []byte("poster")
	bench := newBench(t, fetched{"/poster.jpg": source})
	queue := &queued{}
	bench.service.parts.Queue = queue
	// Only portrait-s: the source was smaller than portrait-m, so it is smaller than portrait-l.
	hash := bench.stored(t, source, nodeproto.PortraitS)
	for _, size := range []nodeproto.ArtSizeName{nodeproto.PortraitM, nodeproto.PortraitL} {
		if served, err := bench.service.Serve(t.Context(), hash, size, FormatWebP); err != nil ||
			served.Redirect != "portrait-s.webp" {
			t.Errorf("%s: %+v, %v", size, served, err)
		}
	}
	if len(*queue) != 0 {
		t.Errorf("queued %+v for a source too small", *queue)
	}
}

func TestHandle_MakesAnOnRequestSizeAndNotesOneThatCannotBeMade(t *testing.T) {
	t.Parallel()
	source := []byte("poster")
	bench := newBench(t, fetched{"/poster.jpg": source})
	queue := &queued{}
	bench.service.parts.Queue = queue
	hash := bench.stored(t, source, nodeproto.PortraitS, nodeproto.PortraitM)
	if _, err := bench.service.Serve(t.Context(), hash, nodeproto.PortraitL, FormatWebP); err != nil {
		t.Fatal(err)
	}
	job := jobs.Job{ID: 2, Kind: jobs.KindArtwork, Subject: bench.itemID, Args: (*queue)[0].Args}

	// The node finds the source too small for portrait-l: noted, and never queued again.
	bench.node.answer = func(command nodeproto.Artwork) nodeproto.ArtworkResult {
		if !slices.Equal(command.SizeNames, []nodeproto.ArtSizeName{nodeproto.PortraitL}) {
			t.Errorf("asked for %v, want portrait-l only", command.SizeNames)
		}
		return nodeproto.ArtworkResult{JobID: command.JobID}
	}
	if err := bench.service.Handle(t.Context(), job); err != nil {
		t.Fatal(err)
	}
	if _, noted := bench.art[hash+"/portrait-l.none"]; !noted {
		t.Fatalf("stored %v; want portrait-l noted as impossible", keys(bench.art))
	}
	*queue = nil
	_, err := bench.service.Serve(t.Context(), hash, nodeproto.PortraitL, FormatWebP)
	if err != nil || len(*queue) != 0 {
		t.Errorf("after the note: %v, queued %+v", err, *queue)
	}

	// Made: portrait-l is stored under the same hash.
	delete(bench.art, hash+"/portrait-l.none")
	bench.node.answer = func(command nodeproto.Artwork) nodeproto.ArtworkResult {
		return nodeproto.ArtworkResult{JobID: command.JobID, Images: []nodeproto.ArtImage{
			{SizeName: nodeproto.PortraitL, WebP: ourWebP},
		}}
	}
	if err := bench.service.Handle(t.Context(), job); err != nil {
		t.Fatal(err)
	}
	if served, err := bench.service.Serve(t.Context(), hash, nodeproto.PortraitL, FormatWebP); err != nil ||
		served.Redirect != "" {
		t.Errorf("after making it: %+v, %v", served, err)
	}
}

func TestHandle_ASourceThatChangedMakesNoSizeUnderTheOldHash(t *testing.T) {
	t.Parallel()
	images := fetched{"/poster.jpg": []byte("poster")}
	bench := newBench(t, images)
	queue := &queued{}
	bench.service.parts.Queue = queue
	hash := bench.stored(t, []byte("poster"), nodeproto.PortraitS, nodeproto.PortraitM)
	if _, err := bench.service.Serve(t.Context(), hash, nodeproto.PortraitL, FormatWebP); err != nil {
		t.Fatal(err)
	}
	images["/poster.jpg"] = []byte("a different poster at the same path")
	sent := len(bench.node.sent)
	job := jobs.Job{ID: 2, Kind: jobs.KindArtwork, Subject: bench.itemID, Args: (*queue)[0].Args}
	if err := bench.service.Handle(t.Context(), job); err != nil {
		t.Fatal(err)
	}
	if len(bench.node.sent) != sent {
		t.Error("a changed source was re-encoded into the old image's sizes")
	}
	if _, noted := bench.art[hash+"/portrait-l.none"]; !noted {
		t.Errorf("stored %v; want portrait-l noted as not makeable", keys(bench.art))
	}
}
