// SPDX-License-Identifier: GPL-3.0-or-later

package artwork

import (
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/kstenerud/orpheum/internal/core/datadir"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/core/providers"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/store"
)

// webpSuffix ends the name of every WebP size the art directory holds: "portrait-m.webp".
const webpSuffix = ".webp"

// Queue is where the make_artwork_sizes jobs go: jobs.Queue.
type Queue interface {
	Enqueue(ctx context.Context, job jobs.Request) (bool, error)
}

// Nodes is the node channel (nodes.Service): the node to send work to, and its command queue,
// which parks a job on a node that is not connected (D231 §11).
type Nodes interface {
	ConnectedNodeID(ctx context.Context) (int64, bool)
	Send(ctx context.Context, nodeID int64, command nodeproto.Command) error
	Enqueue(nodeID int64, command nodeproto.Command)
	Unanswered(ctx context.Context, nodeID int64, err error) error
}

// ArtFiles keeps the images' sizes: datadir.Directories.
type ArtFiles interface {
	WriteArt(sourceSha256Hex, name string, contents []byte) error
	ReadArt(sourceSha256Hex, name string) ([]byte, error)
	HasArt(sourceSha256Hex, name string) (bool, error)
	RemoveArt(sourceSha256Hex, name string) error
	ListArt() ([]datadir.ArtFile, error)
}

// Settings are what the service reads from the settings.
type Settings interface {
	Int(ctx context.Context, key settings.Key) (int64, error)
	Duration(ctx context.Context, key settings.Key) (time.Duration, error)
	String(ctx context.Context, key settings.Key) (string, error)
}

// Parts are what the service is built from.
type Parts struct {
	Database *store.Database
	Queue    Queue
	Nodes    Nodes
	// Exchanges are how a job asks a node for work and waits for its results; the node channel
	// hands them the results (nodes.Service.SetResultSink), which are the store's one way in (D212 §8).
	Exchanges *nodes.Exchanges
	ArtFiles  ArtFiles
	Cache     *store.Cache // when each image was last served
	Settings  Settings
	Fetchers  map[string]Fetcher // by source: "tmdb"
	Clock     clock.Clock
	Logger    *slog.Logger
	// AnswerWait is how long a job waits for the node's word on it; defaultAnswerWait when zero.
	AnswerWait time.Duration
}

// defaultAnswerWait is how long a job waits for the node's word on a re-encode, whose progress it
// hears at each status tick while it waits or runs: two minutes is far past an image's encode,
// which takes seconds, and many ticks.
const defaultAnswerWait = 2 * time.Minute

// Service fetches, re-encodes through the node, and stores artwork.
type Service struct {
	parts Parts

	exchanges *nodes.Exchanges
	holdings  *holdings
	beside    *besideTheMedia
}

// NewService returns the service.
func NewService(parts Parts) *Service {
	if parts.AnswerWait == 0 {
		parts.AnswerWait = defaultAnswerWait
	}
	return &Service{
		parts: parts, exchanges: parts.Exchanges,
		holdings: newHoldings(), beside: &besideTheMedia{kindsByItemByLibrary: map[int64]map[int64][]string{}},
	}
}

// artworkJobArgs is a make_artwork_sizes job's arguments: the images one item's source names that are not stored.
type artworkJobArgs struct {
	ArtworkSource string `json:"artworkSource"`
	Refs          []Ref  `json:"refs"`
	// SourceSha256 and SizeNames make sizes of a stored image that are made only on request, from its
	// source fetched again (D212 §2, §3).
	SourceSha256 string                  `json:"sourceSha256,omitempty"`
	SizeNames    []nodeproto.ArtSizeName `json:"sizeNames,omitempty"`
}

// queueOwed queues a job for each item with images owed. An item with a job already open is left to
// it; what that job does not store is owed again at the next pass.
func (service *Service) queueOwed(ctx context.Context, items []ItemImages) {
	queued := 0
	for _, item := range items {
		args, err := json.Marshal(artworkJobArgs{ArtworkSource: item.Source, Refs: item.Refs})
		if err == nil {
			var added bool
			added, err = service.parts.Queue.Enqueue(ctx, jobs.Request{
				Kind: jobs.KindMakeArtworkSizes, Target: jobs.Target{ItemID: item.ItemID}, ArgsJSON: string(args),
				Reason: "the images " + item.Source + " names for it", Destination: service.destinationOf(item.Source),
			})
			if added {
				queued++
			}
		}
		if err != nil {
			service.parts.Logger.ErrorContext(ctx, "could not queue artwork", "itemId", item.ItemID, "err", err)
			return
		}
	}
	if queued > 0 {
		service.parts.Logger.InfoContext(ctx, "queued artwork", "queuedJobs", queued)
	}
}

