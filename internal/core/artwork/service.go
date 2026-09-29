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
	webpFormat  = ".webp"
)

// Queue is where the artwork jobs go: jobs.Queue.
type Queue interface {
	Enqueue(ctx context.Context, job jobs.New) (bool, error)
}

// Nodes is the node channel: the node to send work to, and its command queue.
type Nodes interface {
	Connected() (int64, bool)
	IsConnected(nodeID int64) bool
	Enqueue(nodeID int64, command nodeproto.Command)
}

// Store keeps the images' sizes: datadir.Directories.
type Store interface {
	WriteArt(hash, name string, contents []byte) error
	ReadArt(hash, name string) ([]byte, error)
	HasArt(hash, name string) (bool, error)
	RemoveArt(hash, name string) error
	ListArt() ([]datadir.ArtFile, error)
}

// Settings are what the service reads from the settings.
type Settings interface {
	Int(ctx context.Context, key settings.Key) (int64, error)
}

// Parts are what the service is built from.
type Parts struct {
	Database *store.Database
	Queue    Queue
	Nodes    Nodes
	Store    Store
	Cache    *store.Cache // when each image was last served
	Settings Settings
	Fetchers map[string]Fetcher // by source: "tmdb"
	Clock    clock.Clock
	Logger   *slog.Logger
	// Wait is how long a job waits for the node's answer; two minutes when zero.
	Wait time.Duration
}

// Service fetches, re-encodes through the node, and stores artwork.
type Service struct {
	parts Parts

	mutex    sync.Mutex
	next     int64
	waiting  map[int64]chan nodeproto.ArtworkResult
	breakers map[string]*breaker // by source
	holdings *holdings
}

// NewService returns the service.
func NewService(parts Parts) *Service {
	if parts.Wait == 0 {
		parts.Wait = defaultWait
	}
	return &Service{
		parts: parts, waiting: map[int64]chan nodeproto.ArtworkResult{}, breakers: map[string]*breaker{},
		holdings: newHoldings(),
	}
}

// owed is an artwork job's arguments: the images one item's source names that are not stored.
type owed struct {
	Source string    `json:"source"`
	Refs   []owedRef `json:"refs"`
	// Hash and Sizes make sizes of a stored image that are made only on request, from its
	// source fetched again (D212 §2, §3).
	Hash  string                  `json:"hash,omitempty"`
	Sizes []nodeproto.ArtSizeName `json:"sizes,omitempty"`
}

// owedRef is one image the job fetches: its kind, and the source's reference to it.
type owedRef struct {
	Kind string `json:"kind"`
	Ref  string `json:"ref"`
}

