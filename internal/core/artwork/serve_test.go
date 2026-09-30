// SPDX-License-Identifier: GPL-3.0-or-later

package artwork

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/jobs/jobstest"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// storePoster gives the bench a poster stored at the sizes named, from source, as the item's poster.
func (bench *bench) storePoster(t *testing.T, source []byte, sizes ...nodeproto.ArtSizeName) string {
	t.Helper()
	bench.node.answer = func(command nodeproto.Artwork) nodeproto.ArtworkResult {
		var images []nodeproto.ArtImage
		for _, size := range sizes {
			images = append(images, nodeproto.ArtImage{SizeName: size, WebP: ourWebP})
		}
		return nodeproto.ArtworkResult{JobID: command.JobID, Images: images}
	}
	bench.handle(t, Ref{Kind: library.ArtPoster, SourceRef: "/poster.jpg"})
	return Received(source).sourceSha256Hex()
}

func TestServe_AStoredSizeIsServedAndAnythingElseIsNoArt(t *testing.T) {
	t.Parallel()
	source := []byte("poster")
	bench := newBench(t, fakeFetcher{"/poster.jpg": source})
	bench.service.parts.Queue = &jobstest.Queue{}
	sourceSha256Hex := bench.storePoster(t, source, nodeproto.PortraitS)
	served, err := bench.service.Serve(t.Context(), sourceSha256Hex, nodeproto.PortraitS, FormatWebP)
	if err != nil || string(served.Bytes) != string(ourWebP) || served.RedirectName != "" {
		t.Errorf("a stored size: %+v, %v", served, err)
	}
	for _, request := range []struct {
		sourceSha256Hex string
		size            nodeproto.ArtSizeName
		format          string
	}{
		{sourceSha256Hex, "portrait-xxl", FormatWebP},
		{sourceSha256Hex, nodeproto.PortraitS, "png"},
		{"../state", nodeproto.PortraitS, FormatWebP},
		{strings.Repeat("0", 64), nodeproto.PortraitS, FormatWebP},
		{sourceSha256Hex, nodeproto.LandscapeS, FormatWebP}, // no size of that shape
	} {
		_, err := bench.service.Serve(t.Context(), request.sourceSha256Hex, request.size, request.format)
		if !errors.Is(err, ErrNoArt) {
			t.Errorf("Serve(%+v) = %v, want ErrNoArt", request, err)
		}
	}
}

func TestServe_ASizeNotMadeRedirectsToTheNearestAndOnlyAnOnRequestSizeIsQueued(t *testing.T) {
	t.Parallel()
	source := []byte("poster")
	bench := newBench(t, fakeFetcher{"/poster.jpg": source})
	queue := &jobstest.Queue{}
	bench.service.parts.Queue = queue
	sourceSha256Hex := bench.storePoster(t, source, nodeproto.PortraitS, nodeproto.PortraitM)

	served, err := bench.service.Serve(t.Context(), sourceSha256Hex, nodeproto.PortraitL, FormatWebP)
	if err != nil || served.RedirectName != "portrait-m.webp" {
		t.Fatalf("portrait-l: %+v, %v; want a redirect to portrait-m", served, err)
	}
	if len(*queue) != 1 || (*queue)[0].ItemID != bench.itemID || (*queue)[0].Kind != jobs.KindMakeArtworkSizes {
		t.Fatalf("queued %+v; want one job to make portrait-l", *queue)
	}
	var args artworkJobArgs
	if err := json.Unmarshal([]byte((*queue)[0].ArgsJSON), &args); err != nil || args.SourceSha256 != sourceSha256Hex ||
		!slices.Equal(args.SizeNames,
			[]nodeproto.ArtSizeName{nodeproto.PortraitL}) || args.Refs[0].SourceRef != "/poster.jpg" {
		t.Errorf("the job's args %+v (%v)", args, err)
	}

	// A JPEG of a size not made goes to the nearest size's JPEG.
	if served, err := bench.service.Serve(t.Context(), sourceSha256Hex, nodeproto.PortraitL, FormatJPEG); err != nil ||
		served.RedirectName != "portrait-m.jpeg" {
		t.Errorf("portrait-l JPEG: %+v, %v", served, err)
	}
}