// destinationOf is what a job for a source's images needs that can be down: its image server; ""
// for images beside the media, whose node is found when the job runs.
func (service *Service) destinationOf(source string) jobs.Destination {
	if fetcher, isKnown := service.parts.Fetchers[source]; isKnown {
		return fetcher.Destination()
	}
	return ""
}

// Handle runs one make_artwork_sizes job: each image it names is fetched, re-encoded by the node, and
// stored.
func (service *Service) Handle(ctx context.Context, job jobs.Job) error {
	ctx = providers.WithImportance(ctx, providers.JobImportance(job.Priority))
	var args artworkJobArgs
	if err := json.Unmarshal([]byte(job.ArgsJSON), &args); err != nil {
		return fmt.Errorf("make_artwork_sizes job %d: %w", job.ID, jobs.MarkPermanent(err))
	}
	fetcher, known := service.parts.Fetchers[args.ArtworkSource]
	if !known && args.ArtworkSource != SourceLocal {
		return fmt.Errorf("make_artwork_sizes job %d: %w", job.ID, jobs.MarkPermanent(fmt.Errorf("no fetcher for %q",
			args.ArtworkSource)))
	}
	for _, ref := range args.Refs {
		want := wantedImage{job: job, itemID: job.ItemID, source: args.ArtworkSource, ref: ref}
		var err error
		if len(args.SizeNames) > 0 {
			err = service.makeSizes(ctx, fetcher, want, args)
		} else {
			err = service.fetchImage(ctx, fetcher, want)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// wantedImage is one image an item's source names, and the job that fetches it.
type wantedImage struct {
	job    jobs.Job
	itemID int64
	source string
	ref    Ref
}

func (service *Service) fetchImage(ctx context.Context, fetcher Fetcher, want wantedImage) error {
	shape, known := shapeOf(want.ref.Kind)
	if !known {
		return nil // a kind this release does not show
	}
	held, err := service.isHeld(ctx, want)
	if err != nil || held {
		return err
	}
	command := nodeproto.Artwork{Shape: shape, SizeNames: fetchedSizes(shape)}
	var nodeID int64
	sourceSha256Hex := ""
	if want.source == SourceLocal {
		nodeID, command.AbsolutePath, err = service.localFile(ctx, want.ref.SourceRef)
		if errors.Is(err, ErrNoImage) {
			return service.recordRefusal(ctx, want, err.Error())
		}
	} else {
		var download Unsanitised
		download, err = service.fetch(ctx, fetcher, want.ref.SourceRef)
		if errors.Is(err, ErrNoImage) {
			return service.recordMissing(ctx, want, err)
		}
		if err == nil {
			command.SourceImage, sourceSha256Hex = download.bytes, download.sourceSha256Hex()
			nodeID, err = service.anyNode(ctx)
		}
	}
	if err != nil {
		return err
	}
	result, err := service.reEncode(ctx, nodeID, want.job, command)
	if err != nil {
		return err
	}
	return service.keep(ctx, want, cmp.Or(sourceSha256Hex, result.SourceSha256), result)
}

// localFile is the node that holds an image beside the media and its path there. A file gone
// since it was owed is ErrNoImage; one under a root or mount the last scan found missing parks.
func (service *Service) localFile(ctx context.Context, ref string) (int64, *nodeproto.Path, error) {
	fileID, parsed := localRefFileID(ref)
	if !parsed {
		return 0, nil, fmt.Errorf("%q: %w", ref, ErrNoImage)
	}
	file, err := store.OrNotFound(service.parts.Database.Read().GetFileForArtwork(ctx, fileID))
	if errors.Is(err, store.ErrNotFound) {
		return 0, nil, fmt.Errorf("file %d is gone: %w", fileID, ErrNoImage)
	}
	if err != nil {
		return 0, nil, fmt.Errorf("read file %d: %w", fileID, err)
	}
	missing, isMissing, err := library.UnavailableUnder(ctx, service.parts.Database.Read(), file.NodeID,
		file.ReportedPathRaw)
	if err != nil {
		return 0, nil, err
	}
	if isMissing {
		// Its disk is away: not the image's refusal, which would be kept for good (G9), but a wait
		// for a scan that finds it (D231 §5).
		return 0, nil, jobs.ParkOnMissingRoot(file.NodeID, missing.Path, missing.Since,
			fmt.Errorf("file %d is under it", fileID))
	}
	return file.NodeID, &nodeproto.Path{Text: file.Path, Raw: file.ReportedPathRaw}, nil
}

// anyNode is a connected node to re-encode a download; with none, the job parks until any node
// says hello.
func (service *Service) anyNode(ctx context.Context) (int64, error) {
	nodeID, connected := service.parts.Nodes.ConnectedNodeID(ctx)
	if !connected {
		return 0, jobs.Park(jobs.Parking{
			Destination: jobs.DestinationAnyNode, Reason: "no node is connected", AwaitsSignal: true,
		}, errors.New("no node is connected to re-encode it"))
	}
	return nodeID, nil
}

// isHeld reports whether the item's artwork already has this image, stored or refused: a job
// retried after storing some of its images.
func (service *Service) isHeld(ctx context.Context, want wantedImage) (bool, error) {
	rows, err := service.parts.Database.Read().ListLiveArtwork(ctx, want.itemID)
	if err != nil {
		return false, fmt.Errorf("list item %d's artwork: %w", want.itemID, err)
	}
	return holds(rows, want.source, want.ref), nil
}

// reEncode sends a source to the node for job and waits for its answer. A job cancelled while
// it waits tells the node to stop.
func (service *Service) reEncode(
	ctx context.Context, nodeID int64, job jobs.Job, command nodeproto.Artwork,
) (nodeproto.ArtworkResult, error) {
	maxSourcePixels, err := service.parts.Settings.Int(ctx, settings.ArtworkMaxSourcePixels)
	if err != nil {
		return nodeproto.ArtworkResult{}, fmt.Errorf("read artwork_max_source_pixels: %w", err)
	}
	quality, err := service.parts.Settings.Int(ctx, settings.ArtworkWebPQuality)
	if err != nil {
		return nodeproto.ArtworkResult{}, fmt.Errorf("read artwork_webp_quality: %w", err)
	}
	command.MaxSourcePixels, command.WebPQuality = maxSourcePixels, int(quality)
	exchange, err := service.exchanges.Ask(ctx, nodeID, job,
		nodeproto.Command{Type: nodeproto.CommandEncodeArtwork, EncodeArtwork: &command}, "re-encoding an image")
	if err != nil {
		return nodeproto.ArtworkResult{}, fmt.Errorf("re-encode an image: %w", err)
	}
	defer exchange.Close()
	result, err := exchange.Next(ctx, service.parts.AnswerWait)
	switch {
	case errors.Is(context.Cause(ctx), jobs.ErrCancelled):
		return nodeproto.ArtworkResult{}, jobs.MarkPermanent(jobs.ErrCancelled)
	case errors.Is(err, nodes.ErrNodeRestarted) && !job.PreviousAttemptToolFailed:
		// The node lost the work with its restart: the tool's failure, tried again at no cost
		// (D238 §3); a second tool failure in a row fails the job, which the queue records.
		return nodeproto.ArtworkResult{}, jobs.MarkInterrupted(err)
	case errors.Is(err, nodes.ErrNodeRestarted):
		return nodeproto.ArtworkResult{}, jobs.MarkPermanent(fmt.Errorf("%s%w", jobs.ToolFailedTwice, err))
	case err != nil:
		return nodeproto.ArtworkResult{}, err
	case result.Artwork == nil:
		return nodeproto.ArtworkResult{}, fmt.Errorf("node %d answered re-encoding an image with something else",
			nodeID)
	}
	return *result.Artwork, nil
}

// keep stores what the node made, then records it as the item's image of its kind; a refusal is
// recorded with no hash.
func (service *Service) keep(
	ctx context.Context, want wantedImage, sourceSha256Hex string, result nodeproto.ArtworkResult,
) error {
	shape, _ := shapeOf(want.ref.Kind) // checked before fetching
	images, why := storableImages(result, shape)
	sourceSha256, err := hex.DecodeString(sourceSha256Hex)
	if why == "" && (err != nil || len(sourceSha256) != sha256.Size) {
		why = "the node named no source hash"
	}
	if why != "" {
		return service.recordRefusal(ctx, want, why)
	}
	for _, made := range images {
		if err := service.writeArt(ctx, sourceSha256Hex, string(made.SizeName)+webpSuffix, made.WebP); err != nil {
			return err
		}
	}
	if err := service.noteCannotMake(ctx, sourceSha256Hex, sizesTooLarge(shape, result.Images)); err != nil {
		return err
	}
	return service.record(ctx, want, sourceSha256, nil)
}

// storableImages is what of the node's answer may be stored: sizes asked for, in WebP. It says why
// nothing may be.
func storableImages(result nodeproto.ArtworkResult, shape nodeproto.Shape) ([]nodeproto.ArtImage, string) {
	if result.Refusal != "" {
		return nil, "the node refused it: " + result.Refusal
	}
	asked := fetchedSizes(shape)
	var kept []nodeproto.ArtImage
	for _, image := range result.Images {
		if slices.Contains(asked, image.SizeName) && isWebP(image.WebP) {
			kept = append(kept, image)
		}
	}
	if len(kept) == 0 {
		return nil, "the node's answer had no size that was asked for, in WebP"
	}
	return kept, ""
}

// isWebP reports whether bytes are a RIFF WebP container, as our encoder writes.
func isWebP(image []byte) bool {
	return len(image) > 12 && bytes.Equal(image[:4], []byte("RIFF")) && bytes.Equal(image[8:12], []byte("WEBP"))
}

// recordRefusal records an image that could not be stored, with why and when, so it is not
// fetched again until a person's refresh asks (D231 §4); an older image of the kind stays shown.
func (service *Service) recordRefusal(ctx context.Context, want wantedImage, why string) error {
	service.parts.Logger.WarnContext(ctx, "an image was not stored", "itemId", want.itemID,
		"source", want.source, "sourceRef", want.ref.SourceRef, "reason", why)
	return service.record(ctx, want, nil, &refusal{why: why})
}

// RecordFailure records a make_artwork_sizes job that failed as its images' own failure (D231 §4,
// §10): each image it named that its item does not hold is recorded as refused, with why, so the
// sweep does not ask for it again until a person's refresh does. A job making sizes on request
// records nothing: the next request asks again.
func (service *Service) RecordFailure(ctx context.Context, job jobs.Job, failure string) error {
	var args artworkJobArgs
	if err := json.Unmarshal([]byte(job.ArgsJSON), &args); err != nil || len(args.SizeNames) > 0 {
		return nil //nolint:nilerr // arguments that cannot be read name no image to record
	}
	for _, ref := range args.Refs {
		want := wantedImage{job: job, itemID: job.ItemID, source: args.ArtworkSource, ref: ref}
		if _, known := shapeOf(ref.Kind); !known {
			continue
		}
		held, err := service.isHeld(ctx, want)
		if err != nil {
			return err
		}
		if !held {
			if err := service.recordRefusal(ctx, want, "given up: "+failure); err != nil {
				return err
			}
		}
	}
	return nil
}

// shapeOf is the shape an artwork kind is cut to.
func shapeOf(kind string) (nodeproto.Shape, bool) {
	switch kind {
	case library.ArtPoster:
		return nodeproto.ShapePortrait, true
	case library.ArtBackdrop, library.ArtStill:
		return nodeproto.ShapeLandscape, true
	case library.ArtLogo:
		return nodeproto.ShapeLogo, true
	}
	return "", false
}

// fetchedSizes are the shape's sizes made when an image is fetched; the rest are made when asked
// for (D212 §3).
func fetchedSizes(shape nodeproto.Shape) []nodeproto.ArtSizeName {
	var names []nodeproto.ArtSizeName
	for _, size := range shape.Sizes() {
		if !size.IsMadeOnRequest {
			names = append(names, size.Name)
		}
	}
	return names
}
