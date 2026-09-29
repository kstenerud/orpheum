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
	lowWaterFraction = 0.9
	// The recommended ceiling is what is never evicted, plus 15%.
	headroomFactor   = 1.15
	day              = 24 * time.Hour
	cannotMakeSuffix = ".none"
)

// holdings is what the art directory holds, kept in memory from one listing at start and every
// write and removal since, and when each image was last served, to the day.
type holdings struct {
	mutex                  sync.Mutex
	sizeBytesByNameByHash  map[string]map[string]int64 // hash, then file name, to size
	totalBytes             int64
	lastServedDayByHash    map[string]int64 // hash to day
	unsavedServedDayByHash map[string]int64 // served since the last flush
	pressure               Pressure
	sweepWake              chan struct{}
}

// Pressure is the artwork cache against its ceiling, as the last sweep found it.
type Pressure struct {
	TotalBytes    int64 // bytes held
	RequiredBytes int64 // bytes never evicted: what live items need to be shown
	CeilingBytes  int64
	// IsHighWater is what is never evicted exceeding the ceiling: artwork is still served and
	// fetched as needed, and the admin is told the ceiling to set (D212 §7).
	IsHighWater      bool
	RecommendedBytes int64 // the ceiling to set: what is required plus 15%, in whole GiB
}

func newHoldings() *holdings {
	return &holdings{
		sizeBytesByNameByHash: map[string]map[string]int64{}, lastServedDayByHash: map[string]int64{},
		unsavedServedDayByHash: map[string]int64{},
		sweepWake:              make(chan struct{}, 1),
	}
}

// LoadHoldings reads what the art directory holds and when each image was last served; call it once,
// before serving.
func (service *Service) LoadHoldings(ctx context.Context) error {
	files, err := service.parts.ArtFiles.ListArt()
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
		held.add(file.Hash, file.Name, file.SizeBytes)
	}
	for _, row := range rows {
		held.lastServedDayByHash[row.SourceSha256Hex] = row.LastServedEpochDay
	}
	return nil
}

func (held *holdings) add(hash, name string, size int64) {
	if held.sizeBytesByNameByHash[hash] == nil {
		held.sizeBytesByNameByHash[hash] = map[string]int64{}
	}
	held.totalBytes += size - held.sizeBytesByNameByHash[hash][name]
	held.sizeBytesByNameByHash[hash][name] = size
}

// writeArt stores one file and counts it; past the ceiling it wakes the sweep.
func (service *Service) writeArt(ctx context.Context, hash, name string, contents []byte) error {
	if err := service.parts.ArtFiles.WriteArt(hash, name, contents); err != nil {
		return fmt.Errorf("store art %s %s: %w", hash, name, err)
	}
	held := service.holdings
	held.mutex.Lock()
	held.add(hash, name, int64(len(contents)))
	total := held.totalBytes
	held.mutex.Unlock()
	ceiling, err := service.ceilingBytes(ctx)
	if err == nil && total > ceiling {
		select {
		case held.sweepWake <- struct{}{}:
		default:
		}
	}
	return nil
}

// noteServed records that an image was served today, in memory; Flush writes it.
func (service *Service) noteServed(hash string) {
	todayEpochDay := service.parts.Clock.Now().Unix() / int64(day/time.Second)
	held := service.holdings
	held.mutex.Lock()
	defer held.mutex.Unlock()
	if held.lastServedDayByHash[hash] < todayEpochDay {
		held.lastServedDayByHash[hash], held.unsavedServedDayByHash[hash] = todayEpochDay, todayEpochDay
	}
}

