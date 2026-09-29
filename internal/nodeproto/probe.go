// SPDX-License-Identifier: GPL-3.0-or-later

package nodeproto

// Probe is what the node observed inside one media file: the parsed, bounded record the core
// stores verbatim as file.probe (schema.md) and re-derives from. It is built by the node from
// ffprobe's output, never passed through; hostile bytes stop at the node (D126). Every string
// is valid UTF-8 and bounded in length, and the lists are bounded in count.
type Probe struct {
	Format   ProbeFormat    `json:"format"`
	Streams  []ProbeStream  `json:"streams"`
	Chapters []ProbeChapter `json:"chapters"`
}

// ProbeFormat is the container.
type ProbeFormat struct {
	// Names are the demuxer's names for the container, most specific last, as ffprobe lists
	// them ("matroska", "webm"). The extension is a hint; this is what the file is.
	Names      []string `json:"names"`
	DurationMs int64    `json:"durationMs"`
	BitRateBps int64    `json:"bitRateBps"`
	Title      string   `json:"title"`
}

// ProbeStream is one track, as the container describes it (scanner.md, Track facts).
type ProbeStream struct {
	StreamIndex int `json:"streamIndex"`
	// Kind is ffprobe's codec_type: video, audio, subtitle, attachment or data.
	Kind         string `json:"kind"`
	Codec        string `json:"codec"`
	CodecProfile string `json:"codecProfile"`
	// ContainerLanguage is the tag exactly as the container carried it ("ger", "deu", "en").
	ContainerLanguage string `json:"containerLanguage"`
	// LanguageTag is Language in BCP 47 form ("de" for both "ger" and "deu"), or empty when the
	// container carried none, "und", or a code no standard knows (scanner.md, D206).
	LanguageTag string `json:"languageTag"`
	// Title is the track title, verbatim: often the only signal of a track's kind.
	Title string `json:"title"`
	// Disposition lists the flags ffprobe reported as set, sorted: "default", "forced",
	// "hearing_impaired", "comment", "descriptions", "dub", "original", ...
	Disposition []string `json:"disposition"`
	DurationMs  int64    `json:"durationMs"`
	BitRateBps  int64    `json:"bitRateBps"`

	Video ProbeVideo `json:"video"`
	Audio ProbeAudio `json:"audio"`
	// Attachment names an attached file, such as a font for ASS subtitles.
	Attachment ProbeAttachment `json:"attachment"`
}

// ProbeVideo holds a video stream's picture facts; zero for other kinds.
type ProbeVideo struct {
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	PixelFormat string `json:"pixelFormat"`
	// FrameRate is ffprobe's r_frame_rate, a ratio such as "24000/1001".
	FrameRate      string `json:"frameRate"`
	ColorTransfer  string `json:"colorTransfer"`
	ColorPrimaries string `json:"colorPrimaries"`
}

// ProbeAudio holds an audio stream's sound facts; zero for other kinds.
type ProbeAudio struct {
	ChannelCount  int    `json:"channelCount"`
	ChannelLayout string `json:"channelLayout"`
	SampleRateHz  int    `json:"sampleRateHz"`
}

// ProbeAttachment names an attached file; zero for other kinds.
type ProbeAttachment struct {
	Filename string `json:"filename"`
	MimeType string `json:"mimeType"`
}

// ProbeChapter is one chapter mark.
type ProbeChapter struct {
	StartMs int64  `json:"startMs"`
	EndMs   int64  `json:"endMs"`
	Title   string `json:"title"`
}
