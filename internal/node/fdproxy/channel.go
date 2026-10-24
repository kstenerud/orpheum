// SPDX-License-Identifier: AGPL-3.0-or-later

package fdproxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"sync"
	"time"
)

// Source is the film a channel serves: its size, and its bytes by offset, read through the node's
// turn on its disk, with the disk's deadline. A read fills buffer whole or fails.
type Source interface {
	Size() int64
	ReadAt(ctx context.Context, buffer []byte, offset int64) error
}

// Config is one channel's: what it serves and on what terms.
type Config struct {
	// Run is the node's identifier for the run, at least 1, echoed in DONE.
	Run    int64
	Source Source
	// MostOutstanding is the most READs the node lets FFmpeg have unanswered, 1 to 16; FFmpeg's
	// CAPABILITY may lower it.
	MostOutstanding int8
	// BudgetBytes is the most the run may be served; a run past it is killed (Budget). Zero is no budget.
	BudgetBytes int64
	// DataPipeBytes is the size asked for the pipe that carries the bytes (Pipe Sizes); zero leaves it.
	DataPipeBytes int
}

// The channel's errors: why the node killed a run.
var (
	// ErrReadFailed is the film's storage failing to answer a READ: a read error, or its deadline.
	ErrReadFailed = errors.New("the film could not be read for FFmpeg")
	// ErrOverBudget is a run served more than its kind should need (Budget).
	ErrOverBudget = errors.New("FFmpeg asked for more of the film than its run should need")
)

// largestFileBytes is the largest file a REQUEST may give (Checks Without Overflow): headroom, so the
// size plus a block, a READ or a seek from the end never passes the largest int64 on either side.
const largestFileBytes = 1 << 62

// ErrFilmTooLarge is a film larger than the protocol may serve (Checks Without Overflow), which no file is.
var ErrFilmTooLarge = errors.New("the film is larger than fdproxy serves")

// largestBlockBytes is the largest block the node serves in, whatever FFmpeg offers: the largest
// DATA body the patched FFmpeg accepts.
const largestBlockBytes = 4 << 20

// drainWait is how long Finish waits for FFmpeg's last frames once it has exited: they are in the
// pipe already, so this bounds only a pipe some other process still holds.
const drainWait = 5 * time.Second

// Channel serves one input film to one FFmpeg run over two pipes (fdproxy's specification): FFmpeg's READs come
// on one, and the parent's answers go on the other. The node judges every frame FFmpeg sends, and
// kills the run on any violation, any read the film's storage fails, and a budget passed.
type Channel struct {
	config Config
	// requests is the node's end of the pipe FFmpeg writes; data, of the pipe FFmpeg reads. Their
	// other ends are FFmpeg's, closed here once it has started.
	requests, data            *os.File
	childReading, childWriter *os.File

	// kill ends the run; it is called under mutex, and never once isClosing is set (failLocked).
	kill     func()
	killOnce sync.Once
	// stopWriting ends a storage read the writer waits on, once FFmpeg has exited.
	stopWriting context.CancelFunc
	// pipeSizeRefused is why the data pipe kept its default size, nil when it took the size asked.
	pipeSizeRefused error

	mutex sync.Mutex
	// waiting are the READs taken and not yet answered, oldest first. Guarded by mutex.
	waiting []readRequest
	// changed is signalled when waiting grows, the channel closes, or a hold ends. Guarded by mutex.
	changed *sync.Cond
	// isClosing ends the writer once FFmpeg has exited. Guarded by mutex.
	isClosing bool
	// isHeld holds READs back: a paused run reads nothing (Pausing). Guarded by mutex.
	isHeld bool
	// terms are the REQUEST sent, once it is. Guarded by mutex.
	terms Terms
	// account is what the channel saw. Guarded by mutex.
	account Account

	readerDone, writerDone chan struct{}
}

// Account is what one channel saw of its run: FFmpeg's DONE, nil when it sent none; the node's own
// reason for killing it, nil when it did not; the bytes served; and frames after DONE.
type Account struct {
	Done        *Done
	Failure     error
	ServedBytes int64
	LateFrames  int
}

