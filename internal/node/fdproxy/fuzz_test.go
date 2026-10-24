// SPDX-License-Identifier: AGPL-3.0-or-later

package fdproxy

import (
	"encoding/binary"
	"math"
	"testing"
	"time"
)

// fuzzBudget is the budget the first seeds' runs have: three times their film.
const fuzzBudget = 3000

// mostFuzzedBytes keeps what FFmpeg sends inside a pipe's default 64 KiB, so a write never waits on
// a node that stopped reading.
const mostFuzzedBytes = 60 << 10

// mostFuzzedSteps bounds a structured run, keeping each run of the fuzz short.
const mostFuzzedSteps = 200

// largestFuzzedFilm keeps the films small, and a 4 KiB data pipe still fills.
const largestFuzzedFilm = 16 << 10

// fuzzSettleLimit bounds each wait for the node to reach the state the model says: far beyond the
// microseconds it takes, so only a node that has parted from the model waits it out.
const fuzzSettleLimit = 10 * time.Second

// The ways a fuzzed run is driven.
const (
	// A burst writes FFmpeg's bytes all at once and closes them, so the reader races the writer;
	// what is served is checked against the order of the READs, not against when each came.
	fuzzBurst = iota
	// Paced writes them in chunks, waiting after each for the node to reach the model's state.
	fuzzPaced
	// Structured makes the frames from the input, mostly well formed, with the run's other
	// events between them: the film released, held or failing, Hold and Release, FFmpeg falling
	// silent, ceasing to read its DATA, or exiting with frames still to come.
	fuzzStructured
	fuzzModes
)

// FFmpeg may have been taken over by the film it reads, so whatever it sends, the node must judge it
// and end, and serve only what the specification lets it: the right bytes for each READ, in order and
// numbered as asked, none past the budget, none after a breach, and READs held to exactly the
// number agreed; its account is what FFmpeg sent and what the node did, and it kills the run
// exactly when it records a reason to while the run runs (Serving). The input sets the run (its
// film, run, READs outstanding, budget, pipe and film's behaviour), then is either FFmpeg's bytes or
// steps from which the fuzz makes them; fuzzModel says what the node must do with them. The seeds
// run with the package's tests; `go test -fuzz FuzzChannel_JudgesWhateverFFmpegSends
// ./internal/node/fdproxy/` explores beyond them.
func FuzzChannel_JudgesWhateverFFmpegSends(f *testing.F) {
	for _, seed := range fuzzSeeds() {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		input := &fuzzInput{raw: raw}
		setup := decodeFuzzSetup(input)
		source := newFuzzFilm(setup.filmSize, setup.behaviours)
		config := setup.config
		config.Source = source
		channel, err := Open(config)
		if setup.config.Run < 1 || setup.config.MostOutstanding < 1 || setup.config.MostOutstanding > mostOutstanding {
			if err == nil {
				channel.Abandon()
				t.Fatalf("Open took a config outside its rules: %+v", config)
			}
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		run := startFuzzRun(t, channel, source, setup)
		var account Account
		switch setup.mode {
		case fuzzBurst:
			account = run.burst(input.rest())
		case fuzzPaced:
			account = run.paced(input.rest())
		default:
			account = run.structured(input)
		}
		if budget := config.BudgetBytes; budget > 0 && account.ServedBytes > budget {
			t.Errorf("served %d bytes past the budget of %d", account.ServedBytes, budget)
		}
		// A burst's failures all come before Finish begins, so each one kills the run; the other
		// modes are held to the model's kill (fuzzRun.check).
		if setup.mode == fuzzBurst && run.killed.Load() != (account.Failure != nil) {
			t.Errorf("killed %v, but the account's failure is %v", run.killed.Load(), account.Failure)
		}
	})
}

// fuzzInput reads the fuzz's input as it is needed; past its end, every value is zero.
type fuzzInput struct{ raw []byte }

func (input *fuzzInput) exhausted() bool { return len(input.raw) == 0 }

func (input *fuzzInput) bytes(count int) []byte {
	count = min(count, len(input.raw))
	taken := input.raw[:count]
	input.raw = input.raw[count:]
	return taken
}

func (input *fuzzInput) rest() []byte { return input.bytes(len(input.raw)) }

// padded is the next count bytes, zeros past the input's end.
func (input *fuzzInput) padded(count int) []byte {
	raw := make([]byte, count)
	copy(raw, input.bytes(count))
	return raw
}

