// SPDX-License-Identifier: AGPL-3.0-or-later

package chunks

import (
	"context"
	"slices"
	"time"

	"github.com/kstenerud/orpheum/internal/core/chunks/qualities"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
)

// runsToTakeOver is how many fair runs of a node's kind of film and quality its runs' figure takes
// over from its trials' at: three (§3).
const runsToTakeOver = 3

// runSpeedsKept is how many of the last fair runs of a node's kind of film and quality are kept, and
// their middle one taken: an odd number, so the middle is one of them (§3, the lead's reading).
const runSpeedsKept = 5

// SpeedFrom is where the speed a start's quality was expected to be made at came from, as its log
// line says it: its node's start-up trials, or its fair runs of the film's kind and the quality.
type SpeedFrom string

// The sources of an expected speed (§2, §3).
const (
	SpeedFromTrial SpeedFrom = "trial"
	SpeedFromRuns  SpeedFrom = "runs"
)

// filmKind is a kind of film as its node's runs are kept by: its picture's codec, bit depth and
// height, as a trial's kind is named (nodeproto.SpeedTrial).
type filmKind struct {
	codec            string
	bitDepth, height int
}

// kindOf is the kind of film whose picture is picture.
func kindOf(picture nodeproto.ProbeStream) filmKind {
	return filmKind{codec: picture.Codec, bitDepth: picture.Video.BitDepth(), height: picture.Video.Height}
}

// isFairRun is whether a run's report is a fair measure of its node's speed (§3): a run of a quality
// that finished, made its speed window's film, nodeproto.SpeedWindowFilmMs, and went alone on its node
// over the window (D572).
func isFairRun(command nodeproto.MakeChunks, report nodeproto.MakeChunksReport) bool {
	return report.State == nodeproto.RunDone && command.Stream.Kind == nodeproto.StreamQuality &&
		!report.WasBesideAnotherRun && report.FilmMadeMs >= nodeproto.SpeedWindowFilmMs && report.SpeedPercent > 0 &&
		report.DecodedOn != ""
}

// keepRunSpeed keeps a fair run's speed under its node's speed key, its film's kind and its quality,
// the last runSpeedsKept of them, and forgets the node's runs made under another key, which start
// over (§3). A run not fair, or of a node not heard since the core started, keeps nothing; a failure
// to keep it is logged, and the run's end taken as before.
func (service *Service) keepRunSpeed(
	ctx context.Context, nodeID int64, command nodeproto.MakeChunks, report nodeproto.MakeChunksReport,
) {
	if service.encoders == nil || !isFairRun(command, report) {
		return
	}
	speedKey := service.encoders.SpeedKey(nodeID)
	if speedKey == "" {
		return
	}
	film, _, err := service.cut(ctx, fileOfStore(command.StoreDir), OnTime) // a cut on time reads no timeline
	if err != nil {
		service.logger.WarnContext(ctx, "could not keep a run's speed", "nodeId", nodeID,
			"storeDir", command.StoreDir, "err", err)
		return
	}
	kind := kindOf(film.picture)
	quality := command.Stream
	err = service.cache.Write(ctx, func(queries *cachegen.Queries) error {
		if deleteErr := queries.DeleteNodeRunSpeedsOfOtherKeys(ctx, cachegen.DeleteNodeRunSpeedsOfOtherKeysParams{
			NodeID: nodeID, SpeedKey: speedKey,
		}); deleteErr != nil {
			return deleteErr
		}
		if insertErr := queries.InsertNodeRunSpeed(ctx, cachegen.InsertNodeRunSpeedParams{
			NodeID: nodeID, SpeedKey: speedKey, Codec: kind.codec, BitDepth: int64(kind.bitDepth),
			Height: int64(kind.height), QualityKbps: int64(quality.BitRateKbps), QualityHeight: int64(quality.Height),
			SpeedPercent: int64(report.SpeedPercent), FilmMadeMs: report.FilmMadeMs, DecodedOn: report.DecodedOn,
			MadeAt: service.clock.Now().UnixMilli(),
		}); insertErr != nil {
			return insertErr
		}
		return queries.DeleteNodeRunSpeedsBeyondKept(ctx, cachegen.DeleteNodeRunSpeedsBeyondKeptParams{
			NodeID: nodeID, Codec: kind.codec, BitDepth: int64(kind.bitDepth), Height: int64(kind.height),
			QualityKbps: int64(quality.BitRateKbps), QualityHeight: int64(quality.Height), KeptCount: runSpeedsKept,
		})
	})
	if err != nil {
		service.logger.WarnContext(ctx, "could not keep a run's speed", "nodeId", nodeID,
			"storeDir", command.StoreDir, "err", err)
	}
}

