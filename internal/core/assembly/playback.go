// SPDX-License-Identifier: AGPL-3.0-or-later

package assembly

import (
	"github.com/kstenerud/orpheum/internal/core/chunks"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/loudness"
	"github.com/kstenerud/orpheum/internal/core/playback"
	"github.com/kstenerud/orpheum/internal/core/readthrough"
	"github.com/kstenerud/orpheum/internal/core/subtitles"
)

// openPlayback is what plays a file and the work a play asks of the nodes: the read-through for its
// sounds' loudness and its picture's timeline, its subtitles and its chunks, and the playback
// service.
func (services *Services) openPlayback(parts Parts) {
	database, queue := parts.Database, services.Queue
	services.ReadThrough = readthrough.NewService(readthrough.Parts{
		Database: database, Queue: queue, Nodes: services.nodes, Exchanges: services.Exchanges, Clock: parts.Clock,
		Logger: parts.Logger,
	})
	levels := loudness.NewService(loudness.Parts{Database: database, Settings: parts.Settings})
	services.subtitles = subtitles.NewService(subtitles.Parts{
		Database: database, Queue: queue, Nodes: services.nodes, Exchanges: services.Exchanges,
		Subtitler: services.subtitler, Clock: parts.Clock, MaxSilence: nodeWorkMaxSilence,
	})
	services.chunks = chunks.NewService(chunks.Parts{
		Database: database, Cache: parts.Cache, Settings: parts.Settings, Queue: queue, Nodes: services.nodes,
		Exchanges: services.Exchanges, Loudness: levels, ReadThrough: services.ReadThrough, Logger: parts.Logger,
		Clock: parts.Clock, Encoders: services.nodes, Absences: services.ingester.Absences(),
	})
	var reads playback.FileReads = services.Scans
	if parts.FileReads != nil {
		reads = parts.FileReads
	}
	services.playback = playback.NewService(playback.Parts{
		Database: database, Nodes: services.nodes, Settings: parts.Settings, Clock: parts.Clock,
		Random: parts.Random, Subtitles: services.subtitles,
		Preferences: services.API.Preferences, Loudness: levels, Chunks: services.chunks, Offers: services.chunks,
		Reads: reads, Roots: services.Scans, Absences: services.ingester.Absences(),
	})
	// A run its node cannot make fast enough moves its players down a quality, before it is evicted
	// (D286 §4, D309 §6).
	services.chunks.MovePlayersBy(services.playback.MoveOffStream)
	// The nodes keep the chunk stores by how strongly the players want them (D309 §8).
	services.nodes.SetStoreTiers(services.playback.StoreTiers)
	// A file read again whose probe differs throws out what the differences touch, each thing by the
	// package that keeps it (D392 §3).
	services.ingester.SetDerivedDroppers(derivedDroppers(services))
}

// derivedDroppers are how each thing derived from a file's probe is thrown out once the facts it was
// made from change (library.DerivedDroppers): the read-through's measurements and timelines, the
// subtitles' texts, the frame stills and the chunk stores.
func derivedDroppers(services *Services) library.DerivedDroppers {
	return library.DerivedDroppers{
		Loudness: readthrough.ForgetLoudness, Timeline: readthrough.ForgetTimeline,
		SubtitleText: subtitles.ForgetText, FrameStills: services.artwork.RetireFrameStills,
		PictureChunks: services.chunks.ForgetPictureStores, SoundChunks: services.chunks.ForgetSoundStores,
	}
}

// playingFiles are the files played now, as the playback service last heard them: a playback a
// film's page warmed up, which has not played, is no one watching (D324 §2).
func playingFiles(service *playback.Service) func() []int64 {
	return func() []int64 { return filesPlayed(service.NowPlaying()) }
}

// filesPlayed are the files of the playbacks that have played.
func filesPlayed(nowPlaying []playback.NowPlaying) []int64 {
	var files []int64
	for _, playing := range nowPlaying {
		if playing.HasPlayed {
			files = append(files, playing.FileID)
		}
	}
	return files
}