// Flush writes when images were served since the last flush, in one batch.
func (service *Service) Flush(ctx context.Context) error {
	held := service.holdings
	held.mutex.Lock()
	unsaved := held.unsavedServedDayByHash
	held.unsavedServedDayByHash = map[string]int64{}
	held.mutex.Unlock()
	if len(unsaved) == 0 {
		return nil
	}
	err := service.parts.Cache.Write(ctx, func(queries *cachegen.Queries) error {
		for hash, servedDay := range unsaved {
			marked := cachegen.MarkArtServedParams{SourceSha256Hex: hash, LastServedEpochDay: servedDay}
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

// SweepWakes is woken when a write takes the cache past its ceiling.
func (service *Service) SweepWakes() <-chan struct{} {
	return service.holdings.sweepWake
}

func (service *Service) ceilingBytes(ctx context.Context) (int64, error) {
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
	hash, name    string
	sizeBytes     int64
	class         class
	lastServedDay int64
	goneAt        int64
}

// Sweep evicts what the ceiling says to: removed items' images once their grace has passed,
// always; and past the ceiling, in the order of D212 §6, least recently served first within each
// class, down to 90% of it. It never evicts a live item's poster-wall size or the smallest size
// of each shape, so a request always has a size to go to. It returns the cache's pressure.
func (service *Service) Sweep(ctx context.Context) (Pressure, error) {
	ceiling, err := service.ceilingBytes(ctx)
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
	isLiveByHash := map[string]bool{}
	goneAtByHash := map[string]int64{}
	for _, row := range rows {
		key := hex.EncodeToString(row.SourceSha256)
		isLiveByHash[key], goneAtByHash[key] = row.Live == 1, row.GoneAt
	}
	sorted := service.classify(isLiveByHash, goneAtByHash)
	candidates, required, total := sorted.candidates, sorted.requiredBytes, sorted.totalBytes
	graceEndsAt := service.parts.Clock.Now().Add(-time.Duration(graceDays) * day).UnixMilli()
	isOverCeiling, lowWaterBytes := total > ceiling, int64(float64(ceiling)*lowWaterFraction)
	for _, file := range candidates {
		isPastGrace := file.class == classRemoved && file.goneAt < graceEndsAt
		if !isPastGrace && (!isOverCeiling || total <= lowWaterBytes) {
			continue
		}
		if err := service.evict(file); err != nil {
			return Pressure{}, err
		}
		total -= file.sizeBytes
	}
	pressure := Pressure{
		TotalBytes: total, RequiredBytes: required, CeilingBytes: ceiling, IsHighWater: required > ceiling,
	}
	if pressure.IsHighWater {
		pressure.RecommendedBytes = (int64(float64(required)*headroomFactor) + gib - 1) / gib * gib
	}
	service.recordPressure(ctx, pressure)
	return pressure, nil
}

// classified is every held file a sweep may evict, soonest first, with the bytes never evicted
// and the bytes held in all.
type classified struct {
	candidates    []candidate
	requiredBytes int64
	totalBytes    int64
}

// classify sorts every held file into its class.
func (service *Service) classify(isLiveByHash map[string]bool, goneAtByHash map[string]int64) classified {
	held := service.holdings
	held.mutex.Lock()
	defer held.mutex.Unlock()
	var candidates []candidate
	var required int64
	for hash, files := range held.sizeBytesByNameByHash {
		base := baseNameOf(files)
		for name, size := range files {
			file := candidate{
				hash: hash, name: name, sizeBytes: size, lastServedDay: held.lastServedDayByHash[hash],
				goneAt: goneAtByHash[hash],
			}
			file.class = classOf(name, base, isLiveByHash[hash])
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
		if first.lastServedDay != second.lastServedDay {
			return int(first.lastServedDay - second.lastServedDay)
		}
		return strings.Compare(first.hash+first.name, second.hash+second.name)
	})
	return classified{candidates: candidates, requiredBytes: required, totalBytes: held.totalBytes}
}

// classOf is a file's class.
func classOf(name, base string, isLive bool) class {
	switch {
	case strings.HasSuffix(name, cannotMakeSuffix):
		return classKept // an empty note that a size cannot be made
	case !isLive:
		return classRemoved
	case strings.HasSuffix(name, "."+FormatJPEG):
		return classOnDemand
	}
	size, known := nodeproto.ArtSizeName(strings.TrimSuffix(name, webpSuffix)).Size()
	switch {
	case !known:
		return classRemoved
	case size.Name == nodeproto.PortraitM || size.Name == size.Shape.Sizes()[0].Name:
		return classKept // the poster wall's size, and the smallest of each shape
	case size.IsMadeOnRequest:
		return classOnDemand
	case name == base:
		return classBase
	}
	return classOther
}

// baseNameOf is an image's largest WebP size held: what its other sizes are cut from.
func baseNameOf(files map[string]int64) string {
	base, width := "", 0
	for name := range files {
		size, known := nodeproto.ArtSizeName(strings.TrimSuffix(name, webpSuffix)).Size()
		if known && strings.HasSuffix(name, webpSuffix) && size.Width > width {
			base, width = name, size.Width
		}
	}
	return base
}

// evict removes one file and forgets it.
func (service *Service) evict(file candidate) error {
	if err := service.parts.ArtFiles.RemoveArt(file.hash, file.name); err != nil {
		return fmt.Errorf("evict art %s %s: %w", file.hash, file.name, err)
	}
	held := service.holdings
	held.mutex.Lock()
	defer held.mutex.Unlock()
	held.totalBytes -= held.sizeBytesByNameByHash[file.hash][file.name]
	delete(held.sizeBytesByNameByHash[file.hash], file.name)
	if len(held.sizeBytesByNameByHash[file.hash]) == 0 {
		delete(held.sizeBytesByNameByHash, file.hash)
	}
	return nil
}

// recordPressure keeps the sweep's finding and says so when high water begins or ends.
func (service *Service) recordPressure(ctx context.Context, pressure Pressure) {
	held := service.holdings
	held.mutex.Lock()
	wasHighWater := held.pressure.IsHighWater
	held.pressure = pressure
	held.mutex.Unlock()
	switch {
	case pressure.IsHighWater && !wasHighWater:
		service.parts.Logger.WarnContext(ctx, "the artwork cache ceiling is smaller than what the libraries need "+
			"to show; artwork is fetched again as it is needed", "requiredBytes", pressure.RequiredBytes,
			"ceilingBytes", pressure.CeilingBytes, "recommendedGiB", pressure.RecommendedBytes/gib)
	case !pressure.IsHighWater && wasHighWater:
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
