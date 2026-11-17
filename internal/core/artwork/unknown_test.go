// SPDX-License-Identifier: AGPL-3.0-or-later

package artwork

import (
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/jobs/jobstest"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// A size made on request that the node refused to cut, for a cause it does not name as one that
// blocks the size (an encode that failed, a probe that timed out), is not noted as one that cannot be
// made: the job fails, and is tried again, and a later request asks again (D508 §2).
func TestHandle_ASizeTheNodeRefusedToCutIsTriedAgainAndNotNoted(t *testing.T) {
	t.Parallel()
	source := []byte("poster")
	bench := newBench(t, fakeFetcher{"/poster.jpg": source})
	queue := &jobstest.Queue{}
	bench.service.parts.Queue = queue
	sourceSha256Hex := bench.storePoster(t, source, nodeproto.PortraitS, nodeproto.PortraitM)
	if _, err := bench.service.Serve(t.Context(), sourceSha256Hex, nodeproto.PortraitL, FormatWebP); err != nil {
		t.Fatal(err)
	}
	if len(*queue) != 1 {
		t.Fatalf("queued %+v; want portrait-l's job", *queue)
	}
	job := jobs.Job{
		ID: 2, Kind: jobs.KindMakeArtworkSizes, Target: jobs.Target{ItemID: bench.itemID},
		ArgsJSON: (*queue)[0].ArgsJSON,
	}
	bench.node.answer = func(nodeproto.Artwork) nodeproto.ArtworkResult {
		return nodeproto.ArtworkResult{Refusal: "encoding portrait-l: signal: killed"}
	}
	if err := bench.run(t, job); err == nil || !strings.Contains(err.Error(), "signal: killed") {
		t.Errorf("the job answered %v; want it failed with the node's refusal, to be tried again", err)
	}
	if _, isNoted := bench.art[sourceSha256Hex+"/portrait-l.none"]; isNoted {
		t.Fatalf("stored %v; want portrait-l not noted as one that cannot be made", storedNames(bench.art))
	}
	*queue = nil
	if _, err := bench.service.Serve(t.Context(), sourceSha256Hex, nodeproto.PortraitL, FormatWebP); err != nil {
		t.Fatal(err)
	}
	if len(*queue) != 1 {
		t.Errorf("a later request queued %+v; want portrait-l asked for again", *queue)
	}
}

// A node's answer with no size that was asked for, in WebP, or with no hash to store it under, is no
// fact of the image but a fault: the job fails and is tried again, and nothing is recorded that would
// keep the image from being fetched again (D508 §2).
func TestHandle_AnAnswerWithNothingToStoreIsTriedAgainAndNotRecordedAsRefused(t *testing.T) {
	t.Parallel()
	for name, answer := range map[string]nodeproto.ArtworkResult{
		"no size asked for": {Images: []nodeproto.ArtImage{{SizeName: nodeproto.LandscapeS, WebP: ourWebP}}},
		"no WebP":           {Images: []nodeproto.ArtImage{{SizeName: nodeproto.PortraitS, WebP: []byte("jpeg")}}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			bench := newBench(t, fakeFetcher{"/poster.jpg": []byte("poster")})
			bench.node.answer = func(nodeproto.Artwork) nodeproto.ArtworkResult { return answer }
			if err := bench.run(t, bench.job(Ref{Kind: library.ArtPoster, SourceRef: "/poster.jpg"})); err == nil {
				t.Error("the job succeeded; want it failed, to be tried again")
			}
			if rows := bench.rows(t); len(rows) != 0 {
				t.Errorf("rows %+v; want none, so the poster is fetched again", rows)
			}
			sent := len(bench.node.sent)
			stored := nodeproto.ArtworkResult{
				Images: []nodeproto.ArtImage{{SizeName: nodeproto.PortraitS, WebP: ourWebP}},
			}
			bench.node.answer = func(nodeproto.Artwork) nodeproto.ArtworkResult { return stored }
			bench.handle(t, Ref{Kind: library.ArtPoster, SourceRef: "/poster.jpg"})
			if rows := bench.rows(t); len(bench.node.sent) == sent || len(rows) != 1 || rows[0].SourceSha256 == nil {
				t.Errorf("tried again: rows %+v; want the poster stored", rows)
			}
		})
	}
}
