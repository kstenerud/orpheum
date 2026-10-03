// SPDX-License-Identifier: GPL-3.0-or-later

package library

import (
	"github.com/kstenerud/orpheum/internal/core/textindex"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// cleanRecord cleans the text in a node's record where it enters the core (principles §22,
// D211): what a container's tags and a platform's error text say is attacker-controlled —
// a track title, a chapter name, stderr quoting a filename — and past here every one of them is
// safe to store, relate and show. Paths are cleaned by nodePath, which also keeps their bytes.
func cleanRecord(record nodeproto.ScanRecord) nodeproto.ScanRecord {
	if record.Observed != nil {
		observed := *record.Observed
		observed.Probe = cleanProbe(observed.Probe)
		record.Observed = &observed
	}
	if record.Skip != nil {
		skip := *record.Skip
		skip.Rule = nodeproto.SkipRule(textindex.Line(string(skip.Rule)))
		record.Skip = &skip
	}
	if record.Error != nil {
		failure := *record.Error
		failure.Code = nodeproto.ErrorCode(textindex.Line(string(failure.Code)))
		failure.PlatformError.Text = textindex.Prose(failure.PlatformError.Text)
		failure.PlatformError.ModeText = textindex.Line(failure.PlatformError.ModeText)
		record.Error = &failure
	}
	return record
}

// cleanProbe cleans every string a probe carries: every one came from the file's container.
func cleanProbe(probe nodeproto.Probe) nodeproto.Probe {
	probe.Format.ContainerTitle = textindex.Line(probe.Format.ContainerTitle)
	probe.Format.FormatNames = cleanAll(probe.Format.FormatNames)
	streams := make([]nodeproto.ProbeStream, len(probe.Streams))
	for index, stream := range probe.Streams {
		stream.Kind, stream.Codec = textindex.Line(stream.Kind), textindex.Line(stream.Codec)
		stream.CodecProfile, stream.ContainerTitle = textindex.Line(stream.CodecProfile),
			textindex.Line(stream.ContainerTitle)
		stream.ContainerLanguage, stream.Language = textindex.Line(stream.ContainerLanguage),
			textindex.Line(stream.Language)
		stream.Disposition = cleanAll(stream.Disposition)
		stream.Video.PixelFormat = textindex.Line(stream.Video.PixelFormat)
		stream.Video.FrameRate = textindex.Line(stream.Video.FrameRate)
		stream.Video.ColorTransfer = textindex.Line(stream.Video.ColorTransfer)
		stream.Video.ColorPrimaries = textindex.Line(stream.Video.ColorPrimaries)
		stream.Audio.ChannelLayout = textindex.Line(stream.Audio.ChannelLayout)
		stream.Attachment.Filename = textindex.Line(stream.Attachment.Filename)
		stream.Attachment.MimeType = textindex.Line(stream.Attachment.MimeType)
		streams[index] = stream
	}
	probe.Streams = streams
	chapters := make([]nodeproto.ProbeChapter, len(probe.Chapters))
	for index, chapter := range probe.Chapters {
		chapter.ContainerTitle = textindex.Line(chapter.ContainerTitle)
		chapters[index] = chapter
	}
	probe.Chapters = chapters
	return probe
}

func cleanAll(texts []string) []string {
	if texts == nil {
		return nil
	}
	cleaned := make([]string, len(texts))
	for index, text := range texts {
		cleaned[index] = textindex.Line(text)
	}
	return cleaned
}