func TestServe_NothingIsQueuedForASizeTheSourceCannotMake(t *testing.T) {
	t.Parallel()
	source := []byte("poster")
	bench := newBench(t, fakeFetcher{"/poster.jpg": source})
	queue := &jobstest.Queue{}
	bench.service.parts.Queue = queue
	// Only portrait-s: the source was smaller than portrait-m, so it is smaller than portrait-l.
	sourceSha256Hex := bench.storePoster(t, source, nodeproto.PortraitS)
	for _, size := range []nodeproto.ArtSizeName{nodeproto.PortraitM, nodeproto.PortraitL} {
		if served, err := bench.service.Serve(t.Context(), sourceSha256Hex, size, FormatWebP); err != nil ||
			served.RedirectName != "portrait-s.webp" {
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
	bench := newBench(t, fakeFetcher{"/poster.jpg": source})
	queue := &jobstest.Queue{}
	bench.service.parts.Queue = queue
	sourceSha256Hex := bench.storePoster(t, source, nodeproto.PortraitS, nodeproto.PortraitM)
	if _, err := bench.service.Serve(t.Context(), sourceSha256Hex, nodeproto.PortraitL, FormatWebP); err != nil {
		t.Fatal(err)
	}
	job := jobs.Job{
		ID: 2, Kind: jobs.KindMakeArtworkSizes, Target: jobs.Target{ItemID: bench.itemID},
		ArgsJSON: (*queue)[0].ArgsJSON,
	}

	// The node finds the source too small for portrait-l: noted, and never queued again.
	bench.node.answer = func(command nodeproto.Artwork) nodeproto.ArtworkResult {
		if !slices.Equal(command.SizeNames, []nodeproto.ArtSizeName{nodeproto.PortraitL}) {
			t.Errorf("asked for %v, want portrait-l only", command.SizeNames)
		}
		if command.JobID != job.ID {
			t.Errorf("the command names job %d, want the job that makes it, %d", command.JobID, job.ID)
		}
		return nodeproto.ArtworkResult{JobID: command.JobID}
	}
	if err := bench.service.Handle(t.Context(), job); err != nil {
		t.Fatal(err)
	}
	if _, noted := bench.art[sourceSha256Hex+"/portrait-l.none"]; !noted {
		t.Fatalf("stored %v; want portrait-l noted as impossible", storedNames(bench.art))
	}
	*queue = nil
	_, err := bench.service.Serve(t.Context(), sourceSha256Hex, nodeproto.PortraitL, FormatWebP)
	if err != nil || len(*queue) != 0 {
		t.Errorf("after the note: %v, queued %+v", err, *queue)
	}

	// Made: portrait-l is stored under the same hash.
	delete(bench.art, sourceSha256Hex+"/portrait-l.none")
	bench.node.answer = func(command nodeproto.Artwork) nodeproto.ArtworkResult {
		return nodeproto.ArtworkResult{JobID: command.JobID, Images: []nodeproto.ArtImage{
			{SizeName: nodeproto.PortraitL, WebP: ourWebP},
		}}
	}
	if err := bench.service.Handle(t.Context(), job); err != nil {
		t.Fatal(err)
	}
	if served, err := bench.service.Serve(t.Context(), sourceSha256Hex, nodeproto.PortraitL, FormatWebP); err != nil ||
		served.RedirectName != "" {
		t.Errorf("after making it: %+v, %v", served, err)
	}
}

func TestHandle_ASourceThatChangedMakesNoSizeUnderTheOldHash(t *testing.T) {
	t.Parallel()
	images := fakeFetcher{"/poster.jpg": []byte("poster")}
	bench := newBench(t, images)
	queue := &jobstest.Queue{}
	bench.service.parts.Queue = queue
	sourceSha256Hex := bench.storePoster(t, []byte("poster"), nodeproto.PortraitS, nodeproto.PortraitM)
	if _, err := bench.service.Serve(t.Context(), sourceSha256Hex, nodeproto.PortraitL, FormatWebP); err != nil {
		t.Fatal(err)
	}
	images["/poster.jpg"] = []byte("a different poster at the same path")
	sent := len(bench.node.sent)
	job := jobs.Job{
		ID: 2, Kind: jobs.KindMakeArtworkSizes, Target: jobs.Target{ItemID: bench.itemID},
		ArgsJSON: (*queue)[0].ArgsJSON,
	}
	if err := bench.service.Handle(t.Context(), job); err != nil {
		t.Fatal(err)
	}
	if len(bench.node.sent) != sent {
		t.Error("a changed source was re-encoded into the old image's sizes")
	}
	if _, noted := bench.art[sourceSha256Hex+"/portrait-l.none"]; !noted {
		t.Errorf("stored %v; want portrait-l noted as not makeable", storedNames(bench.art))
	}
}
