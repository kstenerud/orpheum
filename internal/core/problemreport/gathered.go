// SPDX-License-Identifier: AGPL-3.0-or-later

package problemreport

import (
	"time"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// gatheredReport is a report as it is kept, in problem_report.gathered_json, beside the columns that
// say who made it, when, from which version and in their words: everything else gathered, nothing
// made safe. Each download renders it.
type gatheredReport struct {
	Salt       []byte              `json:"salt"`
	DeviceName string              `json:"deviceName,omitempty"`
	Page       string              `json:"page,omitempty"`
	Device     []gatheredFact      `json:"device,omitempty"`
	Playback   *gatheredPlayback   `json:"playback,omitempty"`
	Components []gatheredComponent `json:"components"`
}

// gatheredFact is one of the device's facts.
type gatheredFact struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// gatheredPlayback is the player's state, and its file as the core knew it.
type gatheredPlayback struct {
	PlaybackID int64         `json:"playbackId,omitempty"`
	ItemID     int64         `json:"itemId,omitempty"`
	FileID     int64         `json:"fileId,omitempty"`
	PositionMs int64         `json:"positionMs"`
	Quality    string        `json:"quality,omitempty"`
	State      string        `json:"state,omitempty"`
	File       *gatheredFile `json:"file,omitempty"`
}

// gatheredFile is the file played: the path its node reported, its size and its kept probe.
type gatheredFile struct {
	Path      string          `json:"path"`
	SizeBytes int64           `json:"sizeBytes"`
	Probe     nodeproto.Probe `json:"probe"`
}

// gatheredComponent is the core or one node, with its recent log as the node protocol writes it, each
// value of its kind, or why it gave none.
type gatheredComponent struct {
	Name    string              `json:"name"`
	Node    bool                `json:"node,omitempty"`
	Version string              `json:"version,omitempty"`
	Lines   []nodeproto.LogLine `json:"lines,omitempty"`
	Absent  string              `json:"absent,omitempty"`
}

// report is the kept report as Render takes it, made at made by version in words; its reporter's
// name, its known values and other roots are the caller's.
func (gathered gatheredReport) report(made time.Time, version, words string) Report {
	report := Report{
		Made: made, Version: version, Salt: gathered.Salt, Words: words, Page: gathered.Page,
		Reporter: Reporter{DeviceName: gathered.DeviceName},
	}
	for _, fact := range gathered.Device {
		report.Device = append(report.Device, Fact(fact))
	}
	if playback := gathered.Playback; playback != nil {
		report.Playback = &Playback{
			PlaybackID: playback.PlaybackID, ItemID: playback.ItemID, FileID: playback.FileID,
			Position: time.Duration(playback.PositionMs) * time.Millisecond, Quality: playback.Quality,
			State: playback.State,
		}
		if playback.File != nil {
			report.Playback.File = playback.File.shape()
		}
	}
	for _, component := range gathered.Components {
		log := nodeproto.RecentLog{Lines: component.Lines}
		report.Components = append(report.Components, Component{
			Name: component.Name, Node: component.Node, Version: component.Version, Lines: log.SlogLines(),
			Absent: component.Absent,
		})
	}
	return report
}

// shape is the file's shape, its streams' and container's titles left behind (D74).
func (file gatheredFile) shape() *FileShape {
	shape := &FileShape{
		Path: file.Path, SizeBytes: file.SizeBytes,
		Duration: time.Duration(file.Probe.Format.DurationMs) * time.Millisecond,
	}
	if names := file.Probe.Format.FormatNames; len(names) > 0 {
		shape.Container = names[len(names)-1]
	}
	for _, stream := range file.Probe.Streams {
		shape.Streams = append(shape.Streams, Stream{
			Index: stream.StreamIndex, Kind: stream.Kind, Codec: stream.Codec, Profile: stream.CodecProfile,
			BitDepth: stream.Video.BitDepth(), Width: stream.Video.Width, Height: stream.Video.Height,
			FrameRate: stream.Video.FrameRate, ChannelLayout: stream.Audio.ChannelLayout, Language: stream.Language,
			Dispositions: stream.Disposition,
		})
	}
	return shape
}