// Open makes a channel's pipes. Hand ChildFiles to FFmpeg, contiguous and in order, and call Start
// once it has started; Finish once it has exited.
func Open(config Config) (*Channel, error) {
	if config.Run < 1 || config.Source == nil || config.MostOutstanding < 1 ||
		config.MostOutstanding > mostOutstanding || config.BudgetBytes < 0 {
		return nil, fmt.Errorf("a channel's config is outside its rules: run %d, outstanding %d, budget %d",
			config.Run, config.MostOutstanding, config.BudgetBytes)
	}
	if size := config.Source.Size(); size < 0 || size > largestFileBytes {
		return nil, fmt.Errorf("%w: %d bytes", ErrFilmTooLarge, size)
	}
	childReading, data, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("make the data pipe: %w", err)
	}
	requests, childWriter, err := os.Pipe()
	if err != nil {
		return nil, errors.Join(fmt.Errorf("make the request pipe: %w", err), childReading.Close(), data.Close())
	}
	channel := &Channel{
		config: config, requests: requests, data: data, childReading: childReading, childWriter: childWriter,
		readerDone: make(chan struct{}), writerDone: make(chan struct{}),
	}
	channel.changed = sync.NewCond(&channel.mutex)
	if config.DataPipeBytes > 0 {
		// The protocol works at any size, only slower: past the user's pipe memory, 8 KiB (Pipe Sizes).
		channel.pipeSizeRefused = sizePipe(data, config.DataPipeBytes)
	}
	return channel, nil
}

// PipeSizeRefused is why the data pipe kept its default size, for the node to log once; nil when
// it took the size asked.
func (channel *Channel) PipeSizeRefused() error { return channel.pipeSizeRefused }

// ChildFiles are FFmpeg's ends: the pipe it reads, then the one it writes. In a command's
// ExtraFiles from index first, they are descriptors 3+first and 4+first.
func (channel *Channel) ChildFiles() []*os.File {
	return []*os.File{channel.childReading, channel.childWriter}
}

// ProtocolName is what FFmpeg calls the protocol.
const ProtocolName = "fdproxy"

// InputArguments name the input for FFmpeg: the protocol alone allowed, its descriptors, and the
// blocks it keeps (Naming the Input, Reading in Blocks).
func InputArguments(readFD, writeFD, cacheBlocks int) []string {
	return []string{
		"-protocol_whitelist", ProtocolName,
		"-read_fd", strconv.Itoa(readFD), "-write_fd", strconv.Itoa(writeFD),
		"-cache_blocks", strconv.Itoa(cacheBlocks),
		"-i", ProtocolName + ":",
	}
}

// Start serves the channel to a started FFmpeg, whose run kill ends: it closes the node's copies of
// FFmpeg's ends, so the request pipe ends when FFmpeg does (Descriptors).
func (channel *Channel) Start(ctx context.Context, kill func()) {
	channel.kill = kill
	_ = channel.childReading.Close() //nolint:errcheck // FFmpeg holds its own copy
	_ = channel.childWriter.Close()  //nolint:errcheck // as above
	ctx, channel.stopWriting = context.WithCancel(ctx)
	go channel.read(ctx)
}

// Abandon closes every end of a channel whose FFmpeg never started.
func (channel *Channel) Abandon() {
	for _, file := range []*os.File{channel.childReading, channel.childWriter, channel.requests, channel.data} {
		_ = file.Close() //nolint:errcheck // nothing was served
	}
}

// Hold stops serving READs, for a paused run; Release serves them again (Pausing).
func (channel *Channel) Hold() { channel.setHeld(true) }

// Release ends a Hold.
func (channel *Channel) Release() { channel.setHeld(false) }

func (channel *Channel) setHeld(isHeld bool) {
	channel.mutex.Lock()
	channel.isHeld = isHeld
	channel.changed.Broadcast()
	channel.mutex.Unlock()
}

// Finish ends a channel whose FFmpeg has exited, and says what it saw: it closes the data pipe,
// which frees a writer FFmpeg left blocked, reads what FFmpeg wrote before it exited, and closes
// the request pipe (End of a Run).
func (channel *Channel) Finish() Account {
	channel.mutex.Lock()
	channel.isClosing = true
	channel.waiting = nil
	channel.changed.Broadcast()
	channel.mutex.Unlock()
	channel.stopWriting()
	_ = channel.data.Close() //nolint:errcheck // frees a blocked write; FFmpeg is gone
	select {
	case <-channel.readerDone:
	case <-time.After(drainWait):
	}
	_ = channel.requests.Close() //nolint:errcheck // read to its end, or given up on
	<-channel.readerDone
	// A storage read that waits in the kernel keeps its buffer until it returns, and is not waited
	// for: a dead share holds one goroutine, never the run (Read Deadline).
	select {
	case <-channel.writerDone:
	case <-time.After(drainWait):
	}
	channel.mutex.Lock()
	defer channel.mutex.Unlock()
	return channel.account
}

