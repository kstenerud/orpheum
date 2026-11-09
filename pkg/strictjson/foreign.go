// SPDX-License-Identifier: AGPL-3.0-or-later

package strictjson

import (
	"fmt"
	"io"
	"reflect"
	"slices"
)

// Foreign reads a document whose sender is not Orpheum's (D458), and is named by the rule that
// sender's own JSON reader follows for a key said twice in one object. It takes anything JSON
// allows, within the same limits, since the sender's quirks are its own and what is read is trusted
// by where it comes from: an escaped NUL, and invalid UTF-8 and a lone surrogate as U+FFFD, as
// encoding/json reads them. A key still fills a field in its own case alone, and one naming a field
// only in another case is refused. The edge that reads the document cleans what its sender is known
// to get wrong.
type Foreign int

const (
	// LastWins keeps a key's last value: the common behaviour (JavaScript's JSON.parse, Go's and
	// Python's standard readers), for a sender whose own rule is not known.
	LastWins Foreign = iota + 1
	// FirstWins keeps its first: FFmpeg's, whose av_dict_get gives the first of the entries its
	// tag conversion makes under one name, for ffprobe.
	FirstWins
)

// Unmarshal decodes data as encoding/json's Unmarshal does, the values a repeated key does not keep
// left out; a document past a limit, or naming a field in another case, is refused, wrapping
// ErrRefused.
func (repeats Foreign) Unmarshal(data []byte, into any) error {
	if repeats != LastWins && repeats != FirstWins {
		return fmt.Errorf("strictjson: Foreign is %d, not LastWins or FirstWins", int(repeats))
	}
	kept, err := checkBy(data, defaultLimits(), targetOf(into), rules{foreign: true, repeats: repeats})
	if err != nil {
		return err
	}
	return decode(kept, into)
}

// Decode reads reader to its end, at most MaxDocumentBytes, and decodes what it read as Unmarshal
// does.
func (repeats Foreign) Decode(reader io.Reader, into any) error {
	data, err := readDocument(reader)
	if err != nil {
		return err
	}
	return repeats.Unmarshal(data, into)
}

// rules are how a check reads a document: D456's, or a foreign sender's.
type rules struct {
	foreign bool
	repeats Foreign
}

// targetOf is the type into decodes into.
func targetOf(into any) reflect.Type {
	if pointer := reflect.TypeOf(into); pointer != nil && pointer.Kind() == reflect.Pointer {
		return pointer.Elem()
	}
	return freeForm()
}

// member is one key and its value in an open object: where it starts (its key's opening quote) and
// ends (past its value), and whether a repeat of its key leaves it out.
type member struct {
	start, end int
	dropped    bool
}

// dropMembers records, as the object whose members begin at from closes, the spans of the document
// that leave out its dropped members with the commas between them, so what is left is one object.
func (scan *scanner) dropMembers(from int) {
	members := scan.members[from:]
	if !slices.ContainsFunc(members, func(held member) bool { return held.dropped }) {
		return
	}
	lastKept := -1
	for index, held := range members {
		switch {
		case !held.dropped:
			lastKept = index
		case index+1 < len(members):
			scan.drops = append(scan.drops, span{held.start, members[index+1].start})
		default:
			// The last member, after a kept one: its comma goes with it.
			scan.drops = append(scan.drops, span{members[lastKept].end, held.end})
		}
	}
}

// withoutDrops is data with the dropped spans left out; spans that nest or touch are one.
func withoutDrops(data []byte, drops []span) []byte {
	if len(drops) == 0 {
		return data
	}
	slices.SortFunc(drops, func(a, b span) int { return a.start - b.start })
	kept := make([]byte, 0, len(data))
	copied := 0
	for _, drop := range drops {
		if drop.end <= copied {
			continue
		}
		if drop.start > copied {
			kept = append(kept, data[copied:drop.start]...)
		}
		copied = drop.end
	}
	return append(kept, data[copied:]...)
}
