// SPDX-License-Identifier: AGPL-3.0-or-later

package probe

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/kstenerud/orpheum/internal/node/disc"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/langtag"
)

// ParseDisc turns ffprobe's JSON of a disc's main title into the probe record, with what only the
// disc says (D350 §4): which disc and title it is, its duration (a program stream's is estimated),
// its chapters when the stream has none, and each stream's language by the identifier the disc
// gives it, where the stream carries none.
func ParseDisc(output []byte, title disc.Title) (nodeproto.Probe, error) {
	var raw rawOutput
	if err := json.Unmarshal(output, &raw); err != nil {
		return nodeproto.Probe{}, fmt.Errorf("ffprobe's output is not the JSON it promised: %w", err)
	}
	record, err := raw.toProbe()
	if err != nil {
		return nodeproto.Probe{}, err
	}
	record.Format.Disc = &nodeproto.ProbeDisc{Kind: string(title.Kind), TitleNumber: title.Number}
	if title.DurationMs > 0 {
		record.Format.DurationMs = title.DurationMs
	}
	if len(title.ChaptersMs) > maxChapters {
		return nodeproto.Probe{}, fmt.Errorf("%w: %d chapters (at most %d)",
			ErrTooLarge, len(title.ChaptersMs), maxChapters)
	}
	if len(record.Chapters) == 0 {
		record.Chapters = discChapters(title)
	}
	for index, stream := range raw.Streams {
		if record.Streams[index].ContainerLanguage != "" {
			continue
		}
		if language := trackLanguage(title, stream); language != "" {
			record.Streams[index].ContainerLanguage = language
			record.Streams[index].Language = langtag.BCP47(language)
		}
	}
	return record, nil
}

// trackLanguage is the language the disc gives the stream of its kind with its identifier, or "".
func trackLanguage(title disc.Title, stream rawStream) string {
	id, err := strconv.ParseInt(stream.ID, 0, 64)
	if err != nil {
		return ""
	}
	for _, track := range title.Tracks {
		if track.ID == id && track.Kind == stream.CodecType {
			return boundedText(track.Language)
		}
	}
	return ""
}

// discChapters are the title's chapters, each ending where the next starts and the last at its end.
func discChapters(title disc.Title) []nodeproto.ProbeChapter {
	chapters := make([]nodeproto.ProbeChapter, 0, len(title.ChaptersMs))
	for index, startMs := range title.ChaptersMs {
		endMs := title.DurationMs
		if index+1 < len(title.ChaptersMs) {
			endMs = title.ChaptersMs[index+1]
		}
		chapters = append(chapters, nodeproto.ProbeChapter{StartMs: startMs, EndMs: endMs})
	}
	return chapters
}
