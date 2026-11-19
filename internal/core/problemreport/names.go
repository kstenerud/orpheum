// SPDX-License-Identifier: AGPL-3.0-or-later

package problemreport

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"path"
	"strings"
)

// Kind is what a pseudonymised value names; a tag starts with it (`item:a1b2c3`).
type Kind string

// The kinds a report's tags name.
const (
	// KindItem is a film's, a series', an episode's or an other video's title.
	KindItem Kind = "item"
	// KindPath is a file's or a folder's path: its tag keeps its depth and the file's extension.
	KindPath Kind = "path"
	// KindUser is a person's name in the household.
	KindUser Kind = "user"
	// KindDevice is a device's name.
	KindDevice Kind = "device"
	// KindNode is a media node's name.
	KindNode Kind = "node"
	// KindHouse is the household's (the realm's) name.
	KindHouse Kind = "house"
	// KindAddress is a network address or a host's name.
	KindAddress Kind = "address"
	// KindEmail is an e-mail address.
	KindEmail Kind = "email"
	// KindText is any other value the report does not know to be only shape.
	KindText Kind = "text"
)

// known says whether kind is one a Known may name.
func (kind Kind) known() bool {
	switch kind {
	case KindItem, KindPath, KindUser, KindDevice, KindNode, KindHouse, KindAddress, KindEmail, KindText:
		return true
	}
	return false
}

// tagHexStart is a tag's first length in hex digits; a collision lengthens it by tagHexStep.
const (
	tagHexStart = 6
	tagHexStep  = 2
)

// pathPartSpace is the space of a path's components' tags: one component has one tag in every path
// it is part of, so two files of one folder show it.
const pathPartSpace = "/"

// entry is one value the report names, with the tag that stands for it.
type entry struct {
	kind  Kind
	value string
	tag   string
	// other is true when the value names an other video (D349 §10): the full form leaves its tag
	// unless the download asks for other libraries.
	other bool
}

// names are a report's tags: one salt, one tag a value, no tag two values'.
type names struct {
	salt []byte
	// entries are by kind and value; order is the order they were first named, the table's.
	entries map[entryKey]*entry
	order   []*entry
	// hexes are each space's tags by value, and taken the values each tag stands for, so a
	// collision is seen and lengthened.
	hexes map[string]map[string]string
	taken map[string]map[string]string
	// otherRoots are the roots of the other libraries, each ending in a slash.
	otherRoots []string
}

type entryKey struct {
	kind  Kind
	value string
}

func newNames(salt []byte, otherRoots []string) *names {
	roots := make([]string, 0, len(otherRoots))
	for _, root := range otherRoots {
		if root = strings.TrimRight(root, "/"); root != "" {
			roots = append(roots, root+"/")
		}
	}
	return &names{
		salt: salt, entries: map[entryKey]*entry{}, hexes: map[string]map[string]string{},
		taken: map[string]map[string]string{}, otherRoots: roots,
	}
}

// name is the entry for value as kind, made the first time it is asked for. Asked for as other, it
// is an other video's; so is a path under an other root, however it is asked for.
func (names *names) name(kind Kind, value string, other bool) *entry {
	key := entryKey{kind, value}
	if found, ok := names.entries[key]; ok {
		found.other = found.other || other
		return found
	}
	made := &entry{kind: kind, value: value, other: other}
	if kind == KindPath {
		made.tag = string(KindPath) + ":" + names.pathTag(value)
		made.other = other || names.underOtherRoot(value)
	} else {
		made.tag = string(kind) + ":" + names.hex(string(kind), value)
	}
	names.entries[key] = made
	names.order = append(names.order, made)
	return made
}

// pathTag is value with each component replaced by its own tag, its slashes (so its depth) and
// its last component's extension kept: /media/a/b.mkv is three tags under slashes, the last ending .mkv.
func (names *names) pathTag(value string) string {
	parts := strings.Split(value, "/")
	for index, part := range parts {
		switch part {
		case "", ".", "..":
			continue
		}
		tag := names.hex(pathPartSpace, part)
		if index == len(parts)-1 {
			tag += extension(part)
		}
		parts[index] = tag
	}
	return strings.Join(parts, "/")
}

// extension is a file name's extension when it looks like one (a dot and one to five letters or
// digits), else nothing: the shape a container's name keeps.
func extension(name string) string {
	ext := path.Ext(name)
	if len(ext) < 2 || len(ext) > 6 || ext == name {
		return ""
	}
	for _, r := range ext[1:] {
		if !isASCIIAlnum(r) {
			return ""
		}
	}
	return ext
}

func isASCIIAlnum(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'
}

func (names *names) underOtherRoot(value string) bool {
	for _, root := range names.otherRoots {
		if value+"/" == root || strings.HasPrefix(value, root) {
			return true
		}
	}
	return false
}

// hex is value's tag within space: the salted hash's first digits, lengthened while another value
// of the space already has them.
func (names *names) hex(space, value string) string {
	if names.hexes[space] == nil {
		names.hexes[space] = map[string]string{}
		names.taken[space] = map[string]string{}
	}
	if tag, ok := names.hexes[space][value]; ok {
		return tag
	}
	mac := hmac.New(sha256.New, names.salt)
	_, _ = mac.Write([]byte(space))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(value))
	sum := hex.EncodeToString(mac.Sum(nil))
	for length := tagHexStart; ; length += tagHexStep {
		tag := sum[:min(length, len(sum))]
		owner, isTaken := names.taken[space][tag]
		if !isTaken || owner == value || length >= len(sum) {
			names.taken[space][tag] = value
			names.hexes[space][value] = tag
			return tag
		}
	}
}

// visible is how an entry reads in a form: the full form names it, except an other video's when the
// download did not ask for them; the shareable form gives its tag.
func (options Options) visible(named *entry) string {
	if options.Form == FormFull && (!named.other || options.IncludeOther) {
		return named.value
	}
	return named.tag
}
