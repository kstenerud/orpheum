// SPDX-License-Identifier: GPL-3.0-or-later

package artwork

import (
	"context"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
)

// The ceiling's workings (D212 §6, §7).
const (
	gib = int64(1) << 30
	// A sweep evicts down to 90% of the ceiling, so it does not run again at the next write.
	lowWater = 0.9
	// The recommended ceiling is what is never evicted, plus 15%.
	headroom = 1.15
	day      = 24 * time.Hour
	noneMark = ".none"
)

// holdings is what the art directory holds, kept in memory from one listing at start and every
// write and removal since, and when each image was last served, to the day.
type holdings struct {
	mutex               sync.Mutex
	files               map[string]map[string]int64 // hash, then file name, to size
	total               int64
	lastServedDayByHash map[string]int64 // hash to day
	unsaved             map[string]int64 // served since the last flush
	pressure            Pressure
	sweep               chan struct{}
}

// Pressure is the artwork cache against its ceiling, as the last sweep found it.
type Pressure struct {
	TotalBytes    int64 // bytes held
	RequiredBytes int64 // bytes never evicted: what live items need to be shown
	CeilingBytes  int64
	// HighWater is what is never evicted exceeding the ceiling: artwork is still served and
	// fetched as needed, and the admin is told the ceiling to set (D212 §7).
	HighWater        bool
	RecommendedBytes int64 // the ceiling to set: what is required plus 15%, in whole GiB
}

func newHoldings() *holdings {
	return &holdings{
		files: map[string]map[string]int64{}, lastServedDayByHash: map[string]int64{}, unsaved: map[string]int64{},
		sweep: make(chan struct{}, 1),
	}
}

// Load reads what the art directory holds and when each image was last served; call it once,
// before serving.
func (service *Service) Load(ctx context.Context) error {
	files, err := service.parts.Store.ListArt()
	if err != nil {
		return err
	}
	rows, err := service.parts.Cache.Read().ListArtServed(ctx)
	if err != nil {
		return fmt.Errorf("read when art was last served: %w", err)
	}
	held := service.holdings
	held.mutex.Lock()
	defer held.mutex.Unlock()
	for _, file := range files {
		held.add(file.Hash, file.Name, file.Size)
	}
	for _, row := range rows {
		held.lastServedDayByHash[row.ContentHashHex] = row.LastServedEpochDay
	}
	return nil
}

func (held *holdings) add(hash, name string, size int64) {
	if held.files[hash] == nil {
		held.files[hash] = map[string]int64{}
	}
	held.total += size - held.files[hash][name]
	held.files[hash][name] = size
}

// writeArt stores one file and counts it; past the ceiling it wakes the sweep.
func (service *Service) writeArt(ctx context.Context, hash, name string, contents []byte) error {
	if err := service.parts.Store.WriteArt(hash, name, contents); err != nil {
		return fmt.Errorf("store art %s %s: %w", hash, name, err)
	}
	held := service.holdings
	held.mutex.Lock()
	held.add(hash, name, int64(len(contents)))
	total := held.total
	held.mutex.Unlock()
	ceiling, err := service.ceiling(ctx)
	if err == nil && total > ceiling {
		select {
		case held.sweep <- struct{}{}:
		default:
		}
	}
	return nil
}

// noteServed records that an image was served today, in memory; Flush writes it.
func (service *Service) noteServed(hash string) {
	today := service.parts.Clock.Now().Unix() / int64(day/time.Second)
	held := service.holdings
	held.mutex.Lock()
	defer held.mutex.Unlock()
	if held.lastServedDayByHash[hash] < today {
		held.lastServedDayByHash[hash], held.unsaved[hash] = today, today
	}
}

