// SPDX-License-Identifier: AGPL-3.0-or-later

package fdproxy

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"os"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/testutil"
)

// film is a Source over bytes in memory, which can fail its reads.
type film struct {
	bytes    []byte
	failAt   int64 // a read at this offset fails; -1 for none
	reads    atomic.Int32
	readGate chan struct{} // when set, each read waits for a value on it
	// entered, when set, is told each read that starts, and stopped each gated read its context
	// ended.
	entered, stopped chan struct{}
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
	if source.entered != nil {
		source.entered <- struct{}{}
	}
	if source.readGate != nil {
		select {
		case <-source.readGate:
		case <-ctx.Done():
			if source.stopped != nil {
				source.stopped <- struct{}{}
			}
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

// saidDoneOK is whether the account carries a DONE that said OK for the test's run, 7.
func saidDoneOK(account Account) bool {
	return account.Done != nil && account.Done.IsOK && account.Done.Run == 7
}

func testConfig(source Source) Config {
	return Config{Run: 7, Source: source, MostOutstanding: 2, DataPipeBytes: 1 << 20}
}

// A film is served whole, block by block, two READs in flight, and the account carries FFmpeg's DONE
// OK for its run, by which execx judges it (Messages, Judging a Run).
func TestChannel_ServesAFilmWholeAndAccountsForItsDone(t *testing.T) {
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
	if !saidDoneOK(account) || account.ServedBytes != 1000 || account.Failure != nil {
		t.Errorf("the account: %+v; want DONE OK for run 7, no failure, 1000 bytes served", account)
	}
	if fake.killed.Load() {
		t.Error("a run that kept to the protocol was killed")
	}
}

// sizedFilm is a film that says it is of size bytes, and is never read.
type sizedFilm int64

func (size sizedFilm) Size() int64 { return int64(size) }

func (sizedFilm) ReadAt(context.Context, []byte, int64) error { return errors.New("never read") }

// The node never offers FFmpeg a film past the protocol's largest size, 2^62 bytes, so no sum of the
// size and a block can pass the largest int64 on either side (Checks Without Overflow); a film of that size is served.
func TestOpen_RefusesAFilmPastTheLargestSizeTheProtocolServes(t *testing.T) {
	t.Parallel()
	for _, example := range []struct {
		size     int64
		isServed bool
	}{{1 << 62, true}, {1<<62 + 1, false}, {math.MaxInt64, false}, {-1, false}} {
		channel, err := Open(testConfig(sizedFilm(example.size)))
		if channel != nil {
			channel.Abandon()
		}
		if isServed := err == nil; isServed != example.isServed || (!isServed && !errors.Is(err, ErrFilmTooLarge)) {
			t.Errorf("a film of %d bytes: %v; want served %v", example.size, err, example.isServed)
		}
	}
}

// However large a block FFmpeg offers, the node serves in blocks no larger than its own largest, so an
// FFmpeg taken over by its film cannot make the node hold gigabytes for it (REQUEST).
func TestChannel_ServesInBlocksNoLargerThanItsOwnLargestWhateverFFmpegOffers(t *testing.T) {
	t.Parallel()
	channel, fake := startFake(t, testConfig(newFilm(1000)))
	fake.send(typeHello, helloBody(versionRange{lowest: 1, highest: 1}))
	fake.receive()
	fake.send(typeCapability, put(put(put([]byte(nil), int32(1<<30)), int32(1<<30)), int8(2)))
	got, body := fake.receive()
	if got.kind != typeRequest || len(body) != requestBytes {
		t.Fatalf("the node answered CAPABILITY with type %d, %d bytes", got.kind, len(body))
	}
	if block, longest := int32Of(body[18:22]), int32Of(body[22:26]); block != largestBlockBytes ||
		longest != largestBlockBytes {
		t.Errorf("offered blocks of 1 GiB, the node asked for blocks of %d and reads of %d; want %d",
			block, longest, largestBlockBytes)
	}
	fake.exit()
	channel.Finish()
}

// Every way FFmpeg can break the protocol ends its run: the node kills it and says why (Serving).
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
		{"a frame whose body never came", func(fake *fakeFFmpeg) {
			_, _ = fake.writing.Write([]byte{byte(typeRead), 2, 12, 0, 0, 0}) //nolint:errcheck // the fake then exits
			fake.exit()
		}},
		// The writer stops counting a READ when it takes it (READ), so with two allowed, one taken
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

// A run that breaks the protocol while the node reads the film for an earlier READ is sent nothing
// more: the read's DATA, once the film answers, is not written (Serving). The fake here keeps its
// ends when killed, so the test reads all the node sent.
func TestChannel_SendsNothingOnceTheRunHasFailed(t *testing.T) {
	t.Parallel()
	source := newFilm(1000)
	source.readGate, source.entered = make(chan struct{}), make(chan struct{}, 1)
	channel, err := Open(testConfig(source))
	if err != nil {
		t.Fatal(err)
	}
	ends := channel.ChildFiles()
	fake := &fakeFFmpeg{t: t, reading: duplicate(t, ends[0]), writing: duplicate(t, ends[1])}
	killed := make(chan struct{})
	channel.Start(t.Context(), func() { close(killed) })
	fake.open(256, 2)
	fake.send(typeRead, readBody(0, 256))
	testutil.Receive(t, source.entered, "the READ to be taken")
	fake.send(typeRead, readBody(900, 200))
	testutil.Receive(t, killed, "the kill for the READ past the end")
	source.readGate <- struct{}{}
	testutil.Receive(t, channel.writerDone, "the writer to end")
	_ = fake.writing.Close() //nolint:errcheck // FFmpeg exits
	account := channel.Finish()
	sent, err := io.ReadAll(fake.reading)
	_ = fake.reading.Close() //nolint:errcheck // the fake's last end
	if err != nil || len(sent) != 0 {
		t.Errorf("after the breach the node sent %d bytes (%v); want none", len(sent), err)
	}
	if !errors.Is(account.Failure, ErrViolation) {
		t.Errorf("the account's failure: %v; want a violation", account.Failure)
	}
}

// The opening exchange is checked too: a version range the node does not share, or a CAPABILITY
// outside its rules, ends the run before any terms are sent (HELLO, CAPABILITY).
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
// says the film could not be read, which is the disk's failure, not the film's (READ).
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
	if !errors.Is(account.Failure, ErrReadFailed) {
		t.Errorf("the account: %+v; want the film's read failed", account)
	}
}

// A run served more than its budget is killed before the read that would pass it (Budget).
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

// A held run is answered nothing until it is released, and its held time is no failure (Pausing).
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
// (DONE). One that stops reading leaves the node's writer blocked, which Finish frees.
func TestChannel_FinishesWhateverStateFFmpegLeftItIn(t *testing.T) {
	t.Parallel()
	channel, fake := startFake(t, testConfig(newFilm(10)))
	fake.exit()
	if account := channel.Finish(); account.Done != nil || account.Failure != nil {
		t.Errorf("an FFmpeg that ended at once: %+v; want no DONE and no failure", account)
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

// FFmpeg has exited by the time Finish begins, and a process reaped is never signalled (Stopping),
// so nothing Finish meets kills the run: a storage read it stops, the block FFmpeg prefetched before
// it said DONE OK and exited 0, as ffprobe does, fails nothing; and a breach read from what FFmpeg
// left in the pipe is the run's failure, but kills nothing (End of a Run, Judging a Run).
func TestChannel_KillsNothingOnceFinishHasBegun(t *testing.T) {
	t.Parallel()
	for _, example := range []struct {
		name string
		// last is what FFmpeg sends, once Finish has stopped the prefetch's read, before it exits.
		last []byte
		want error
	}{
		{"DONE OK, its prefetch still being read", frame(typeDone, 4, doneOK()), nil},
		{"a READ past the end", frame(typeRead, 4, readBody(900, 200)), ErrViolation},
	} {
		t.Run(example.name, func(t *testing.T) {
			t.Parallel()
			source := newFilm(1000)
			source.readGate, source.entered, source.stopped = make(chan struct{}), make(chan struct{}, 2),
				make(chan struct{}, 1)
			channel, fake := startFake(t, testConfig(source))
			fake.open(256, 2)
			fake.send(typeRead, readBody(0, 256))
			source.readGate <- struct{}{}
			fake.receive()
			fake.send(typeRead, readBody(256, 256)) // the prefetch
			testutil.Receive(t, source.entered, "the first read")
			testutil.Receive(t, source.entered, "the prefetch's read")
			finished := make(chan Account, 1)
			go func() { finished <- channel.Finish() }()
			testutil.Receive(t, source.stopped, "Finish to stop the prefetch's read")
			_, _ = fake.writing.Write(example.last) //nolint:errcheck // the fake then exits
			fake.exit()
			account := testutil.Receive(t, finished, "Finish")
			if !errors.Is(account.Failure, example.want) || (example.want == nil && account.Failure != nil) {
				t.Errorf("the account's failure: %v; want %v", account.Failure, example.want)
			}
			if fake.killed.Load() {
				t.Error("the channel killed a run Finish had begun on: its process was reaped")
			}
		})
	}
}

// Frames after DONE are counted and change nothing: FFmpeg is exiting (DONE).
func TestChannel_FramesAfterDoneChangeNothing(t *testing.T) {
	t.Parallel()
	channel, fake := startFake(t, testConfig(newFilm(100)))
	fake.open(64, 2)
	fake.send(typeDone, doneOK())
	fake.send(typeRead, readBody(0, 64))
	fake.exit()
	account := channel.Finish()
	if account.LateFrames != 1 || account.Failure != nil || !saidDoneOK(account) {
		t.Errorf("the account: %+v; want one late frame, no failure and the DONE OK", account)
	}
}

// A DONE that failed carries its kind, code and message, and the message is safe to log (Logging Text).
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

// A DONE is checked like every frame: no negative field, OK or not, a kind the version defines, and a
// failure's body exactly its fields' sum (Integers, Lengths, DONE). A run of 0, which FFmpeg sends
// when it refused terms with no run, is no breach.
func TestDecodeDone_RefusesADoneThatBreaksItsRules(t *testing.T) {
	t.Parallel()
	failed := func(run int64, kind int8, code int32, messageLength int16, message string) []byte {
		body := put(put(put(put(put([]byte(nil), run), int8(resultErr)), kind), code), messageLength)
		return append(body, message...)
	}
	for _, example := range []struct {
		name     string
		body     []byte
		isBreach bool
	}{
		{"OK for run 0", put(put([]byte(nil), int64(0)), int8(resultOK)), false},
		{"ERR for run 0", failed(0, int8(KindTerms), 0, 0, ""), false},
		{"ERR with a message", failed(7, int8(KindChannel), 5, 3, "EIO"), false},
		{"OK for a negative run", put(put([]byte(nil), int64(-1)), int8(resultOK)), true},
		{"ERR for a negative run", failed(-1, int8(KindChannel), 5, 0, ""), true},
		{"a kind below the first", failed(7, 0, 5, 0, ""), true},
		{"a kind past the last", failed(7, int8(KindInterrupted)+1, 5, 0, ""), true},
		{"a negative code", failed(7, int8(KindChannel), -5, 0, ""), true},
		{"a negative message length", failed(7, int8(KindChannel), 5, -1, ""), true},
		{"a message longer than the longest", failed(7, int8(KindChannel), 5, longestMessage+1,
			string(make([]byte, longestMessage+1))), true},
		{"a message shorter than its length", failed(7, int8(KindChannel), 5, 4, "EIO"), true},
		{"a message longer than its length", failed(7, int8(KindChannel), 5, 2, "EIO"), true},
		{"OK with more after it", append(put(put([]byte(nil), int64(7)), int8(resultOK)), 0), true},
	} {
		_, err := decodeDone(example.body)
		if isBreach := err != nil; isBreach != example.isBreach || (isBreach && !errors.Is(err, ErrViolation)) {
			t.Errorf("%s: %v; want a breach %v", example.name, err, example.isBreach)
		}
	}
}

// The terms allow no more READs outstanding than either side offers: FFmpeg's CAPABILITY, or the
// node's own most (REQUEST).
func TestChannel_TermsAllowTheFewerREADsOutstandingOfWhatEitherSideOffers(t *testing.T) {
	t.Parallel()
	for _, example := range []struct{ offered, want int8 }{{1, 1}, {2, 2}, {4, 2}} {
		channel, fake := startFake(t, testConfig(newFilm(100)))
		if terms := fake.open(64, example.offered); terms.Outstanding != example.want {
			t.Errorf("FFmpeg offered %d READs outstanding, the node at most 2: terms of %d; want %d",
				example.offered, terms.Outstanding, example.want)
		}
		fake.exit()
		channel.Finish()
	}
}

// With two READs outstanding agreed, one READ taken to answer and two waiting keep to the rules,
// and a third waiting does not: the node stops counting a READ when it takes it (READ).
func TestChannel_HoldsFFmpegToExactlyTheREADsOutstandingAgreed(t *testing.T) {
	t.Parallel()
	source := newFilm(1000)
	source.readGate, source.entered = make(chan struct{}), make(chan struct{}, 1)
	channel, fake := startFake(t, testConfig(source))
	fake.open(256, 2)
	fake.send(typeRead, readBody(0, 256))
	testutil.Receive(t, source.entered, "the first READ to be taken")
	fake.send(typeRead, readBody(256, 256))
	fake.send(typeRead, readBody(512, 256))
	waitUntil(t, channel, "two READs to wait, or a failure", func() bool {
		return len(channel.waiting) == 2 || channel.account.Failure != nil
	})
	channel.mutex.Lock()
	failure := channel.account.Failure
	channel.mutex.Unlock()
	if failure != nil {
		t.Fatalf("two READs waiting, one taken, of two agreed: %v; want no breach", failure)
	}
	fake.send(typeRead, readBody(768, 232))
	fake.waitForKill()
	if account := channel.Finish(); !errors.Is(account.Failure, ErrViolation) {
		t.Errorf("a third READ waiting: %v; want a breach", account.Failure)
	}
}

// On DONE the node stops serving the channel: it drops every READ still waiting, stops the film's
// read under way and sends nothing more, and a DONE that comes before READs it has not answered is no
// breach (When DONE Is Sent).
func TestChannel_DropsTheREADsStillWaitingOnDone(t *testing.T) {
	t.Parallel()
	source := newFilm(1000)
	source.readGate, source.entered = make(chan struct{}), make(chan struct{}, 2)
	source.stopped = make(chan struct{}, 1)
	channel, fake := startFake(t, testConfig(source))
	fake.open(256, 2)
	fake.send(typeRead, readBody(0, 256))
	testutil.Receive(t, source.entered, "the first READ to be taken")
	fake.send(typeRead, readBody(256, 256))
	fake.send(typeDone, doneOK())
	waitUntil(t, channel, "the DONE", func() bool { return channel.account.Done != nil })
	channel.mutex.Lock()
	waiting := len(channel.waiting)
	channel.mutex.Unlock()
	if waiting != 0 {
		t.Errorf("%d READs still waiting after DONE; want none", waiting)
	}
	testutil.Receive(t, source.stopped, "the film's read under way to be stopped")
	testutil.Receive(t, channel.writerDone, "the writer to end")
	_ = fake.writing.Close() //nolint:errcheck // FFmpeg exits
	account := channel.Finish()
	sent, err := io.ReadAll(fake.reading)
	_ = fake.reading.Close() //nolint:errcheck // the fake's last end
	if err != nil || len(sent) != 0 {
		t.Errorf("after DONE the node sent %d bytes (%v); want none", len(sent), err)
	}
	if account.Failure != nil || fake.killed.Load() || source.reads.Load() != 1 {
		t.Errorf("a DONE before a READ was answered: failure %v, killed %v, %d film reads; want none, no, 1",
			account.Failure, fake.killed.Load(), source.reads.Load())
	}
}

// A frame after DONE that breaks the framing changes nothing either: FFmpeg has given its account,
// and the frame is counted late, never the run's failure (DONE).
func TestChannel_AMalformedFrameAfterDoneChangesNothing(t *testing.T) {
	t.Parallel()
	channel, fake := startFake(t, testConfig(newFilm(100)))
	fake.open(64, 2)
	fake.send(typeDone, doneOK())
	fake.sendNumbered(typeRead, 9, readBody(0, 64)) // out of sequence
	fake.exit()
	account := channel.Finish()
	if account.LateFrames != 1 || account.Failure != nil || fake.killed.Load() || !saidDoneOK(account) {
		t.Errorf("the account: %+v, killed %v; want one late frame, no failure, no kill and the DONE OK",
			account, fake.killed.Load())
	}
}

// waitUntil waits until condition, which reads the channel under its mutex, holds.
func waitUntil(t *testing.T, channel *Channel, waitingFor string, condition func() bool) {
	t.Helper()
	held := make(chan struct{})
	go func() {
		defer close(held)
		for {
			channel.mutex.Lock()
			isMet := condition()
			channel.mutex.Unlock()
			if isMet {
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()
	testutil.Receive(t, held, waitingFor)
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
