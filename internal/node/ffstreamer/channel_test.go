// SPDX-License-Identifier: AGPL-3.0-or-later

package ffstreamer

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// film is a Source over bytes in memory, which can fail its reads.
type film struct {
	bytes    []byte
	failAt   int64 // a read at this offset fails; -1 for none
	reads    atomic.Int32
	readGate chan struct{} // when set, each read waits for a value on it
}

func newFilm(size int) *film {
	contents := make([]byte, size)
	for index := range contents {
		contents[index] = byte(index * 7)
	}
	return &film{bytes: contents, failAt: -1}
}

func (source *film) Size() int64 { return int64(len(source.bytes)) }

func (source *film) ReadAt(ctx context.Context, buffer []byte, offset int64) error {
	source.reads.Add(1)
	if source.readGate != nil {
		select {
		case <-source.readGate:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if offset == source.failAt {
		return errors.New("input/output error")
	}
	copy(buffer, source.bytes[offset:])
	return nil
}

// fakeFFmpeg is FFmpeg's side of a channel, played by the test over duplicates of FFmpeg's ends,
// as a started FFmpeg holds its own copies. Killing it closes them, as a kill does.
type fakeFFmpeg struct {
	t        *testing.T
	reading  *os.File // FFmpeg reads the node's frames here
	writing  *os.File // and writes its own here
	sequence int8
	killed   atomic.Bool
	// dead closes when the node kills it.
	dead chan struct{}
}

func startFake(t *testing.T, config Config) (*Channel, *fakeFFmpeg) {
	t.Helper()
	channel, err := Open(config)
	if err != nil {
		t.Fatal(err)
	}
	ends := channel.ChildFiles()
	fake := &fakeFFmpeg{
		t: t, reading: duplicate(t, ends[0]), writing: duplicate(t, ends[1]), dead: make(chan struct{}),
	}
	channel.Start(t.Context(), fake.kill)
	return channel, fake
}

func duplicate(t *testing.T, file *os.File) *os.File {
	t.Helper()
	descriptor, err := syscall.Dup(int(file.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	return os.NewFile(uintptr(descriptor), file.Name())
}

func (fake *fakeFFmpeg) kill() {
	if fake.killed.CompareAndSwap(false, true) {
		fake.exit()
		close(fake.dead)
	}
}

// exit is FFmpeg ending: its ends close.
func (fake *fakeFFmpeg) exit() {
	_ = fake.reading.Close() //nolint:errcheck // the fake's own ends
	_ = fake.writing.Close() //nolint:errcheck // as above
}

func (fake *fakeFFmpeg) send(kind int8, body []byte) {
	fake.sendNumbered(kind, fake.sequence, body)
	fake.sequence = nextSequence(fake.sequence)
}

func (fake *fakeFFmpeg) sendNumbered(kind, sequence int8, body []byte) {
	_, _ = fake.writing.Write(frame(kind, sequence, body)) //nolint:errcheck // a killed fake's write fails
}

// receive reads one frame the node sent.
func (fake *fakeFFmpeg) receive() (header, []byte) {
	fake.t.Helper()
	raw := make([]byte, headerBytes)
	if _, err := io.ReadFull(fake.reading, raw); err != nil {
		fake.t.Fatalf("read the node's frame: %v", err)
	}
	got := decodeHeader(raw)
	body := make([]byte, got.length)
	if _, err := io.ReadFull(fake.reading, body); err != nil {
		fake.t.Fatalf("read the node's frame's body: %v", err)
	}
	return got, body
}

// open makes FFmpeg's side of the opening exchange, offering blocks of block bytes and outstanding
// READs, and returns the node's terms.
func (fake *fakeFFmpeg) open(block int32, outstanding int8) Terms {
	fake.t.Helper()
	fake.send(typeHello, helloBody(versionRange{lowest: 1, highest: 1}))
	if got, _ := fake.receive(); got.kind != typeHello || got.sequence != 0 {
		fake.t.Fatalf("the node answered HELLO with type %d, sequence %d", got.kind, got.sequence)
	}
	fake.send(typeCapability, put(put(put([]byte(nil), block), 4*block), outstanding))
	got, body := fake.receive()
	if got.kind != typeRequest || got.sequence != 1 || len(body) != requestBytes {
		fake.t.Fatalf("the node answered CAPABILITY with type %d, sequence %d, %d bytes", got.kind, got.sequence,
			len(body))
	}
	return Terms{
		Version: int16Of(body[0:2]), Run: int64Of(body[2:10]), FileSize: int64Of(body[10:18]),
		BlockSize: int32Of(body[18:22]), LongestRead: int32Of(body[22:26]), Outstanding: int8Of(body[26]),
	}
}

// readWhole reads a film of size bytes in blocks, two READs in flight, as FFmpeg reads straight
// through, and returns what the node sent.
func (fake *fakeFFmpeg) readWhole(size int64, block int32) []byte {
	fake.t.Helper()
	ask := func(offset int64) {
		fake.send(typeRead, readBody(offset, int32(min(int64(block), size-offset)))) //nolint:gosec // a block
	}
	var read bytes.Buffer
	ask(0)
	ask(int64(block))
	for offset := int64(0); offset < size; offset += int64(block) {
		got, body := fake.receive()
		if got.kind != typeData || int64(len(body)) != min(int64(block), size-offset) {
			fake.t.Fatalf("DATA for %d: type %d, %d bytes", offset, got.kind, len(body))
		}
		read.Write(body)
		if next := offset + 2*int64(block); next < size {
			ask(next)
		}
	}
	return read.Bytes()
}

func readBody(offset int64, length int32) []byte {
	return put(put([]byte(nil), offset), length)
}

// doneOK is a DONE that says OK for the test's run, 7.
func doneOK() []byte {
	return put(put([]byte(nil), int64(7)), int8(resultOK))
}

func testConfig(source Source) Config {
	return Config{Run: 7, Source: source, MostOutstanding: 2, DataPipeBytes: 1 << 20}
}

// A film is served whole, block by block, two READs in flight, and a run that exits 0 after DONE OK
// for its run succeeded (§7, §9.10).
func TestChannel_ServesAFilmWholeAndJudgesARunByItsDone(t *testing.T) {
	t.Parallel()
	source := newFilm(1000)
	channel, fake := startFake(t, testConfig(source))
	terms := fake.open(256, 4)
	if terms.Run != 7 || terms.FileSize != 1000 || terms.BlockSize != 256 || terms.Outstanding != 2 {
		t.Fatalf("the terms: %+v; want run 7, 1000 bytes, blocks of 256, 2 outstanding", terms)
	}
	read := fake.readWhole(1000, 256)
	fake.send(typeDone, doneOK())
	fake.exit()
	account := channel.Finish()
	if !bytes.Equal(read, source.bytes) {
		t.Error("the bytes served are not the film's")
	}
	if !account.Succeeded(0, 7) || account.ServedBytes != 1000 || account.Failure != nil {
		t.Errorf("the account: %+v; want a success that served 1000 bytes", account)
	}
	if account.Succeeded(1, 7) || account.Succeeded(0, 8) {
		t.Error("a run that exited 1, or echoed another run, succeeded")
	}
	if fake.killed.Load() {
		t.Error("a run that kept to the protocol was killed")
	}
}

// Every way FFmpeg can break the protocol ends its run: the node kills it and says why (§9.2).
func TestChannel_KillsARunThatBreaksTheProtocol(t *testing.T) {
	t.Parallel()
	for _, breach := range []struct {
		name string
		act  func(fake *fakeFFmpeg)
	}{
		{"a READ past the end", func(fake *fakeFFmpeg) { fake.send(typeRead, readBody(900, 200)) }},
		{"a READ at a negative offset", func(fake *fakeFFmpeg) { fake.send(typeRead, readBody(-1, 10)) }},
		{"a READ longer than the terms allow", func(fake *fakeFFmpeg) { fake.send(typeRead, readBody(0, 512)) }},
		{"a READ of nothing", func(fake *fakeFFmpeg) { fake.send(typeRead, readBody(0, 0)) }},
		{"a READ whose end overflows", func(fake *fakeFFmpeg) {
			fake.send(typeRead, readBody(1<<62, 256))
		}},
		{"the wrong sequence number", func(fake *fakeFFmpeg) { fake.sendNumbered(typeRead, 9, readBody(0, 256)) }},
		{"a type FFmpeg never sends", func(fake *fakeFFmpeg) { fake.send(typeData, make([]byte, 4)) }},
		{"a body longer than its type's", func(fake *fakeFFmpeg) { fake.send(typeRead, make([]byte, 13)) }},
		{"a DONE with an unknown result", func(fake *fakeFFmpeg) {
			body := doneOK()
			body[8] = 5
			fake.send(typeDone, body)
		}},
		{"a frame cut off inside its header", func(fake *fakeFFmpeg) {
			_, _ = fake.writing.Write([]byte{byte(typeRead), 2, 12}) //nolint:errcheck // the fake then exits
			fake.exit()
		}},
		// The writer stops counting a READ when it takes it (§7.4), so with two allowed, one taken
		// and two waiting keep to the rules, and a fourth does not.
		{"too many READs outstanding", func(fake *fakeFFmpeg) {
			for range 4 {
				fake.send(typeRead, readBody(0, 256))
			}
		}},
	} {
		t.Run(breach.name, func(t *testing.T) {
			t.Parallel()
			source := newFilm(1000)
			source.readGate = make(chan struct{}) // no READ is answered, so the count only grows
			channel, fake := startFake(t, testConfig(source))
			fake.open(256, 2)
			breach.act(fake)
			fake.waitForKill()
			account := channel.Finish()
			if !errors.Is(account.Failure, ErrViolation) {
				t.Errorf("the account's failure: %v; want a violation", account.Failure)
			}
		})
	}
}

// The opening exchange is checked too: a version range the node does not share, or a CAPABILITY
// outside its rules, ends the run before any terms are sent (§7.1, §7.2).
func TestChannel_KillsARunWhoseOpeningItCannotWorkWith(t *testing.T) {
	t.Parallel()
	for _, opening := range []struct {
		name string
		act  func(fake *fakeFFmpeg)
	}{
		{"no version shared", func(fake *fakeFFmpeg) {
			fake.send(typeHello, helloBody(versionRange{lowest: 2, highest: 3}))
		}},
		{"an invalid range", func(fake *fakeFFmpeg) {
			fake.send(typeHello, helloBody(versionRange{lowest: 1, highest: 0}))
		}},
		{"a CAPABILITY outside its rules", func(fake *fakeFFmpeg) {
			fake.send(typeHello, helloBody(versionRange{lowest: 1, highest: 1}))
			fake.receive()
			fake.send(typeCapability, make([]byte, capabilityBytes))
		}},
		{"a first frame that is not HELLO", func(fake *fakeFFmpeg) {
			fake.send(typeRead, readBody(0, 1))
		}},
	} {
		t.Run(opening.name, func(t *testing.T) {
			t.Parallel()
			channel, fake := startFake(t, testConfig(newFilm(100)))
			opening.act(fake)
			fake.waitForKill()
			if account := channel.Finish(); !errors.Is(account.Failure, ErrViolation) {
				t.Errorf("the account's failure: %v; want a violation", account.Failure)
			}
		})
	}
}

// A READ the film's storage cannot answer is never answered: the run is killed, and its account
// says the film could not be read, which is the disk's failure, not the film's (§7.4).
func TestChannel_KillsARunWhoseFilmCannotBeRead(t *testing.T) {
	t.Parallel()
	source := newFilm(1000)
	source.failAt = 256
	channel, fake := startFake(t, testConfig(source))
	fake.open(256, 2)
	fake.send(typeRead, readBody(0, 256))
	fake.receive()
	fake.send(typeRead, readBody(256, 256))
	fake.waitForKill()
	account := channel.Finish()
	if !errors.Is(account.Failure, ErrReadFailed) || account.Succeeded(0, 7) {
		t.Errorf("the account: %+v; want the film's read failed", account)
	}
}

// A run served more than its budget is killed before the read that would pass it (§9.6).
func TestChannel_KillsARunPastItsBudget(t *testing.T) {
	t.Parallel()
	config := testConfig(newFilm(1000))
	config.BudgetBytes = 600
	channel, fake := startFake(t, config)
	fake.open(256, 2)
	for _, offset := range []int64{0, 256} {
		fake.send(typeRead, readBody(offset, 256))
		fake.receive()
	}
	fake.send(typeRead, readBody(0, 256)) // 768 bytes would pass 600
	fake.waitForKill()
	account := channel.Finish()
	if !errors.Is(account.Failure, ErrOverBudget) || account.ServedBytes != 512 {
		t.Errorf("the account: %+v; want over budget after 512 bytes", account)
	}
}

// A held run is answered nothing until it is released, and its held time is no failure (§2).
func TestChannel_AHeldRunIsAnsweredOnlyOnceReleased(t *testing.T) {
	t.Parallel()
	source := newFilm(1000)
	channel, fake := startFake(t, testConfig(source))
	fake.open(256, 2)
	channel.Hold()
	fake.send(typeRead, readBody(0, 256))
	time.Sleep(50 * time.Millisecond) // gives a writer that ignored the hold time to read
	if got := source.reads.Load(); got != 0 {
		t.Fatalf("a held run's film was read %d times; want none", got)
	}
	channel.Release()
	if got, body := fake.receive(); got.kind != typeData || len(body) != 256 {
		t.Fatalf("after the release: type %d, %d bytes; want the READ's DATA", got.kind, len(body))
	}
	fake.send(typeDone, doneOK())
	fake.exit()
	if account := channel.Finish(); account.Failure != nil || fake.killed.Load() {
		t.Errorf("a held run's account: %+v, killed %v; want neither", account, fake.killed.Load())
	}
}

// An FFmpeg that ends before its HELLO sent no DONE and is not killed: its exit says it failed
// (§7.5). One that stops reading leaves the node's writer blocked, which Finish frees.
func TestChannel_FinishesWhateverStateFFmpegLeftItIn(t *testing.T) {
	t.Parallel()
	channel, fake := startFake(t, testConfig(newFilm(10)))
	fake.exit()
	if account := channel.Finish(); account.Done != nil || account.Failure != nil || account.Succeeded(0, 7) {
		t.Errorf("an FFmpeg that ended at once: %+v; want no DONE, no failure, no success", account)
	}
	config := testConfig(newFilm(4 << 20))
	config.DataPipeBytes = 0 // the default 64 KiB, which one block overfills
	channel, fake = startFake(t, config)
	fake.open(1<<20, 2)
	fake.send(typeRead, readBody(0, 1<<20))
	fake.send(typeDone, doneOK())
	time.Sleep(50 * time.Millisecond) // lets the writer fill the pipe and block
	_ = fake.writing.Close()          //nolint:errcheck // FFmpeg exits without reading its DATA
	finished := make(chan Account, 1)
	go func() { finished <- channel.Finish() }()
	select {
	case account := <-finished:
		if account.Done == nil || !account.Done.IsOK {
			t.Errorf("the account: %+v; want the DONE FFmpeg sent before it exited", account)
		}
	case <-time.After(2 * drainWait):
		t.Fatal("Finish did not free a writer blocked on FFmpeg's full pipe")
	}
	_ = fake.reading.Close() //nolint:errcheck // the fake's last end
}

// Frames after DONE are counted and change nothing: FFmpeg is exiting (§7.5).
func TestChannel_FramesAfterDoneChangeNothing(t *testing.T) {
	t.Parallel()
	channel, fake := startFake(t, testConfig(newFilm(100)))
	fake.open(64, 2)
	fake.send(typeDone, doneOK())
	fake.send(typeRead, readBody(0, 64))
	fake.exit()
	account := channel.Finish()
	if account.LateFrames != 1 || account.Failure != nil || !account.Succeeded(0, 7) {
		t.Errorf("the account: %+v; want one late frame and a success", account)
	}
}

// A DONE that failed carries its kind, code and message, and the message is safe to log (§9.11).
func TestDecodeDone_ReadsAFailureAndMakesItsMessageSafeToLog(t *testing.T) {
	t.Parallel()
	message := []byte("Input/output error \x1b[31m\u202e\xff")
	body := make([]byte, doneErrFixed, doneErrFixed+len(message))
	binary.LittleEndian.PutUint64(body[0:8], 7)
	body[8], body[9] = resultErr, byte(KindChannel)
	binary.LittleEndian.PutUint32(body[10:14], 5)
	binary.LittleEndian.PutUint16(body[14:16], uint16(len(message))) //nolint:gosec // a short message
	body = append(body, message...)
	done, err := decodeDone(body)
	if err != nil {
		t.Fatal(err)
	}
	want := `Input/output error \u001b[31m\u202e` + "\ufffd"
	if done.IsOK || done.Kind != KindChannel || done.Code != 5 || done.Message != want {
		t.Errorf("the DONE: %+v; want channel, code 5, message %q", done, want)
	}
}

func TestCacheBlocks_IsOneAndAHalfTimesTheStreamsPlusTwo(t *testing.T) {
	t.Parallel()
	for streams, want := range map[int]int{0: 2, 1: 4, 2: 5, 10: 17} {
		if got := CacheBlocks(streams); got != want {
			t.Errorf("CacheBlocks(%d) = %d; want %d", streams, got, want)
		}
	}
}

// waitForKill waits for the node to kill the run, failing after a while.
func (fake *fakeFFmpeg) waitForKill() {
	fake.t.Helper()
	select {
	case <-fake.dead:
	case <-time.After(5 * time.Second):
		fake.t.Fatal("the node did not kill the run")
	}
}
