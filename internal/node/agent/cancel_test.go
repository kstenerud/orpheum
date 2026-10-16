// SPDX-License-Identifier: AGPL-3.0-or-later

package agent

import (
	"context"
	"testing"

	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// blockingArtworker re-encodes as fakeArtworker does, but holds an image whose bytes are "hold"
// until its job is cancelled, and says when it starts holding and that it was stopped.
type blockingArtworker struct {
	holding chan struct{}
	stopped chan struct{}
}

func (artworker blockingArtworker) Derive(ctx context.Context, command nodeproto.Artwork) nodeproto.ArtworkResult {
	if string(command.SourceImage) != "hold" {
		return fakeArtworker{}.Derive(ctx, command)
	}
	artworker.holding <- struct{}{}
	<-ctx.Done()
	artworker.stopped <- struct{}{}
	return nodeproto.ArtworkResult{Refusal: "cancelled"}
}

// A cancel stops a running re-encode at once, and nothing is reported for it: the next answer the
// core hears is the next job's.
func TestAgent_AnArtworkJobTheCoreCancelledIsStopped(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	artworker := blockingArtworker{holding: make(chan struct{}, 1), stopped: make(chan struct{}, 1)}
	startAgentWithWorkers(t, socket, Workers{Scanner: fakeScanner{}, Artworker: artworker})
	artwork := func(source string) *nodeproto.Artwork {
		return &nodeproto.Artwork{
			SourceImage: []byte(source), Shape: nodeproto.ShapePortrait,
			SizeNames: []nodeproto.ArtSizeName{nodeproto.PortraitS},
		}
	}
	core.commands <- nodeproto.Command{
		CommandSeq: 1,
		JobID:      12,
		Work:       nodeproto.Work{Type: nodeproto.CommandEncodeArtwork, EncodeArtwork: oneImage(artwork("hold"))},
	}
	testutil.Receive(t, artworker.holding, "job 12's re-encode starting")
	core.commands <- nodeproto.Command{CommandSeq: 2, JobID: 12, Work: nodeproto.Work{Type: nodeproto.CommandCancel}}
	testutil.Receive(t, artworker.stopped, "job 12's re-encode stopped by the cancel")
	core.commands <- nodeproto.Command{
		CommandSeq: 3,
		JobID:      13,
		Work:       nodeproto.Work{Type: nodeproto.CommandEncodeArtwork, EncodeArtwork: oneImage(artwork("abc"))},
	}
	result := testutil.Receive(t, core.artwork, "an artwork answer")
	if result.JobID != 13 {
		t.Errorf("the core heard job %d's answer first, want job 13's: cancelled job 12 reports nothing",
			result.JobID)
	}
	if result.Artwork == nil || len(onlyImage(t, result).Images) != 1 {
		t.Errorf("job 13's answer = %+v, want its one size: the slot job 12 held is free", result.Artwork)
	}
}

// A cancel that comes before its attempt's work, as the core queues it when it bumps a job while
// recording it assigned, keeps the work from running when it comes: the node never holds work no
// job wants (staging, 2026-10-16).
func TestAgent_WorkWhoseCancelCameFirstIsNotRun(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	artworker := blockingArtworker{holding: make(chan struct{}, 1), stopped: make(chan struct{}, 1)}
	startAgentWithWorkers(t, socket, Workers{Scanner: fakeScanner{}, Artworker: artworker})
	artwork := func(source string) *nodeproto.Artwork {
		return &nodeproto.Artwork{
			SourceImage: []byte(source), Shape: nodeproto.ShapePortrait,
			SizeNames: []nodeproto.ArtSizeName{nodeproto.PortraitS},
		}
	}
	core.commands <- nodeproto.Command{CommandSeq: 1, JobID: 12, Work: nodeproto.Work{Type: nodeproto.CommandCancel}}
	core.commands <- nodeproto.Command{
		CommandSeq: 2,
		JobID:      12,
		Work:       nodeproto.Work{Type: nodeproto.CommandEncodeArtwork, EncodeArtwork: oneImage(artwork("hold"))},
	}
	core.commands <- nodeproto.Command{
		CommandSeq: 3,
		JobID:      13,
		Work:       nodeproto.Work{Type: nodeproto.CommandEncodeArtwork, EncodeArtwork: oneImage(artwork("abc"))},
	}
	result := testutil.Receive(t, core.artwork, "an artwork answer")
	if result.JobID != 13 {
		t.Errorf("the core heard job %d's answer first, want job 13's: job 12 was cancelled first", result.JobID)
	}
	select {
	case <-artworker.holding:
		t.Error("job 12's re-encode ran, though its cancel came before it")
	default:
	}
}