// runSpeedsOf are the speeds, in percent of real time, a node's fair runs of a kind of film give each
// quality it has made at least runsToTakeOver of under its speed key: the middle of the last
// runSpeedsKept (§3). None for a node not heard since the core started; a failure to read is logged,
// and the trials judge alone.
func (service *Service) runSpeedsOf(ctx context.Context, nodeID int64, kind filmKind) map[qualities.Quality]int {
	speedKey := service.encoders.SpeedKey(nodeID)
	if speedKey == "" {
		return nil
	}
	rows, err := service.cache.Read().ListNodeRunSpeedsOfKind(ctx, cachegen.ListNodeRunSpeedsOfKindParams{
		NodeID: nodeID, SpeedKey: speedKey, Codec: kind.codec, BitDepth: int64(kind.bitDepth),
		Height: int64(kind.height),
	})
	if err != nil {
		service.logger.WarnContext(ctx, "could not read a node's runs' speeds", "nodeId", nodeID, "err", err)
		return nil
	}
	byQuality := map[qualities.Quality][]int{}
	for _, row := range rows { // newest first
		quality := qualities.Quality{Kbps: int(row.QualityKbps), Height: int(row.QualityHeight)}
		if len(byQuality[quality]) < runSpeedsKept {
			byQuality[quality] = append(byQuality[quality], int(row.SpeedPercent))
		}
	}
	figures := map[qualities.Quality]int{}
	for quality, speeds := range byQuality {
		if len(speeds) >= runsToTakeOver {
			figures[quality] = middleOf(speeds)
		}
	}
	return figures
}

// middleOf is the middle of speeds, the median; of an even count, the lower of the two middle ones,
// the slower.
func middleOf(speeds []int) int {
	sorted := slices.Sorted(slices.Values(speeds))
	return sorted[(len(sorted)-1)/2]
}

// measuringHoldStartup is what a hold to measure a run allows past twice the window's expected time:
// the run's start, its FFmpeg opening the film and reaching its first chunk, which the node times as
// part of the window, and the stopped work's FFmpeg ending (D572).
const measuringHoldStartup = 5 * time.Second

// measuringHoldMost is the longest a hold to measure a run keeps its node's other work back, however
// slow the node is judged: at chunkRunSlowestSpeedPercent's default, 110 %, the window takes 27 s, and
// twice that with its start is under a minute, so the cap is met only by a node judged slower than a
// film plays, or not judged at all. It bounds what one start costs the background work, a library's
// scrub-bar pictures above all, which a node starting often would otherwise never finish (D572).
const measuringHoldMost = 90 * time.Second

// holdBackToMeasure holds back the other work of film's node while a played run of a quality makes
// its speed window, so the run goes alone on its node and is a fair measure of its speed (D572,
// start-quality-by-node-speed.md §3): the node's background work, its trickplay above all, is
// stopped and given none until twice the window's expected time at the speed the run is expected at,
// with its start (measuringHoldStartup), and never past measuringHoldMost. It holds nothing for a run
// no one waits to play, of sound or a copied picture, shorter than the window, of a node whose runs
// are not kept (not heard since the core started), or of a quality the node's runs have taken over
// for the film's kind: those need no more measuring. A node that encodes on a GPU is held too: its
// runs are measured as one in software's are. Another player's run, played, is never stopped
// (nodes.Exchanges.HoldBackFor), and the run it goes beside stays unfair. It is called before the
// run is given, so the node is told to stop its work before it is told to start the run.
func (service *Service) holdBackToMeasure(ctx context.Context, job jobs.Job, film cutFilm, run storedRun) {
	if service.holdBack == nil || service.encoders == nil || job.Priority < jobs.PriorityPlayNow ||
		run.stream.Kind != nodeproto.StreamQuality ||
		film.grid.endMs(run.LastChunk)-film.grid.StartsMs[run.FirstChunk] < nodeproto.SpeedWindowFilmMs {
		return
	}
	nodeID := film.file.NodeID
	if service.encoders.SpeedKey(nodeID) == "" {
		return
	}
	speeds := service.nodeSpeedsFor(ctx, film)
	if _, isTakenOver := speeds.ran[run.stream.Quality]; isTakenOver {
		return
	}
	wait := measuringHoldMost
	if speed, _, isJudged := speeds.of(run.stream.Quality); isJudged && speed > 0 {
		windowTakesMs := float64(nodeproto.SpeedWindowFilmMs) * 100 / speed
		wait = min(wait, 2*time.Duration(windowTakesMs)*time.Millisecond+measuringHoldStartup)
	}
	until := service.clock.Now().Add(wait)
	stopped := service.holdBack.HoldBackFor(ctx, nodeID, until)
	service.logger.InfoContext(ctx, "held a node's other work back to measure a played run alone", "nodeId", nodeID,
		"jobId", job.ID, "stream", run.stream.Name(), "stoppedCount", stopped, "heldBackUntil", until)
}