// fail records the node's reason to end the run, the first one only, and kills it (failLocked).
func (channel *Channel) fail(reason error) {
	channel.mutex.Lock()
	defer channel.mutex.Unlock()
	channel.failLocked(reason)
}

// failLocked records the node's reason to end the run, the first one only, and kills it unless
// Finish has begun: FFmpeg has exited by then, and a process reaped is never signalled (Stopping).
// A breach read from what FFmpeg left in the pipe is still the run's failure (Judging a Run). The
// kill is under the mutex, so none is sent once Finish has set isClosing. Requires mutex.
func (channel *Channel) failLocked(reason error) {
	if channel.account.Failure == nil {
		channel.account.Failure = reason
	}
	channel.waiting = nil
	channel.changed.Broadcast()
	if !channel.isClosing {
		channel.killOnce.Do(channel.kill)
	}
}

// read reads FFmpeg's frames: the opening exchange, then its READs and its DONE, until the pipe
// ends. It starts the writer once the terms are sent, and ends it as it ends.
func (channel *Channel) read(ctx context.Context) {
	defer close(channel.readerDone)
	isWriting := false
	defer func() {
		if !isWriting {
			close(channel.writerDone)
		}
	}()
	terms, err := channel.open()
	if err != nil {
		if !errors.Is(err, io.EOF) {
			channel.fail(err)
		}
		return
	}
	isWriting = true
	go channel.write(ctx, terms)
	expected := int8(2)
	for {
		got, body, err := readFrame(channel.requests, expected)
		if err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, os.ErrClosed) {
				channel.fail(err)
			}
			return
		}
		expected = nextSequence(expected)
		if err := channel.take(got, body, terms); err != nil {
			channel.fail(err)
			return
		}
	}
}

// open makes the opening exchange (HELLO to REQUEST): FFmpeg's HELLO, the node's, FFmpeg's CAPABILITY,
// and the node's terms. FFmpeg ending first is io.EOF: no terms, and so no DONE.
func (channel *Channel) open() (Terms, error) {
	got, body, err := readFrame(channel.requests, 0)
	if err != nil {
		return Terms{}, err
	}
	if got.kind != typeHello {
		return Terms{}, fmt.Errorf("%w: the first frame was type %d", ErrViolation, got.kind)
	}
	ours := versionRange{lowest: lowestVersion, highest: highestVersion}
	version, err := agreedVersion(decodeHello(body), ours)
	if err != nil {
		return Terms{}, err
	}
	if writeErr := writeAll(channel.data, frame(typeHello, 0, helloBody(ours))); writeErr != nil {
		return Terms{}, writeErr
	}
	got, body, err = readFrame(channel.requests, 1)
	if err != nil {
		return Terms{}, err
	}
	if got.kind != typeCapability {
		return Terms{}, fmt.Errorf("%w: type %d where CAPABILITY was expected", ErrViolation, got.kind)
	}
	offered, err := decodeCapability(body)
	if err != nil {
		return Terms{}, err
	}
	// FFmpeg's preferred block, but never more than the node's largest: it holds a block's buffer for
	// each run, and FFmpeg may have been taken over by its film (REQUEST).
	block := min(offered.preferredBlock, largestBlockBytes)
	terms := Terms{
		Version: version, Run: channel.config.Run, FileSize: channel.config.Source.Size(),
		BlockSize: block, LongestRead: block, Outstanding: min(offered.outstanding, channel.config.MostOutstanding),
	}
	channel.mutex.Lock()
	channel.terms = terms
	channel.mutex.Unlock()
	return terms, writeAll(channel.data, frame(typeRequest, 1, requestBody(terms)))
}

// take takes one frame after the opening exchange: a READ to answer, or FFmpeg's DONE.
func (channel *Channel) take(got header, body []byte, terms Terms) error {
	channel.mutex.Lock()
	defer channel.mutex.Unlock()
	if channel.account.Done != nil {
		channel.account.LateFrames++ // FFmpeg is exiting; the frame changes nothing (DONE)
		return nil
	}
	switch got.kind {
	case typeDone:
		done, err := decodeDone(body)
		if err != nil {
			return err
		}
		channel.account.Done = &done
		channel.waiting = nil
		return nil
	case typeRead:
		read, err := decodeRead(got.sequence, body, terms)
		if err != nil {
			return err
		}
		// The writer stops counting a READ when it takes it to answer (READ).
		if len(channel.waiting) >= int(terms.Outstanding) {
			return fmt.Errorf("%w: %d READs unanswered, %d allowed", ErrViolation, len(channel.waiting)+1,
				terms.Outstanding)
		}
		channel.waiting = append(channel.waiting, read)
		channel.changed.Broadcast()
		return nil
	}
	return fmt.Errorf("%w: type %d after the opening exchange", ErrViolation, got.kind)
}