// Flush writes when images were served since the last flush, in one batch.
func (service *Service) Flush(ctx context.Context) error {
	held := service.holdings
	held.mutex.Lock()
	unsaved := held.unsaved
	held.unsaved = map[string]int64{}
	held.mutex.Unlock()
	if len(unsaved) == 0 {
		return nil
	}
	err := service.parts.Cache.Write(ctx, func(queries *cachegen.Queries) error {
		for hash, served := range unsaved {
			marked := cachegen.MarkArtServedParams{ContentHashHex: hash, LastServedEpochDay: served}
			if err := queries.MarkArtServed(ctx, marked); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("write when %d images were served: %w", len(unsaved), err)
	}
	return nil
}

// Sweeps is woken when a write takes the cache past its ceiling.
func (service *Service) Sweeps() <-chan struct{} {
	return service.holdings.sweep
}

func (service *Service) ceiling(ctx context.Context) (int64, error) {
	gibs, err := service.parts.Settings.Int(ctx, settings.ArtworkCacheCeilingGiB)
	if err != nil {
		return 0, fmt.Errorf("read artwork_cache_ceiling_gib: %w", err)
	}
	return gibs * gib, nil
}

// class is how soon a file is evicted: the lower, the sooner (D212 §6).
type class int

const (
	classRemoved  class = iota + 1 // a removed item's image
	classOnDemand                  // a size made on request, or JPEG: cut again when asked for
	classOther                     // another size, cut again from the base by the node
	classBase                      // the largest size, fetched again when next needed
	classKept                      // never evicted while its item is live
)

// candidate is one file a sweep may evict.
type candidate struct {
	hash, name string
	size       int64
	class      class
	served     int64
	goneAt     int64
}

// Sweep evicts what the ceiling says to: removed items' images once their grace has passed,
// always; and past the ceiling, in the order of D212 §6, least recently served first within each
// class, down to 90% of it. It never evicts a live item's poster-wall size or the smallest size
// of each shape, so a request always has a size to go to. It returns the cache's pressure.
func (service *Service) Sweep(ctx context.Context) (Pressure, error) {
	ceiling, err := service.ceiling(ctx)
	if err != nil {
		return Pressure{}, err
	}
	graceDays, err := service.parts.Settings.Int(ctx, settings.RemovedItemArtworkKeptD)
	if err != nil {
		return Pressure{}, fmt.Errorf("read removed_item_artwork_kept_d: %w", err)
	}
	rows, err := service.parts.Database.Read().ListArtworkHashes(ctx)
	if err != nil {
		return Pressure{}, fmt.Errorf("list the stored images: %w", err)
	}
	shown := map[string]bool{}
	goneAt := map[string]int64{}
	for _, row := range rows {
		key := hex.EncodeToString(row.ContentHash)
		shown[key], goneAt[key] = row.Live == 1, row.GoneAt
	}
	sorted := service.classify(shown, goneAt)
	candidates, required, total := sorted.candidates, sorted.required, sorted.total
	graceEnds := service.parts.Clock.Now().Add(-time.Duration(graceDays) * day).UnixMilli()
	over, lowMark := total > ceiling, int64(float64(ceiling)*lowWater)
	for _, file := range candidates {
		pastGrace := file.class == classRemoved && file.goneAt < graceEnds
		if !pastGrace && (!over || total <= lowMark) {
			continue
		}
		if err := service.evict(file); err != nil {
			return Pressure{}, err
		}
		total -= file.size
	}
	pressure := Pressure{
		TotalBytes: total, RequiredBytes: required, CeilingBytes: ceiling, HighWater: required > ceiling,
	}
	if pressure.HighWater {
		pressure.RecommendedBytes = (int64(float64(required)*headroom) + gib - 1) / gib * gib
	}
	service.recordPressure(ctx, pressure)
	return pressure, nil
}

// classified is every held file a sweep may evict, soonest first, with the bytes never evicted
// and the bytes held in all.
type classified struct {
	candidates []candidate
	required   int64
	total      int64
}

// classify sorts every held file into its class.
func (service *Service) classify(shown map[string]bool, goneAt map[string]int64) classified {
	held := service.holdings
	held.mutex.Lock()
	defer held.mutex.Unlock()
	var candidates []candidate
	var required int64
	for hash, files := range held.files {
		base := baseOf(files)
		for name, size := range files {
			file := candidate{
				hash: hash, name: name, size: size, served: held.lastServedDayByHash[hash], goneAt: goneAt[hash],
			}
			file.class = classOf(name, base, shown[hash])
			if file.class == classKept {
				required += size
				continue
			}
			candidates = append(candidates, file)
		}
	}
	slices.SortFunc(candidates, func(first, second candidate) int {
		if first.class != second.class {
			return int(first.class - second.class)
		}
		if first.served != second.served {
			return int(first.served - second.served)
		}
		return strings.Compare(first.hash+first.name, second.hash+second.name)
	})
	return classified{candidates: candidates, required: required, total: held.total}
}

// classOf is a file's class.
func classOf(name, base string, live bool) class {
	switch {
	case strings.HasSuffix(name, noneMark):
		return classKept // an empty note that a size cannot be made
	case !live:
		return classRemoved
	case strings.HasSuffix(name, "."+FormatJPEG):
		return classOnDemand
	}
	size, known := nodeproto.ArtSizeName(strings.TrimSuffix(name, webpFormat)).Size()
	switch {
	case !known:
		return classRemoved
	case size.Name == nodeproto.PortraitM || size.Name == size.Shape.Sizes()[0].Name:
		return classKept // the poster wall's size, and the smallest of each shape
	case size.OnRequest:
		return classOnDemand
	case name == base:
		return classBase
	}
	return classOther
}

// baseOf is an image's largest WebP size held: what its other sizes are cut from.
func baseOf(files map[string]int64) string {
	base, width := "", 0
	for name := range files {
		size, known := nodeproto.ArtSizeName(strings.TrimSuffix(name, webpFormat)).Size()
		if known && strings.HasSuffix(name, webpFormat) && size.Width > width {
			base, width = name, size.Width
		}
	}
	return base
}

// evict removes one file and forgets it.
func (service *Service) evict(file candidate) error {
	if err := service.parts.Store.RemoveArt(file.hash, file.name); err != nil {
		return fmt.Errorf("evict art %s %s: %w", file.hash, file.name, err)
	}
	held := service.holdings
	held.mutex.Lock()
	defer held.mutex.Unlock()
	held.total -= held.files[file.hash][file.name]
	delete(held.files[file.hash], file.name)
	if len(held.files[file.hash]) == 0 {
		delete(held.files, file.hash)
	}
	return nil
}

// recordPressure keeps the sweep's finding and says so when high water begins or ends.
func (service *Service) recordPressure(ctx context.Context, pressure Pressure) {
	held := service.holdings
	held.mutex.Lock()
	was := held.pressure.HighWater
	held.pressure = pressure
	held.mutex.Unlock()
	switch {
	case pressure.HighWater && !was:
		service.parts.Logger.WarnContext(ctx, "the artwork cache ceiling is smaller than what the libraries need "+
			"to show; artwork is fetched again as it is needed", "requiredBytes", pressure.RequiredBytes,
			"ceilingBytes", pressure.CeilingBytes, "recommendedGiB", pressure.RecommendedBytes/gib)
	case !pressure.HighWater && was:
		service.parts.Logger.InfoContext(ctx, "the artwork cache fits under its ceiling again")
	}
}

// Pressure is the cache against its ceiling, as the last sweep found it.
func (service *Service) Pressure() Pressure {
	held := service.holdings
	held.mutex.Lock()
	defer held.mutex.Unlock()
	return held.pressure
}
