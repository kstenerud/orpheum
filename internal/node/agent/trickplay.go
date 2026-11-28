// SPDX-License-Identifier: AGPL-3.0-or-later

package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/node/trickplay"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// Trickplayer makes a video's trickplay sheets, giving each as it is finished: trickplay.Maker.
type Trickplayer interface {
	Make(ctx context.Context, film execx.Film, command nodeproto.MakeTrickplay, sheetMade trickplay.SheetMade) error
}

// trickplayWork makes a video's trickplay sheets in the node's one trickplay slot, posting each to
// the core as it is finished, a piece kept whatever becomes of the run, and then the run's end
// (trickplay.md §3).
func (agent *Agent) trickplayWork(
	ctx context.Context, nodeID int64, held attempt, command nodeproto.MakeTrickplay,
) heldWork {
	return agent.answeringWork(ctx, nodeID, answeringJob{
		attempt: held, jobKind: "a trickplay run",
		stalledAnswer: func(stall nodeproto.Stall) nodeproto.Result {
			return nodeproto.Result{Trickplay: &nodeproto.TrickplayResult{Stall: &stall}}
		},
		makeAnswer: func(jobCtx context.Context) (nodeproto.Result, string) {
			answer := func() nodeproto.TrickplayResult {
				// A run of a quality beside it is no fair measure of its node's speed.
				defer agent.runsGoing.starts(jobCtx).ends()
				return agent.makeTrickplay(jobCtx, nodeID, held, command)
			}()
			return nodeproto.Result{Trickplay: &answer}, answer.Refusal + answer.Failure + stallDetail(answer.Stall)
		},
	})
}

// sheetNotTakenError is a sheet the core did not take: the run ends, and the node's word on it is a
// refusal, not the file's failure.
type sheetNotTakenError struct{ err error }

func (refused sheetNotTakenError) Error() string { return refused.err.Error() }
func (refused sheetNotTakenError) Unwrap() error { return refused.err }

// makeTrickplay makes the sheets of a video checked to be the one the core named (D100), or says why
// not: that FFmpeg could not, which is the file's, or that the node did not try.
func (agent *Agent) makeTrickplay(
	ctx context.Context, nodeID int64, held attempt, command nodeproto.MakeTrickplay,
) nodeproto.TrickplayResult {
	refuse := func(err error) nodeproto.TrickplayResult {
		return nodeproto.TrickplayResult{Refusal: err.Error(), RefusalCode: refusalCodeOf(err)}
	}
	if agent.workers.Trickplayer == nil {
		return refuse(errors.New("this node does not make trickplay pictures"))
	}
	film, closeFilm, err := agent.openFilm(ctx, command.Video)
	if stall := stallOf(err); stall != nil {
		return nodeproto.TrickplayResult{Stall: stall}
	}
	if err != nil {
		return refuse(err)
	}
	defer closeFilm()
	err = agent.workers.Trickplayer.Make(ctx, film, command, func(index int, webp []byte) error {
		// A sheet whose size cannot be read is FFmpeg's output gone wrong: the run's failure.
		pictureHeight, unread := trickplay.PictureHeight(webp, command.SheetRows)
		if unread != nil {
			return fmt.Errorf("sheet %d: %w", index, unread)
		}
		if posted := agent.postSheet(ctx, nodeID, nodeproto.TrickplaySheet{
			JobID: held.jobID, AttemptToken: held.token, Index: index, WebP: webp, PictureHeight: pictureHeight,
		}); posted != nil {
			return sheetNotTakenError{err: posted}
		}
		return nil
	})
	var notTaken sheetNotTakenError
	switch stall := stallOf(err); {
	case stall != nil:
		return nodeproto.TrickplayResult{Stall: stall}
	case errors.As(err, &notTaken):
		return refuse(notTaken)
	case err != nil:
		return nodeproto.TrickplayResult{Failure: err.Error()}
	}
	return nodeproto.TrickplayResult{}
}

// postSheet posts a finished sheet until the core takes it, at the pace a result is retried, FFmpeg
// waiting on its pipe meanwhile; one the core refuses for what it is, as for an attempt it has
// failed, ends the run.
func (agent *Agent) postSheet(ctx context.Context, nodeID int64, sheet nodeproto.TrickplaySheet) error {
	encoded, err := json.Marshal(sheet)
	if err != nil {
		return fmt.Errorf("encode sheet %d: %w", sheet.Index, err)
	}
	for tries := int64(1); ; tries++ {
		wait := agent.answerRetry.waitAfter(tries)
		nextHello := agent.ended.nextHello()
		var accepted struct{}
		err = agent.exchange(ctx, http.MethodPost, nodeproto.PathTrickplaySheet, nodeID, bytes.NewReader(encoded),
			&accepted)
		if err == nil {
			return nil
		}
		if isAnsweredWith(err, nodeproto.StatusResultRejected) {
			return fmt.Errorf("the core refused sheet %d: %w", sheet.Index, err)
		}
		agent.logger.InfoContext(ctx, "the core has not taken a trickplay sheet; trying again", "jobId", sheet.JobID,
			"sheet", sheet.Index, "wait", wait.String(), "err", err)
		if !sleepUntilHelloOrDone(ctx, wait, nextHello) {
			return fmt.Errorf("post sheet %d: %w", sheet.Index, context.Cause(ctx))
		}
	}
}
