// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build integration

package chunks_test

import (
	"encoding/binary"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/chunks"
	"github.com/kstenerud/orpheum/internal/node/encode"
	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/node/prepared"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// A RealMedia film's sound interleaved in super-blocks, as cook is, is whole and at its times in
// every run: FFmpeg's demuxer loses the super-block a seek lands in, so a run from the film's start
// told to seek there lost its first, and one led in two seconds early the one its seek landed in,
// each run's sound moved earlier by what it lost (file 15102 on nas.example, 2026-10-27, whose sound
// never started; idiosyncrasies.md). Two runs' chunks join into one sound, from its priming to the
// film's end.
func TestMake_AnInterleavedRealMediaSoundIsWholeAndAtItsTimesInEveryRun(t *testing.T) {
	t.Parallel()
	runner, err := execx.NewRunner(testutil.FFmpegDirectory(t))
	if err != nil {
		t.Fatal(err)
	}
	source := realMediaWithInterleavedSound(t, 24)
	directory := t.TempDir()
	shelf := prepared.New(directory, prepared.Budget{MaxBytes: 1 << 30, DiskUsedLimitPercent: 100},
		clocktest.NewFake(time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)), slog.New(slog.DiscardHandler))
	maker := chunks.NewMaker(runner, shelf, directory, encode.EncoderX264)
	storeDir := "1/chunks-sound0-96k-raw/0123456789abcdef"
	stream := nodeproto.ChunkStream{
		Kind: nodeproto.StreamSound, SoundKbps: 96, Sound: &nodeproto.SoundTrack{ChannelCount: 1},
	}
	// The second run's lead-in starts at 10 s: a seek two seconds before it lands in the super-block
	// from 7.68 s, which is lost through 10.24 s.
	grid := []int64{0, 5_000, 10_000, 13_000, 16_000, 20_000, 24_000}
	for first, leadInMs := range map[int]int64{0: 0, 3: 10_000} {
		command := nodeproto.MakeChunks{
			VideoStreamIndex: 0, VideoCodec: "rv20", Stream: stream, StoreDir: storeDir,
			FirstChunk: first, BoundsMs: grid[first : first+4], LeadInMs: leadInMs, IsSoundInterleaved: true,
		}
		if last, err := maker.Make(t.Context(), execx.Film{Path: source}, command, nil); err != nil || last != first+2 {
			t.Fatalf("the run from %d made through %d, %v", first, last, err)
		}
	}
	times := packetTimes(t, runner, filepath.Join(directory, storeDir), len(grid)-1)
	const frame = 1024.0 / 8000
	for index := 1; index < len(times); index++ {
		if step := times[index] - times[index-1]; step < frame/2 || step > 2*frame {
			t.Errorf("the sound steps %.4f s at %.3f s; want about a frame, %.4f s", step, times[index], frame)
		}
	}
	offset := float64(chunks.TimelineOffsetMs) / 1000
	// The sound ends with its last whole super-block, at 23.04 s.
	end := float64(24_000/interleavedSuperBlockMs*interleavedSuperBlockMs) / 1000
	if len(times) == 0 || math.Abs(times[0]-(offset-frame)) > 0.002 || times[len(times)-1] < offset+end-frame-0.002 {
		t.Fatalf("the sound runs %v; want from its priming, a frame before the film, to its end", times)
	}
}

// RealAudio 28.8 interleaved as RealMedia carries it (Int4), as it carries cook (genr): each packet
// holds half a column of the super-block's frames, so a super-block of interleavedHeight packets is
// decoded only whole, and timed by its first packet (libavformat/rmdec.c). The pinned FFmpeg encodes
// no interleaved RealAudio, cook least of all, so the film's sound is written here, packet by packet.
const (
	interleavedHeight      = 16  // packets to a super-block: sub_packet_h
	interleavedFrameBytes  = 38  // one coded 28.8 frame: 160 samples at 8 kHz
	interleavedPacketBytes = 304 // a packet: interleavedHeight/2 frames
	interleavedFrameMs     = 20
)