func (input *fuzzInput) nextByte() byte { return input.padded(1)[0] }

func (input *fuzzInput) next16() uint16 { return binary.LittleEndian.Uint16(input.padded(2)) }

func (input *fuzzInput) next32() uint32 { return binary.LittleEndian.Uint32(input.padded(4)) }

func (input *fuzzInput) next64() uint64 { return binary.LittleEndian.Uint64(input.padded(8)) }

// fuzzSetup is the run the input sets, from its first 14 bytes.
type fuzzSetup struct {
	mode     int
	config   Config
	filmSize int
	// chunk is the most bytes FFmpeg writes at once; zero writes each frame whole.
	chunk      int
	behaviours uint32
}

// decodeFuzzSetup reads the run: its mode, film size, run (0 is outside the rules, and 0xffff the
// largest), most READs outstanding (0 and 17 outside them), budget (0 is none), chunk, data pipe (a
// page, which a block fills, or the default) and the film's behaviour for each read.
func decodeFuzzSetup(input *fuzzInput) fuzzSetup {
	setup := fuzzSetup{mode: int(input.nextByte()) % fuzzModes, filmSize: int(input.next16()) % (largestFuzzedFilm + 1)}
	run := int64(input.next16())
	if run == math.MaxUint16 {
		run = math.MaxInt64
	}
	setup.config = Config{Run: run, MostOutstanding: int8(input.nextByte() % (mostOutstanding + 2))}
	setup.config.BudgetBytes = int64(input.next16())
	setup.chunk = int(input.nextByte())
	if input.nextByte()%2 == 1 {
		setup.config.DataPipeBytes = 4096
	}
	setup.behaviours = input.next32()
	return setup
}

// encodeFuzzSetup is decodeFuzzSetup's inverse, for the seeds.
func encodeFuzzSetup(setup fuzzSetup) []byte {
	run := uint16(setup.config.Run) //nolint:gosec // a seed's run is small
	if setup.config.Run == math.MaxInt64 {
		run = math.MaxUint16
	}
	// A seed's values are small, so each fits its field.
	//nolint:gosec // G115: as above
	raw := []byte{byte(setup.mode)}
	raw = binary.LittleEndian.AppendUint16(raw, uint16(setup.filmSize)) //nolint:gosec // G115: as above
	raw = binary.LittleEndian.AppendUint16(raw, run)
	raw = append(raw, byte(setup.config.MostOutstanding))                         //nolint:gosec // G115: as above
	raw = binary.LittleEndian.AppendUint16(raw, uint16(setup.config.BudgetBytes)) //nolint:gosec // G115: as above
	raw = append(raw, byte(setup.chunk))                                          //nolint:gosec // G115: as above
	pipe := byte(0)
	if setup.config.DataPipeBytes > 0 {
		pipe = 1
	}
	raw = append(raw, pipe)
	return binary.LittleEndian.AppendUint32(raw, setup.behaviours)
}

// fuzzSeeds are runs FFmpeg's side might make, from which the fuzzer mutates. First, bursts on the
// first form's terms: a whole session that keeps to the protocol, and one of each way to break it.
// Then structured runs that reach what a burst cannot.
func fuzzSeeds() [][]byte {
	first := encodeFuzzSetup(fuzzSetup{
		mode: fuzzBurst, filmSize: 1000, config: Config{Run: 7, MostOutstanding: 2, BudgetBytes: fuzzBudget},
	})
	burst := func(stream []byte) []byte { return append(append([]byte(nil), first...), stream...) }
	hello := frame(typeHello, 0, helloBody(versionRange{lowest: 1, highest: 1}))
	offer := frame(typeCapability, 1, put(put(put([]byte(nil), int32(256)), int32(1024)), int8(2)))
	opening := append(append([]byte(nil), hello...), offer...)
	session := append([]byte(nil), opening...)
	sequence := int8(2)
	for offset := int64(0); offset < 1000; offset += 256 {
		length := int32(min(256, 1000-offset))
		session = append(session, frame(typeRead, sequence, put(put([]byte(nil), offset), length))...)
		sequence++
	}
	session = append(session, frame(typeDone, sequence, put(put([]byte(nil), int64(7)), int8(resultOK)))...)
	seeds := [][]byte{
		burst(session),
		burst(opening),
		burst(nil),
		burst(hello[:3]),
		burst(append(append([]byte(nil), opening...), frame(typeRead, 2, readBody(900, 200))...)),
		burst(append(append([]byte(nil), opening...), frame(typeRead, 9, readBody(0, 256))...)),
		burst(append(append([]byte(nil), opening...), frame(typeDone, 2, []byte{7, 0, 0, 0, 0, 0, 0, 0, 1, 2})...)),
		burst(frame(typeHello, 0, helloBody(versionRange{lowest: 2, highest: 3}))),
	}
	paced := encodeFuzzSetup(fuzzSetup{
		mode: fuzzPaced, filmSize: 1000, chunk: 5, config: Config{Run: 7, MostOutstanding: 2, BudgetBytes: fuzzBudget},
	})
	seeds = append(seeds, append(paced, session...))
	return append(seeds, structuredSeeds()...)
}

