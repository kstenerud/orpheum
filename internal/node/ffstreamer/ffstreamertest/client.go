// SPDX-License-Identifier: AGPL-3.0-or-later

package ffstreamertest

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
)

// The frame types, as the specification's Message Types numbers them.
const (
	typeHello      int8 = 1
	typeCapability int8 = 2
	typeRequest    int8 = 3
	typeRead       int8 = 4
	typeData       int8 = 5
	typeDone       int8 = 6
)

const headerBytes = 6

// Client is FFmpeg's side of one channel: it reads the node's frames from reading and writes its
// own to writing, the two descriptors FFmpeg inherits.
type Client struct {
	reading, writing *os.File
	sequence         int8
	// Terms are the node's REQUEST, once Open has read it.
	Run, FileSize          int64
	BlockSize, LongestRead int32
	Outstanding            int8
}

// NewClient is the client over the descriptors FFmpeg was handed.
func NewClient(readFD, writeFD int) *Client {
	return &Client{
		reading: os.NewFile(uintptr(readFD), "ffstreamer-read"),
		writing: os.NewFile(uintptr(writeFD), "ffstreamer-write"),
	}
}

// Open makes FFmpeg's side of the opening exchange: its HELLO for version 1, the node's, its
// CAPABILITY offering blocks of block bytes and two READs outstanding, and the node's terms.
func (client *Client) Open(block int32) error {
	if err := client.send(typeHello, put(put(nil, int16(1)), int16(1))); err != nil {
		return err
	}
	if _, err := client.receive(typeHello, 0); err != nil {
		return err
	}
	if err := client.send(typeCapability, put(put(put(nil, block), 4*block), int8(2))); err != nil {
		return err
	}
	terms, err := client.receive(typeRequest, 1)
	if err != nil {
		return err
	}
	if len(terms) != 27 {
		return fmt.Errorf("a REQUEST of %d bytes", len(terms))
	}
	client.Run = int64(binary.LittleEndian.Uint64(terms[2:10]))          //nolint:gosec // G115: the wire's signed field
	client.FileSize = int64(binary.LittleEndian.Uint64(terms[10:18]))    //nolint:gosec // as above
	client.BlockSize = int32(binary.LittleEndian.Uint32(terms[18:22]))   //nolint:gosec // as above
	client.LongestRead = int32(binary.LittleEndian.Uint32(terms[22:26])) //nolint:gosec // as above
	client.Outstanding = int8(terms[26])                                 //nolint:gosec // as above
	return nil
}

// ReadAll reads the whole film, block by block, one READ at a time.
func (client *Client) ReadAll() ([]byte, error) {
	film := make([]byte, 0, client.FileSize)
	for offset := int64(0); offset < client.FileSize; offset += int64(client.BlockSize) {
		length := int32(min(int64(client.BlockSize), client.FileSize-offset)) //nolint:gosec // G115: a block
		sequence := client.sequence
		if err := client.send(typeRead, put(put(nil, offset), length)); err != nil {
			return nil, err
		}
		body, err := client.receive(typeData, sequence)
		if err != nil {
			return nil, err
		}
		if len(body) != int(length) {
			return nil, fmt.Errorf("DATA of %d bytes for a READ of %d", len(body), length)
		}
		film = append(film, body...)
	}
	return film, nil
}

// Read asks for one range, and sends whatever it is told, within the terms or not: a test of the
// node's judging.
func (client *Client) Read(offset int64, length int32) error {
	return client.send(typeRead, put(put(nil, offset), length))
}

// Done says DONE OK for the run the terms gave.
func (client *Client) Done() error {
	return client.send(typeDone, put(put(nil, client.Run), int8(0)))
}

func (client *Client) send(kind int8, body []byte) error {
	raw := put(put(put(nil, kind), client.sequence), int32(len(body))) //nolint:gosec // G115: small bodies
	client.sequence = int8((int(client.sequence) + 1) % 128)
	if _, err := client.writing.Write(append(raw, body...)); err != nil {
		return fmt.Errorf("write a frame: %w", err)
	}
	return nil
}

// receive reads the node's next frame, which must be of kind and echo sequence.
func (client *Client) receive(kind, sequence int8) ([]byte, error) {
	raw := make([]byte, headerBytes)
	if _, err := io.ReadFull(client.reading, raw); err != nil {
		return nil, fmt.Errorf("read the node's frame: %w", err)
	}
	gotKind, gotSequence := int8(raw[0]), int8(raw[1])    //nolint:gosec // G115: the wire's signed bytes
	length := int32(binary.LittleEndian.Uint32(raw[2:6])) //nolint:gosec // G115: as above
	if gotKind != kind || gotSequence != sequence || length < 0 {
		return nil, fmt.Errorf("the node sent type %d, sequence %d, length %d; want type %d, sequence %d",
			gotKind, gotSequence, length, kind, sequence)
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(client.reading, body); err != nil {
		return nil, errors.Join(errors.New("the node's frame ended early"), err)
	}
	return body, nil
}

type signed interface {
	~int8 | ~int16 | ~int32 | ~int64
}

func put[T signed](raw []byte, value T) []byte {
	//nolint:errcheck // binary.Append fails only for a type of no fixed size, which signed rules out
	out, _ := binary.Append(raw, binary.LittleEndian, value)
	return out
}
