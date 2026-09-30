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
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/core/datadir"
	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// Waits the service makes: for a node to connect, and for its answer.
const (
	noNodeWait  = time.Minute
	defaultWait = 2 * time.Minute
)

// webpSuffix ends the name of every WebP size the art directory holds: "portrait-m.webp".
const webpSuffix = ".webp"

// Queue is where the make_artwork_sizes jobs go: jobs.Queue.
type Queue interface {
	Enqueue(ctx context.Context, job jobs.Request) (bool, error)
}

// Nodes is the node channel: the node to send work to, and its command queue.
type Nodes interface {
	LastHeardNodeID() (int64, bool)
	IsConnected(nodeID int64) bool
	Enqueue(nodeID int64, command nodeproto.Command)
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
}

// Parts are what the service is built from.
type Parts struct {
	Database *store.Database
	Queue    Queue
	Nodes    Nodes
	ArtFiles ArtFiles
	Cache    *store.Cache // when each image was last served
	Settings Settings
	Fetchers map[string]Fetcher // by source: "tmdb"
	Clock    clock.Clock
	Logger   *slog.Logger
	// AnswerWait is how long a job waits for the node's answer; two minutes when zero.
	AnswerWait time.Duration
}

// Service fetches, re-encodes through the node, and stores artwork.
type Service struct {
	parts Parts

	answers  *nodes.AnswerRouter[nodeproto.ArtworkResult]
	mutex    sync.Mutex
	breakers map[string]*breaker // by source
	holdings *holdings
}

// NewService returns the service.
func NewService(parts Parts) *Service {
	if parts.AnswerWait == 0 {
		parts.AnswerWait = defaultWait
	}
	return &Service{
		parts: parts, answers: nodes.NewAnswerRouter[nodeproto.ArtworkResult](parts.Logger, "an artwork answer"),
		breakers: map[string]*breaker{}, holdings: newHoldings(),
	}
}

// artworkJobArgs is a make_artwork_sizes job's arguments: the images one item's source names that are not stored.
type artworkJobArgs struct {
	ArtworkSource string    `json:"artworkSource"`
	Refs          []owedRef `json:"refs"`
	// SourceSha256 and SizeNames make sizes of a stored image that are made only on request, from its
	// source fetched again (D212 §2, §3).
	SourceSha256 string                  `json:"sourceSha256,omitempty"`
	SizeNames    []nodeproto.ArtSizeName `json:"sizeNames,omitempty"`
}

// owedRef is one image the job fetches: its kind, and the source's reference to it.
type owedRef struct {
	Kind      string `json:"kind"`
	SourceRef string `json:"sourceRef"`
}