// Owe queues a job for each item with images owed. An item with a job already open is left to
// it; what that job does not store is owed again at the next pass.
func (service *Service) Owe(ctx context.Context, items []identify.OwedArtwork) {
	queued := 0
	for _, item := range items {
		refs := make([]owedRef, 0, len(item.Refs))
		for _, ref := range item.Refs {
			refs = append(refs, owedRef{Kind: ref.Kind, Ref: ref.Ref})
		}
		args, err := json.Marshal(owed{Source: item.Source, Refs: refs})
		if err == nil {
			var added bool
			added, err = service.parts.Queue.Enqueue(ctx, jobs.New{
				Kind: jobs.KindArtwork, Subject: item.ItemID, Args: string(args),
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
		service.parts.Logger.InfoContext(ctx, "queued artwork", "jobs", queued)
	}
}

// Handle runs one artwork job: each image it names is fetched, re-encoded by the node, and
// stored.
func (service *Service) Handle(ctx context.Context, job jobs.Job) error {
	var args owed
	if err := json.Unmarshal([]byte(job.Args), &args); err != nil {
		return fmt.Errorf("artwork job %d: %w", job.ID, jobs.Permanent(err))
	}
	fetcher, known := service.parts.Fetchers[args.Source]
	if !known && args.Source != identify.SourceLocal {
		return fmt.Errorf("artwork job %d: %w", job.ID, jobs.Permanent(fmt.Errorf("no fetcher for %q", args.Source)))
	}
	for _, ref := range args.Refs {
		image := wanted{itemID: job.Subject, source: args.Source, ref: ref}
		var err error
		if len(args.Sizes) > 0 {
			err = service.makeSizes(ctx, fetcher, image, args)
		} else {
			err = service.one(ctx, fetcher, image)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// wanted is one image an item's source names.
type wanted struct {
	itemID int64
	source string
	ref    owedRef
}

func (service *Service) one(ctx context.Context, fetcher Fetcher, image wanted) error {
	shape, known := shapeOf(image.ref.Kind)
	if !known {
		return nil // a kind this release does not show
	}
	held, err := service.held(ctx, image)
	if err != nil || held {
		return err
	}
	command := nodeproto.Artwork{Shape: shape, SizeNames: fetchedSizes(shape)}
	var nodeID int64
	hash := ""
	if image.source == identify.SourceLocal {
		nodeID, command.AbsolutePath, err = service.localFile(ctx, image.ref.Ref)
		if errors.Is(err, ErrNoImage) {
			return service.refused(ctx, image, err.Error())
		}
	} else {
		var source Unsanitised
		source, err = service.fetch(ctx, fetcher, image.source, image.ref.Ref)
		if errors.Is(err, ErrNoImage) {
			return service.missing(ctx, fetcher, image, err)
		}
		if err == nil {
			command.SourceImage, hash = source.bytes, source.hash()
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
	return service.keep(ctx, image, cmp.Or(hash, result.SourceHash), result)
}

// localFile is the node that holds an image beside the media and its path there. A file gone
// since it was owed is ErrNoImage; its node not connected, a wait that costs no attempt.
func (service *Service) localFile(ctx context.Context, ref string) (int64, *nodeproto.Path, error) {
	fileID, parsed := identify.LocalFile(ref)
	if !parsed {
		return 0, nil, fmt.Errorf("%q: %w", ref, ErrNoImage)
	}
	file, err := store.One(service.parts.Database.Read().GetFileForArtwork(ctx, fileID))
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
	return file.NodeID, &nodeproto.Path{Text: file.Path, Raw: file.PathKey}, nil
}

// anyNode is a connected node to re-encode a download; none is a wait that costs no attempt.
func (service *Service) anyNode() (int64, error) {
	nodeID, connected := service.parts.Nodes.Connected()
	if !connected {
		return 0, &jobs.RetryAtError{
			At: service.parts.Clock.Now().Add(noNodeWait), Err: errors.New("no node is connected to re-encode it"),
		}
	}
	return nodeID, nil
}

// held reports whether the item's artwork already has this image, stored or refused: a job
// retried after storing some of its images.
func (service *Service) held(ctx context.Context, image wanted) (bool, error) {
	rows, err := service.parts.Database.Read().ListLiveArtwork(ctx, image.itemID)
	if err != nil {
		return false, fmt.Errorf("list item %d's artwork: %w", image.itemID, err)
	}
	return slices.ContainsFunc(rows, func(row sqlcgen.ListLiveArtworkRow) bool {
		return row.Kind == image.ref.Kind && row.Source == image.source && row.SourceRef != nil &&
			*row.SourceRef == image.ref.Ref
	}), nil
}

// reEncode sends a source to the node and waits for its answer.
func (service *Service) reEncode(
	ctx context.Context, nodeID int64, command nodeproto.Artwork,
) (nodeproto.ArtworkResult, error) {
	pixels, err := service.parts.Settings.Int(ctx, settings.ArtworkMaxSourcePixels)
	if err != nil {
		return nodeproto.ArtworkResult{}, fmt.Errorf("read artwork_max_source_pixels: %w", err)
	}
	quality, err := service.parts.Settings.Int(ctx, settings.ArtworkWebPQuality)
	if err != nil {
		return nodeproto.ArtworkResult{}, fmt.Errorf("read artwork_webp_quality: %w", err)
	}
	request, answer := service.await()
	defer service.forget(request)
	command.JobID, command.MaxSourcePixels, command.WebPQuality = request, pixels, int(quality)
	service.parts.Nodes.Enqueue(nodeID, nodeproto.Command{Type: nodeproto.CommandArtwork, Artwork: &command})
	timer := time.NewTimer(service.parts.Wait)
	defer timer.Stop()
	select {
	case result := <-answer:
		return result, nil
	case <-timer.C:
		return nodeproto.ArtworkResult{}, fmt.Errorf("node %d did not answer within %s", nodeID, service.parts.Wait)
	case <-ctx.Done():
		return nodeproto.ArtworkResult{}, fmt.Errorf("wait for node %d: %w", nodeID, ctx.Err())
	}
}

func (service *Service) await() (int64, <-chan nodeproto.ArtworkResult) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	service.next++
	answer := make(chan nodeproto.ArtworkResult, 1)
	service.waiting[service.next] = answer
	return service.next, answer
}

func (service *Service) forget(request int64) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	delete(service.waiting, request)
}

// Answered takes a node's answer to the job waiting for it. One nothing waits for — it came after
// the wait ended — is dropped; its job was retried.
func (service *Service) Answered(ctx context.Context, nodeID int64, result nodeproto.ArtworkResult) {
	service.mutex.Lock()
	answer, found := service.waiting[result.JobID]
	delete(service.waiting, result.JobID)
	service.mutex.Unlock()
	if !found {
		service.parts.Logger.InfoContext(ctx, "an artwork answer came after its wait ended", "nodeId", nodeID,
			"request", result.JobID)
		return
	}
	answer <- result
}

// keep stores what the node made, then records it as the item's image of its kind; a refusal is
// recorded with no hash.
func (service *Service) keep(ctx context.Context, image wanted, hash string, result nodeproto.ArtworkResult) error {
	shape, _ := shapeOf(image.ref.Kind) // checked before fetching
	images, why := checked(result, shape)
	raw, err := hex.DecodeString(hash)
	if why == "" && (err != nil || len(raw) != sha256.Size) {
		why = "the node named no source hash"
	}
	if why != "" {
		return service.refused(ctx, image, why)
	}
	for _, made := range images {
		if err := service.writeArt(ctx, hash, string(made.SizeName)+webpFormat, made.WebP); err != nil {
			return err
		}
	}
	if err := service.cannotMake(ctx, hash, tooLarge(shape, result.Images)); err != nil {
		return err
	}
	return service.record(ctx, image, raw)
}

// checked is what of the node's answer may be stored: sizes asked for, in WebP. It says why
// nothing may be.
func checked(result nodeproto.ArtworkResult, shape nodeproto.Shape) ([]nodeproto.ArtImage, string) {
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

// refused records an image that could not be stored, so it is not fetched again; an older image
// of the kind stays shown.
func (service *Service) refused(ctx context.Context, image wanted, why string) error {
	service.parts.Logger.WarnContext(ctx, "an image was not stored", "itemId", image.itemID,
		"source", image.source, "ref", image.ref.Ref, "why", why)
	return service.record(ctx, image, nil)
}

// record writes the item's artwork row; a stored image replaces the source's earlier one of its
// kind, never a person's choice.
func (service *Service) record(ctx context.Context, image wanted, hash []byte) error {
	err := service.parts.Database.Write(ctx, func(queries *sqlcgen.Queries) error {
		_, err := store.One(queries.GetItemKind(ctx, image.itemID))
		if errors.Is(err, store.ErrNotFound) {
			return nil // removed since it was queued
		}
		if err != nil {
			return fmt.Errorf("read item %d: %w", image.itemID, err)
		}
		id, err := queries.InsertArtwork(ctx, sqlcgen.InsertArtworkParams{
			ItemID: image.itemID, Kind: image.ref.Kind, Source: image.source, SourceRef: &image.ref.Ref,
			ContentHash: hash,
		})
		if err != nil || hash == nil {
			return err
		}
		return queries.MarkArtworkRemoved(ctx, sqlcgen.MarkArtworkRemovedParams{
			RemovedAt: new(service.parts.Clock.Now().UnixMilli()), ItemID: image.itemID, Kind: image.ref.Kind,
			Source: image.source, KeepID: id,
		})
	})
	if err != nil {
		return fmt.Errorf("record item %d's %s: %w", image.itemID, image.ref.Kind, err)
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
		if !size.OnRequest {
			names = append(names, size.Name)
		}
	}
	return names
}