// interleavedSuperBlockMs is how long a super-block plays: every frame of its packets.
const interleavedSuperBlockMs = interleavedHeight * interleavedPacketBytes / interleavedFrameBytes * interleavedFrameMs

// realMediaWithInterleavedSound makes a RealMedia film of seconds, its picture RealVideo 2.0 with a
// keyframe every second, and its sound RealAudio 28.8 interleaved in super-blocks of
// interleavedSuperBlockMs, 2.56 s, as a film with cook's sound is: FFmpeg writes the picture, and
// this the sound beside it, every packet in time order, and an index of the picture's keyframes.
// Its sound's bytes are no recording, but decode as 28.8 decodes any.
func realMediaWithInterleavedSound(t *testing.T, seconds int) string {
	t.Helper()
	pictureOnly := filepath.Join(t.TempDir(), "picture.rm")
	testutil.GenerateMedia(t, pictureOnly,
		"-f", "lavfi", "-i", "testsrc2=size=160x120:rate=24:duration="+strconv.Itoa(seconds),
		"-c:v", "rv20", "-g", "24", "-b:v", "200k", "-an")
	contents, err := os.ReadFile(pictureOnly)
	if err != nil {
		t.Fatal(err)
	}
	pictureHeader, picturePackets := realMediaPictureOf(t, contents)
	packets := mergedByTime(picturePackets, interleavedSoundPackets(seconds*1000))
	soundHeader := interleavedSoundHeader()
	headers := join(pictureHeader, realMediaChunk("MDPR", be16(0), be16(1), be32(0), be32(4000),
		be32(interleavedPacketBytes), be32(interleavedPacketBytes), be32(0), be32(0), be32(seconds*1000),
		str8(""), str8("audio/x-pn-realaudio"), be32(len(soundHeader)), soundHeader))
	const fileHeaderBytes, propertiesBytes, dataHeaderBytes = 18, 50, 18
	dataAt := fileHeaderBytes + propertiesBytes + len(headers)
	// The index lists the picture's keyframes, where FFmpeg's seeks land, as RealProducer's does.
	var data, entries []byte
	keyframes := 0
	for _, packet := range packets {
		if binary.BigEndian.Uint16(packet[4:6]) == 0 && packet[11]&2 != 0 {
			at := dataAt + dataHeaderBytes + len(data)
			entries = append(entries, join(be16(0), packet[6:10], be32(at), be32(0))...)
			keyframes++
		}
		data = append(data, packet...)
	}
	dataChunk := realMediaChunk("DATA", be16(0), be32(len(packets)), be32(0), data)
	file := join(
		realMediaChunk(".RMF", be16(0), be32(0), be32(5)),
		realMediaChunk("PROP", be16(0), be32(0), be32(0), be32(0), be32(0), be32(len(packets)),
			be32(seconds*1000), be32(0), be32(dataAt+len(dataChunk)), be32(dataAt),
			be16(2), be16(0)),
		headers, dataChunk,
		realMediaChunk("INDX", be16(0), be32(keyframes), be16(0), be32(0), entries),
	)
	path := filepath.Join(t.TempDir(), "a film (2001).rm")
	if err := os.WriteFile(path, file, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// realMediaPictureOf is a picture-only RealMedia file's picture: its stream's header chunk, MDPR, and
// its packets, each whole with its header.
func realMediaPictureOf(t *testing.T, contents []byte) (header []byte, packets [][]byte) {
	t.Helper()
	chunkAt := int(binary.BigEndian.Uint32(contents[4:8])) // past .RMF
	for chunkAt+10 <= len(contents) {
		tag := string(contents[chunkAt : chunkAt+4])
		size := int(binary.BigEndian.Uint32(contents[chunkAt+4 : chunkAt+8]))
		switch tag {
		case "MDPR":
			header = append(header, contents[chunkAt:chunkAt+size]...)
		case "DATA":
			count := int(binary.BigEndian.Uint32(contents[chunkAt+10 : chunkAt+14]))
			packetAt := chunkAt + 18
			for range count {
				length := int(binary.BigEndian.Uint16(contents[packetAt+2 : packetAt+4]))
				packets = append(packets, contents[packetAt:packetAt+length])
				packetAt += length
			}
			if header == nil || len(packets) == 0 {
				t.Fatal("FFmpeg's RealMedia film has no picture")
			}
			return header, packets
		}
		if size < 10 {
			break
		}
		chunkAt += size
	}
	t.Fatal("FFmpeg's RealMedia film has no DATA chunk")
	return nil, nil
}

// interleavedSoundHeader is the sound's stream header: RealAudio's own, version 4, for 28.8 at 8 kHz
// mono, interleaved by Int4 (rmdec.c rm_read_audio_stream_info).
func interleavedSoundHeader() []byte {
	return join(
		[]byte(".ra\xfd"), be16(4), be16(0), []byte(".ra4"), be32(0), be16(4), be32(0),
		be16(0), be32(interleavedFrameBytes), be32(0), be32(4000*60/8), be32(0),
		be16(interleavedHeight), be16(interleavedPacketBytes), be16(interleavedFrameBytes), be16(0),
		be16(8000), be32(0), be16(1),
		str8("Int4"), str8("28_8"),
	)
}

// interleavedSoundPackets are the sound's packets through ms of film: each super-block's first
// flagged a keyframe, every packet stamped at its share of its super-block, the bytes a pattern.
func interleavedSoundPackets(ms int) [][]byte {
	var packets [][]byte
	packetMs := interleavedSuperBlockMs / interleavedHeight
	for blockMs := 0; blockMs+interleavedSuperBlockMs <= ms; blockMs += interleavedSuperBlockMs {
		for index := range interleavedHeight {
			payload := make([]byte, interleavedPacketBytes)
			for at := range payload {
				payload[at] = lowByte((blockMs/interleavedSuperBlockMs)*31 + index*7 + at*13)
			}
			flags := byte(0)
			if index == 0 {
				flags = 2
			}
			packets = append(packets, realMediaPacket(1, blockMs+index*packetMs, flags, payload))
		}
	}
	return packets
}

// mergedByTime is the picture's packets and the sound's in one sequence by time, the sound's first, as
// RealProducer writes them: a seek to a picture's keyframe lands past the sound's packets at its time.
func mergedByTime(picture, sound [][]byte) [][]byte {
	stamp := func(packet []byte) uint32 { return binary.BigEndian.Uint32(packet[6:10]) }
	merged := make([][]byte, 0, len(picture)+len(sound))
	for len(picture) > 0 || len(sound) > 0 {
		if len(sound) == 0 || (len(picture) > 0 && stamp(picture[0]) < stamp(sound[0])) {
			merged, picture = append(merged, picture[0]), picture[1:]
		} else {
			merged, sound = append(merged, sound[0]), sound[1:]
		}
	}
	return merged
}

// realMediaPacket is one packet of a DATA chunk: version 0, its length, stream, time and flags.
func realMediaPacket(stream, timestampMs int, flags byte, payload []byte) []byte {
	return join(be16(0), be16(12+len(payload)), be16(stream), be32(timestampMs), []byte{0, flags}, payload)
}

// realMediaChunk is a chunk of a RealMedia file: its tag, its whole size, then its fields.
func realMediaChunk(tag string, fields ...[]byte) []byte {
	body := join(fields...)
	return join([]byte(tag), be32(8+len(body)), body)
}

func join(parts ...[]byte) []byte {
	var joined []byte
	for _, part := range parts {
		joined = append(joined, part...)
	}
	return joined
}

// be16, be32 and lowByte are fields of a RealMedia file, big-endian, each a small number of the
// fixture's own.
func be16(value int) []byte { return binary.BigEndian.AppendUint16(nil, uint16(value)) } //nolint:gosec // G115: small

func be32(value int) []byte { return binary.BigEndian.AppendUint32(nil, uint32(value)) } //nolint:gosec // G115: small

func lowByte(value int) byte { return byte(value & 0xff) }

func str8(text string) []byte { return append([]byte{lowByte(len(text))}, text...) }