// QueueOwed queues a job for each item with images owed. An item with a job already open is left to
// it; what that job does not store is owed again at the next pass.
func (service *Service) QueueOwed(ctx context.Context, items []identify.OwedArtwork) {
	queued := 0
	for _, item := range items {
		refs := make([]owedRef, 0, len(item.Refs))
		for _, ref := range item.Refs {
			refs = append(refs, owedRef{Kind: ref.Kind, SourceRef: ref.Ref})
		}
		args, err := json.Marshal(artworkJobArgs{ArtworkSource: item.Source, Refs: refs})
		if err == nil {
			var added bool
			added, err = service.parts.Queue.Enqueue(ctx, jobs.Request{
				Kind: jobs.KindMakeArtworkSizes, Target: jobs.Target{ItemID: item.ItemID}, ArgsJSON: string(args),
				Reason: "the images " + item.Source + " names for it",
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

// Handle runs one make_artwork_sizes job: each image it names is fetched, re-encoded by the node, and
// stored.
func (service *Service) Handle(ctx context.Context, job jobs.Job) error {
	var args artworkJobArgs
	if err := json.Unmarshal([]byte(job.ArgsJSON), &args); err != nil {
		return fmt.Errorf("make_artwork_sizes job %d: %w", job.ID, jobs.MarkPermanent(err))
	}
	fetcher, known := service.parts.Fetchers[args.ArtworkSource]
	if !known && args.ArtworkSource != identify.SourceLocal {
		return fmt.Errorf("make_artwork_sizes job %d: %w", job.ID, jobs.MarkPermanent(fmt.Errorf("no fetcher for %q",
			args.ArtworkSource)))
	}
	for _, ref := range args.Refs {
		want := wantedImage{jobID: job.ID, itemID: job.ItemID, source: args.ArtworkSource, ref: ref}
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
	jobID  int64
	itemID int64
	source string
	ref    owedRef
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
	command := nodeproto.Artwork{JobID: want.jobID, Shape: shape, SizeNames: fetchedSizes(shape)}
	var nodeID int64
	sourceSha256Hex := ""
	if want.source == identify.SourceLocal {
		nodeID, command.AbsolutePath, err = service.localFile(ctx, want.ref.SourceRef)
		if errors.Is(err, ErrNoImage) {
			return service.recordRefusal(ctx, want, err.Error())
		}
	} else {
		var download Unsanitised
		download, err = service.fetch(ctx, fetcher, want.source, want.ref.SourceRef)
		if errors.Is(err, ErrNoImage) {
			return service.recordMissing(ctx, fetcher, want, err)
		}
		if err == nil {
			command.SourceImage, sourceSha256Hex = download.bytes, download.sourceSha256Hex()
			nodeID, err = service.anyNode()
		}
	}
	if err != nil {
		return err
	}
	result, err := service.reEncode(ctx, nodeID, command)
	if err != nil {
		return err
	}
	return service.keep(ctx, want, cmp.Or(sourceSha256Hex, result.SourceSha256), result)
}

// localFile is the node that holds an image beside the media and its path there. A file gone
// since it was owed is ErrNoImage; its node not connected, a wait that costs no attempt.
func (service *Service) localFile(ctx context.Context, ref string) (int64, *nodeproto.Path, error) {
	fileID, parsed := identify.LocalRefFileID(ref)
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
	if !service.parts.Nodes.IsConnected(file.NodeID) {
		return 0, nil, &jobs.RetryAtError{
			At:  service.parts.Clock.Now().Add(noNodeWait),
			Err: fmt.Errorf("node %d, which holds it, is not connected", file.NodeID),
		}
	}
	return file.NodeID, &nodeproto.Path{Text: file.Path, Raw: file.ReportedPathRaw}, nil
}

// anyNode is a connected node to re-encode a download; none is a wait that costs no attempt.
func (service *Service) anyNode() (int64, error) {
	nodeID, connected := service.parts.Nodes.LastHeardNodeID()
	if !connected {
		return 0, &jobs.RetryAtError{
			At: service.parts.Clock.Now().Add(noNodeWait), Err: errors.New("no node is connected to re-encode it"),
		}
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
	return slices.ContainsFunc(rows, func(row sqlcgen.ListLiveArtworkRow) bool {
		return row.Kind == want.ref.Kind && row.Source == want.source && row.SourceRef != nil &&
			*row.SourceRef == want.ref.SourceRef
	}), nil
}

// reEncode sends a source to the node and waits for its answer, by the job id the command
// carries. A job cancelled while it waits tells the node to stop.
func (service *Service) reEncode(
	ctx context.Context, nodeID int64, command nodeproto.Artwork,
) (nodeproto.ArtworkResult, error) {
	maxSourcePixels, err := service.parts.Settings.Int(ctx, settings.ArtworkMaxSourcePixels)
	if err != nil {
		return nodeproto.ArtworkResult{}, fmt.Errorf("read artwork_max_source_pixels: %w", err)
	}
	quality, err := service.parts.Settings.Int(ctx, settings.ArtworkWebPQuality)
	if err != nil {
		return nodeproto.ArtworkResult{}, fmt.Errorf("read artwork_webp_quality: %w", err)
	}
	answers := service.answers.Await(command.JobID)
	defer service.answers.Forget(command.JobID)
	command.MaxSourcePixels, command.WebPQuality = maxSourcePixels, int(quality)
	service.parts.Nodes.Enqueue(nodeID,
		nodeproto.Command{Type: nodeproto.CommandEncodeArtwork, EncodeArtwork: &command})
	timer := time.NewTimer(service.parts.AnswerWait)
	defer timer.Stop()
	select {
	case result := <-answers:
		return result, nil
	case <-timer.C:
		return nodeproto.ArtworkResult{}, fmt.Errorf("node %d did not answer within %s", nodeID,
			service.parts.AnswerWait)
	case <-ctx.Done():
		if errors.Is(context.Cause(ctx), jobs.ErrCancelled) {
			service.parts.Nodes.Enqueue(nodeID, nodes.CancelCommand(command.JobID))
			return nodeproto.ArtworkResult{}, jobs.MarkPermanent(jobs.ErrCancelled)
		}
		return nodeproto.ArtworkResult{}, fmt.Errorf("wait for node %d: %w", nodeID, ctx.Err())
	}
}

// TakeArtworkAnswer takes a node's answer to the job waiting for it. One nothing waits for — it came after
// the wait ended — is dropped; its job was retried.
func (service *Service) TakeArtworkAnswer(ctx context.Context, nodeID int64, result nodeproto.ArtworkResult) {
	service.answers.Deliver(ctx, nodeID, result.JobID, result)
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
	return service.record(ctx, want, sourceSha256)
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

// recordRefusal records an image that could not be stored, so it is not fetched again; an older image
// of the kind stays shown.
func (service *Service) recordRefusal(ctx context.Context, want wantedImage, why string) error {
	service.parts.Logger.WarnContext(ctx, "an image was not stored", "itemId", want.itemID,
		"source", want.source, "sourceRef", want.ref.SourceRef, "reason", why)
	return service.record(ctx, want, nil)
}

// record writes the item's artwork row; a stored image replaces the source's earlier one of its
// kind, never a person's choice.
func (service *Service) record(ctx context.Context, want wantedImage, sourceSha256 []byte) error {
	err := service.parts.Database.Write(ctx, func(queries *sqlcgen.Queries) error {
		_, err := store.OrNotFound(queries.GetItemKind(ctx, want.itemID))
		if errors.Is(err, store.ErrNotFound) {
			return nil // removed since it was queued
		}
		if err != nil {
			return fmt.Errorf("read item %d: %w", want.itemID, err)
		}
		id, err := queries.InsertArtwork(ctx, sqlcgen.InsertArtworkParams{
			ItemID: want.itemID, Kind: want.ref.Kind, Source: want.source, SourceRef: &want.ref.SourceRef,
			SourceSha256: sourceSha256,
		})
		if err != nil || sourceSha256 == nil {
			return err
		}
		return queries.RetireReplacedArtwork(ctx, sqlcgen.RetireReplacedArtworkParams{
			RemovedAt: new(service.parts.Clock.Now().UnixMilli()), ItemID: want.itemID, Kind: want.ref.Kind,
			Source: want.source, KeepID: id,
		})
	})
	if err != nil {
		return fmt.Errorf("record item %d's %s: %w", want.itemID, want.ref.Kind, err)
	}
	return nil
}

// shapeOf is the shape an artwork kind is cut to.
func shapeOf(kind string) (nodeproto.Shape, bool) {
	switch kind {
	case identify.ArtPoster:
		return nodeproto.ShapePortrait, true
	case identify.ArtBackdrop, identify.ArtStill:
		return nodeproto.ShapeLandscape, true
	case identify.ArtLogo:
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