// write answers the READs in order: it reads the film into its own buffer, then writes DATA. Its
// buffers are the channel's own, so no write waits on another channel (Serving).
func (channel *Channel) write(ctx context.Context, terms Terms) {
	defer close(channel.writerDone)
	buffer := make([]byte, headerBytes+int(terms.LongestRead))
	for {
		read, isTaken := channel.next()
		if !isTaken {
			return
		}
		if !channel.spend(read.length) {
			return
		}
		body := buffer[headerBytes : headerBytes+int(read.length)]
		if err := channel.config.Source.ReadAt(ctx, body, read.offset); err != nil {
			channel.failRead(fmt.Errorf("%w: %d bytes at %d: %w", ErrReadFailed, read.length, read.offset, err))
			return
		}
		if !channel.isServing() {
			return // the run failed, or Finish began, while the film was read: nothing more is sent (Serving)
		}
		put(put(put(buffer[:0], typeData), read.sequence), read.length)
		if err := writeAll(channel.data, buffer[:headerBytes+int(read.length)]); err != nil {
			return // FFmpeg has gone, or Finish closed the pipe
		}
	}
}

// next takes the oldest READ to answer, waiting while there is none or the run is held, and says
// false once the channel is closing or failed.
func (channel *Channel) next() (readRequest, bool) {
	channel.mutex.Lock()
	defer channel.mutex.Unlock()
	for !channel.isClosing && channel.account.Failure == nil && (len(channel.waiting) == 0 || channel.isHeld) {
		channel.changed.Wait()
	}
	if channel.isClosing || channel.account.Failure != nil {
		return readRequest{}, false
	}
	read := channel.waiting[0]
	channel.waiting = channel.waiting[1:]
	return read, true
}

// isServing is whether the writer may still send DATA: not once the run has failed, after which the
// node sends nothing more (Serving), nor once Finish has begun.
func (channel *Channel) isServing() bool {
	channel.mutex.Lock()
	defer channel.mutex.Unlock()
	return !channel.isClosing && channel.account.Failure == nil
}

// failRead fails the run for a READ its film's storage did not answer, unless Finish has begun:
// Finish stops a storage read under way, and a READ whose FFmpeg has exited fails nothing (End of
// a Run): as ffprobe exits 0 after DONE OK, its prefetched block still being read.
func (channel *Channel) failRead(reason error) {
	channel.mutex.Lock()
	defer channel.mutex.Unlock()
	if !channel.isClosing {
		channel.failLocked(reason)
	}
}

// spend counts length bytes against the budget, before they are read, without overflowing:
// served is compared with what is left of the budget, never added to (Checks Without Overflow). It
// says whether to serve them: not past the budget, which fails the run, and not once Finish has
// begun, FFmpeg having exited.
func (channel *Channel) spend(length int32) bool {
	channel.mutex.Lock()
	defer channel.mutex.Unlock()
	if channel.isClosing {
		return false
	}
	budget := channel.config.BudgetBytes
	if budget > 0 && int64(length) > budget-channel.account.ServedBytes {
		channel.failLocked(fmt.Errorf("%w: %d bytes served, %d more asked, budget %d", ErrOverBudget,
			channel.account.ServedBytes, length, budget))
		return false
	}
	channel.account.ServedBytes += int64(length)
	return true
}

// readFrame reads one frame FFmpeg sent, checking its header before reading its body.
func readFrame(from io.Reader, expectedSequence int8) (header, []byte, error) {
	raw := make([]byte, headerBytes)
	if _, err := io.ReadFull(from, raw); err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return header{}, nil, fmt.Errorf("%w: a frame ended inside its header", ErrViolation)
		}
		return header{}, nil, fmt.Errorf("read a frame's header: %w", err)
	}
	got := decodeHeader(raw)
	if err := checkHeader(got, expectedSequence); err != nil {
		return header{}, nil, err
	}
	body := make([]byte, got.length)
	if _, err := io.ReadFull(from, body); err != nil {
		return header{}, nil, fmt.Errorf("%w: a frame ended inside its body: %w", ErrViolation, err)
	}
	return got, body, nil
}

func writeAll(to io.Writer, raw []byte) error {
	if _, err := to.Write(raw); err != nil {
		return fmt.Errorf("write to FFmpeg: %w", err)
	}
	return nil
}