// The film's behaviours for the seeds, two bits a read: every read gated, or the third failing.
const (
	everyReadGated   = 0xaaaaaaaa
	theThirdReadFail = 0x30
)

// structuredSeeds are structured runs, each reaching a state a burst cannot: a read waiting while
// FFmpeg breaks the protocol, the READs outstanding filled exactly, a held run, a failing film, the
// budget passed, a DONE that failed, frames after Finish has begun, and FFmpeg ceasing to read a
// data pipe of a page.
func structuredSeeds() [][]byte {
	setupOf := func(size int, behaviours uint32, budget int64, pipe int) []byte {
		return encodeFuzzSetup(fuzzSetup{
			mode: fuzzStructured, filmSize: size, behaviours: behaviours,
			config: Config{Run: 7, MostOutstanding: 2, BudgetBytes: budget, DataPipeBytes: pipe},
		})
	}
	setup := func(behaviours uint32, budget int64) []byte { return setupOf(1000, behaviours, budget, 0) }
	// offer is a valid HELLO and a CAPABILITY offering blocks of block bytes, the largest body 768
	// more, and 4 READs outstanding.
	offer := func(block uint16) []byte {
		return append(binary.LittleEndian.AppendUint16([]byte{1, 1}, block-1), 0, 3, 3)
	}
	read := func(offset, length uint16) []byte {
		raw := binary.LittleEndian.AppendUint16([]byte{stepRead}, offset)
		return binary.LittleEndian.AppendUint16(raw, length-1)
	}
	seedOf := func(head, opening []byte, parts ...[]byte) []byte {
		raw := append(append([]byte(nil), head...), opening...)
		for _, part := range parts {
			raw = append(raw, part...)
		}
		return raw
	}
	seed := func(head []byte, parts ...[]byte) []byte { return seedOf(head, offer(256), parts...) }
	doneOK := []byte{stepDone, 0}
	pastTheEnd := append(binary.LittleEndian.AppendUint64([]byte{stepReadFuzzed}, 900), 200, 0, 0, 0)
	failed := append([]byte{stepDone, 0x08, 2, 5, 0, 3}, "EIO"...)
	return [][]byte{
		seed(setup(0, 0), read(0, 256), read(256, 256), read(512, 256), read(768, 232), doneOK),
		seed(setup(everyReadGated, 0), read(0, 256), pastTheEnd, []byte{stepReleaseFilm}),
		seed(setup(everyReadGated, 0), read(0, 256), read(256, 256), read(512, 256), read(768, 232)),
		seed(setup(everyReadGated, 0), read(0, 256), read(256, 256), doneOK, []byte{stepReleaseFilm}, read(0, 10)),
		seed(setup(0, 0), []byte{stepHold}, read(0, 256), read(256, 256), []byte{stepRelease}, doneOK),
		seed(setup(theThirdReadFail, 0), read(0, 256), read(256, 256), read(512, 256), read(768, 232), doneOK),
		seed(setup(0, 600), read(0, 256), read(256, 256), read(512, 256)),
		seed(setup(0, 0), read(0, 256), failed),
		seed(setup(everyReadGated, 0), read(0, 256), []byte{stepFinish}, read(256, 256), read(512, 256),
			read(768, 232), pastTheEnd),
		seed(setup(0, 0), read(0, 256), []byte{stepGarbage, 2, byte(typeRead), 3}),
		// A READ's header, numbered 3, whose body never comes.
		seed(setup(0, 0), read(0, 256), []byte{stepGarbage, 5, byte(typeRead), 3, readBytes, 0, 0, 0}),
		seedOf(setupOf(largestFuzzedFilm, everyReadGated, 0, 4096), offer(8192), read(0, 8192), read(8192, 8192),
			[]byte{stepStall, stepReleaseFilm, stepReleaseFilm}),
	}
}
