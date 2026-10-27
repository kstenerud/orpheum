// SPDX-License-Identifier: AGPL-3.0-or-later

package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"

	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/readonly"
)

// wholeHashReadBytes is how much a whole-file hash reads at a time: each read is movement, so a read
// a disk stops answering is a stall within the node's read window, and the job's memory stays small.
// A justified constant: the size of a read is no one's choice.
const wholeHashReadBytes = 1 << 20

// hashWholeWork reads a file whole for the SHA-256 of its every byte, D116's whole-file pass run for a
// candidate copy alone (D363 §2), in the node's own process: each read is movement, and a reading its
// disk stops answering is answered as a stall, left to end on its own (D233 §10).
func (agent *Agent) hashWholeWork(
	ctx context.Context, nodeID int64, held attempt, command nodeproto.HashWhole,
) heldWork {
	return agent.answeringWork(ctx, nodeID, answeringJob{
		attempt: held, jobKind: "a whole-file hash", reading: agent.gateRootOf(command.File.RootPath),
		stalledAnswer: func(stall nodeproto.Stall) nodeproto.Result {
			return nodeproto.Result{WholeHash: &nodeproto.WholeHash{Stall: &stall}}
		},
		makeAnswer: func(jobCtx context.Context) (nodeproto.Result, string) {
			answer := agent.hashWhole(jobCtx, held, command)
			return nodeproto.Result{WholeHash: &answer}, answer.Refusal + answer.Failure + stallDetail(answer.Stall)
		},
	})
}

// hashWhole reads the file named, checked to be the one the core named (D100), through, and says the
// SHA-256 of its bytes with its size and modified time as it was opened, or why it could not.
func (agent *Agent) hashWhole(ctx context.Context, held attempt, command nodeproto.HashWhole) nodeproto.WholeHash {
	file, facts, err := agent.openNamed(ctx, command.File)
	if stall := stallOf(err); stall != nil {
		return nodeproto.WholeHash{Stall: stall}
	}
	if err != nil {
		return nodeproto.WholeHash{Refusal: err.Error(), RefusalCode: refusalCodeOf(err)}
	}
	defer readonly.Close(file)
	hash := sha256.New()
	buffer := make([]byte, wholeHashReadBytes)
	for {
		if err := agent.work.waitWhilePaused(ctx, held.token); err != nil {
			return nodeproto.WholeHash{Refusal: err.Error()}
		}
		read, readErr := file.Read(buffer)
		execx.Moved(ctx)
		hash.Write(buffer[:read])
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return nodeproto.WholeHash{Failure: readErr.Error()}
		}
	}
	return nodeproto.WholeHash{
		SHA256: hex.EncodeToString(hash.Sum(nil)), SizeBytes: facts.Size(), ModifiedAt: facts.ModTime().UnixMilli(),
	}
}
