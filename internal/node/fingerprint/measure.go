// SPDX-License-Identifier: AGPL-3.0-or-later

package fingerprint

import (
	"context"
	"encoding/binary"
	"fmt"
	"strconv"
	"time"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/execx"
)

// Runner runs FFmpeg, watched for a stall: execx.Runner.
type Runner interface {
	RunWatched(ctx context.Context, run execx.WatchedRun) (execx.Ran, error)
}

// fingerprintStartGrace is how long a run's position may take to first move (execx.Watch): opening the
// video on a network mount, probing it and seeking to its closing minutes.
const fingerprintStartGrace = 2 * time.Minute

// fingerprintStallWindow is how long a run's position may stand still: every frame of sound decoded
// moves it, so it stands still only while FFmpeg reads nothing; three minutes outlasts an NFS client's
// retries, as a read-through's window does.
const fingerprintStallWindow = 3 * time.Minute

// maxFingerprintBytes is the most a run's fingerprint may be: four bytes a point, eight points a
// second, so a mebibyte is nine hours of sound, past any opening or closing asked for.
const maxFingerprintBytes = 1 << 20

// Fingerprinter fingerprints a stretch of a video's sound with the bundled FFmpeg's chromaprint muxer.
type Fingerprinter struct {
	runner Runner
}

// NewFingerprinter returns the fingerprinter.
func NewFingerprinter(runner Runner) *Fingerprinter {
	return &Fingerprinter{runner: runner}
}

// Fingerprint is the chromaprint fingerprint of length of film's sound from from, the audio stream at
// audioStreamIndex mixed to stereo first: chromaprint fails outright on 5.1 (D115). A point is 32
// bits, raw, as chromaprint's default algorithm makes them; pointDuration apart.
func (fingerprinter *Fingerprinter) Fingerprint(
	ctx context.Context, film execx.Film, audioStreamIndex int, from, length time.Duration,
) ([]uint32, error) {
	arguments := []string{"-nostats", "-ss", seconds(from), "-t", seconds(length)}
	arguments = append(arguments, film.Input()...)
	arguments = append(arguments, "-map", "0:"+strconv.Itoa(audioStreamIndex), "-vn", "-sn", "-dn", "-ac", "2",
		"-f", "chromaprint", "-fp_format", "raw", "pipe:1")
	ran, err := fingerprinter.runner.RunWatched(ctx, execx.WatchedRun{
		Access: confine.Access{Read: film.Readable()}, Arguments: arguments, StdoutLimitBytes: maxFingerprintBytes,
		Watch: execx.Watch{
			StartGrace: fingerprintStartGrace, StallWindow: fingerprintStallWindow,
			Deadline: execx.DeadlineFor(length),
		},
		Film: film.Stream,
	})
	if err != nil {
		return nil, fmt.Errorf("fingerprint the sound: %w", err)
	}
	return points(ran.Stdout)
}

// points are a raw fingerprint's 32-bit points: the muxer writes them in the machine's order, which is
// little-endian on every machine a node runs on (amd64, arm64).
func points(raw []byte) ([]uint32, error) {
	if len(raw)%4 != 0 {
		return nil, fmt.Errorf("a raw fingerprint of %d bytes is not whole 32-bit points", len(raw))
	}
	found := make([]uint32, len(raw)/4)
	for index := range found {
		found[index] = binary.LittleEndian.Uint32(raw[index*4:])
	}
	return found, nil
}

// seconds is a duration as FFmpeg's time arguments take it, to the millisecond.
func seconds(duration time.Duration) string {
	return strconv.FormatFloat(duration.Seconds(), 'f', 3, 64)
}
