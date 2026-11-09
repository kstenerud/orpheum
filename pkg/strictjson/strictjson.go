// SPDX-License-Identifier: AGPL-3.0-or-later

package strictjson

import (
	"bytes"
	"encoding"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// ErrRefused is a document that breaks one of D456's rules: a duplicate key, a key naming a field
// in another case, invalid UTF-8, a NUL, data after the value, or a limit passed. The error wrapping
// it names the rule and where.
var ErrRefused = errors.New("refused by strict JSON decoding (D456)")

// The limits a document is held to: BONJSON's recommended defaults (D456 §5), and for a whole
// document a bound far below BONJSON's 2,000,000,000 bytes, which a caller's own bound undercuts.
const (
	MaxDepth          = 500
	MaxElements       = 1_000_000
	MaxStringBytes    = 10_000_000
	MaxDocumentBytes  = 100_000_000
	maxKeysScanned    = 32 // past this many keys, an object's keys are looked up in a map
	maxKeyBytesInPath = 64
)

// Unmarshal decodes data as encoding/json's Unmarshal does, once data has passed every rule of D456
// for the type it decodes into; otherwise it decodes nothing and says which rule data broke,
// wrapping ErrRefused.
func Unmarshal(data []byte, into any) error {
	if err := check(data, defaultLimits(), targetOf(into)); err != nil {
		return err
	}
	return decode(data, into)
}

// decode is encoding/json's decoding of a document a check has passed.
func decode(data []byte, into any) error {
	return json.Unmarshal(data, into)
}

// Decode reads reader to its end, at most MaxDocumentBytes, and decodes what it read as Unmarshal
// does: a reader holds one document, and anything after its value is refused.
func Decode(reader io.Reader, into any) error {
	data, err := readDocument(reader)
	if err != nil {
		return err
	}
	return Unmarshal(data, into)
}

// readDocument reads reader to its end, or one byte past MaxDocumentBytes, which the check refuses.
func readDocument(reader io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, MaxDocumentBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read the JSON document: %w", err)
	}
	return data, nil
}

// limits are the bounds a check holds a document to; tests lower them.
type limits struct {
	depth, elements, stringBytes, documentBytes int
}

func defaultLimits() limits {
	return limits{depth: MaxDepth, elements: MaxElements, stringBytes: MaxStringBytes, documentBytes: MaxDocumentBytes}
}

// errNotJSON is a document that is not JSON, which the scan found before encoding/json said so.
var errNotJSON = errors.New("not JSON")

// check is nil when data is one JSON value that keeps every rule of D456 within bounds, its keys
// naming target's fields in their own case.
func check(data []byte, bounds limits, target reflect.Type) error {
	_, err := checkBy(data, bounds, target, rules{})
	return err
}

// checkBy is check by a sender's rules, giving back the document to decode: data itself, or for a
// foreign sender data without the members its repeated keys do not keep.
func checkBy(data []byte, bounds limits, target reflect.Type, by rules) ([]byte, error) {
	if len(data) > bounds.documentBytes {
		return nil, fmt.Errorf("%w: the document is %d bytes, past the limit of %d", ErrRefused, len(data),
			bounds.documentBytes)
	}
	scan := scanner{data: data, bounds: bounds, rules: by}
	scan.skipSpace()
	err := scan.value(target)
	if err == nil {
		scan.skipSpace()
		if scan.offset < len(data) {
			err = scan.refuse("data after the top-level value")
		}
	}
	if errors.Is(err, errNotJSON) {
		// encoding/json's own words for what is wrong, as callers knew them; it checks the whole
		// text before it decodes anything, so the raw message it would fill stays empty.
		var unused json.RawMessage
		if decodeErr := json.Unmarshal(data, &unused); decodeErr != nil {
			return nil, decodeErr
		}
	}
	if err != nil {
		return nil, err
	}
	return withoutDrops(data, scan.drops), nil
}

// frame is one container the scan is inside: its kind, how many elements it has, where its last
// key was, the NFC forms of an object's keys (in scanner.normalised, from keysFrom on), and what
// its elements decode into: a struct's fields, or else the type of every element.
type frame struct {
	isObject                 bool
	elements                 int
	keyStart, keyEnd         int
	keysFrom, normalisedFrom int
	// seen indexes the object's keys by their compared form, once it has many: the latest kept.
	seen        map[string]int
	fields      *structFields
	elementType reflect.Type
}

// scanner walks a document once, checking it against D456's rules.
type scanner struct {
	data   []byte
	offset int
	bounds limits
	frames []frame
	// keys are spans of normalised, the NFC form of each key of the open objects.
	keys       []span
	normalised []byte
	// decoded is a key's text after its escapes, reused from key to key; escaped is one escape's.
	decoded []byte
	escaped [utf8.UTFMax]byte
	// fieldsByType are the fields of each struct type met so far.
	fieldsByType map[reflect.Type]*structFields
	rules        rules
	// members are, for a foreign sender, the members of the open objects, one for each of keys; drops
	// are the spans of the document their repeated keys leave out.
	members []member
	drops   []span
}

type span struct{ start, end int }

func (scan *scanner) refuse(rule string) error {
	return fmt.Errorf("%w: %s at %s (byte %d)", ErrRefused, rule, scan.path(), scan.offset)
}

func (scan *scanner) malformed(what string) error {
	return fmt.Errorf("%w: %s at byte %d", errNotJSON, what, scan.offset)
}

// path is where the scan is, as $."key"[index]: what an error names.
func (scan *scanner) path() string {
	var path strings.Builder
	path.WriteString("$")
	for _, open := range scan.frames {
		switch {
		case open.isObject && open.keyEnd > open.keyStart:
			key := scan.data[open.keyStart:open.keyEnd]
			if len(key) > maxKeyBytesInPath {
				key = key[:maxKeyBytesInPath]
			}
			path.WriteString("." + strconv.QuoteToASCII(string(key)))
		case !open.isObject && open.elements > 0:
			path.WriteString("[" + strconv.Itoa(open.elements-1) + "]")
		}
	}
	return path.String()
}

func (scan *scanner) skipSpace() {
	for scan.offset < len(scan.data) {
		switch scan.data[scan.offset] {
		case ' ', '\t', '\n', '\r':
			scan.offset++
		default:
			return
		}
	}
}

// value checks one value starting at the scan's offset, one level deeper than the open frames,
// that decodes into target.
func (scan *scanner) value(target reflect.Type) error {
	if len(scan.frames)+1 > scan.bounds.depth {
		return scan.refuse(fmt.Sprintf("nesting deeper than %d", scan.bounds.depth))
	}
	if scan.offset >= len(scan.data) {
		return scan.malformed("the document ends where a value should be")
	}
	switch first := scan.data[scan.offset]; {
	case first == '{':
		return scan.container(true, target)
	case first == '[':
		return scan.container(false, target)
	case first == '"':
		_, err := scan.text(false)
		return err
	case first == 't':
		return scan.literal("true")
	case first == 'f':
		return scan.literal("false")
	case first == 'n':
		return scan.literal("null")
	case first == '-' || '0' <= first && first <= '9':
		return scan.number()
	default:
		return scan.malformed("a value cannot start with " + strconv.QuoteRune(rune(first)))
	}
}

func (scan *scanner) literal(word string) error {
	if !bytes.HasPrefix(scan.data[scan.offset:], []byte(word)) {
		return scan.malformed("expected " + word)
	}
	scan.offset += len(word)
	return nil
}

func (scan *scanner) digits() int {
	start := scan.offset
	for scan.offset < len(scan.data) && '0' <= scan.data[scan.offset] && scan.data[scan.offset] <= '9' {
		scan.offset++
	}
	return scan.offset - start
}

// takes moves past the next byte when it is one of these.
func (scan *scanner) takes(these string) bool {
	if scan.offset < len(scan.data) && strings.IndexByte(these, scan.data[scan.offset]) >= 0 {
		scan.offset++
		return true
	}
	return false
}

func (scan *scanner) number() error {
	scan.takes("-")
	if !scan.takes("0") && scan.digits() == 0 {
		return scan.malformed("a number without digits")
	}
	if scan.takes(".") && scan.digits() == 0 {
		return scan.malformed("a fraction without digits")
	}
	if scan.takes("eE") {
		scan.takes("+-")
		if scan.digits() == 0 {
			return scan.malformed("an exponent without digits")
		}
	}
	return nil
}

// container checks an object or an array, its opening bracket at the scan's offset, that decodes
// into target.
func (scan *scanner) container(isObject bool, target reflect.Type) error {
	closing := byte(']')
	if isObject {
		closing = '}'
	}
	scan.frames = append(scan.frames, scan.frameFor(isObject, target))
	scan.offset++
	scan.skipSpace()
	if scan.takes(string(closing)) {
		scan.closeFrame()
		return nil
	}
	for {
		if err := scan.element(isObject); err != nil {
			return err
		}
		scan.skipSpace()
		switch {
		case scan.takes(","):
			scan.skipSpace()
		case scan.takes(string(closing)):
			scan.closeFrame()
			return nil
		default:
			return scan.malformed("expected a comma or the container's end")
		}
	}
}

// frameFor is a new frame for an object or an array that decodes into target: a struct's fields,
// a map's or a list's element type, or free-form.
func (scan *scanner) frameFor(isObject bool, target reflect.Type) frame {
	open := frame{
		isObject: isObject, keysFrom: len(scan.keys), normalisedFrom: len(scan.normalised),
		elementType: freeForm(),
	}
	concrete := concreteOrNil(target)
	if concrete == nil {
		return open
	}
	switch kind := concrete.Kind(); {
	case isObject && kind == reflect.Struct:
		open.fields = scan.fieldsOf(concrete)
	case isObject && kind == reflect.Map, !isObject && (kind == reflect.Slice || kind == reflect.Array):
		open.elementType = concrete.Elem()
	}
	return open
}

// element checks one element of the innermost container: a value, or for an object a key and its
// value.
func (scan *scanner) element(isObject bool) error {
	open := &scan.frames[len(scan.frames)-1]
	open.elements++
	if open.elements > scan.bounds.elements {
		return scan.refuse(fmt.Sprintf("a container of more than %d elements", scan.bounds.elements))
	}
	if !isObject {
		return scan.value(open.elementType)
	}
	if scan.offset >= len(scan.data) || scan.data[scan.offset] != '"' {
		return scan.malformed("an object's key is a string")
	}
	keyStart := scan.offset + 1
	key, err := scan.text(true)
	if err != nil {
		return err
	}
	open = &scan.frames[len(scan.frames)-1]
	open.keyStart, open.keyEnd = keyStart, scan.offset-1
	if addErr := scan.addKey(key, keyStart-1); addErr != nil {
		return addErr
	}
	target, err := scan.fieldOrElement(key)
	if err != nil {
		return err
	}
	scan.skipSpace()
	if !scan.takes(":") {
		return scan.malformed("expected a colon after an object's key")
	}
	scan.skipSpace()
	if err := scan.value(target); err != nil {
		return err
	}
	if scan.rules.foreign {
		scan.members[len(scan.members)-1].end = scan.offset
	}
	return nil
}

// fieldOrElement is what the innermost object's key decodes into: a struct's field named key in
// its own case, a map's value type, or free-form. A key that names a field only when case is
// ignored is refused (D456): encoding/json would fill the field from it.
func (scan *scanner) fieldOrElement(key []byte) (reflect.Type, error) {
	open := &scan.frames[len(scan.frames)-1]
	if open.fields == nil {
		return open.elementType, nil
	}
	if field, named := open.fields.byName[string(key)]; named {
		return field, nil
	}
	if _, hidden := open.fields.hidden[string(key)]; !hidden {
		for _, name := range open.fields.names {
			if bytes.EqualFold(key, []byte(name)) {
				return nil, scan.refuse(fmt.Sprintf("the key %s, which names the field %s only in another case",
					strconv.QuoteToASCII(string(key)), strconv.QuoteToASCII(name)))
			}
		}
	}
	return freeForm(), nil
}

func (scan *scanner) closeFrame() {
	open := scan.frames[len(scan.frames)-1]
	if scan.rules.foreign && open.isObject {
		scan.dropMembers(open.keysFrom)
		scan.members = scan.members[:open.keysFrom]
	}
	scan.keys = scan.keys[:open.keysFrom]
	scan.normalised = scan.normalised[:open.normalisedFrom]
	scan.frames = scan.frames[:len(scan.frames)-1]
}

// addKey adds the key whose member starts at start to the innermost object. D456 refuses a key the
// object already holds, compared after NFC normalisation: two keys that are one text in two forms
// are one key said twice. A foreign sender's keys are compared as encoding/json compares them, as
// they are, and a repeat leaves out the member its Repeats does not keep.
func (scan *scanner) addKey(key []byte, start int) error {
	from := len(scan.normalised)
	if scan.rules.foreign || isASCII(key) || norm.NFC.IsNormal(key) {
		scan.normalised = append(scan.normalised, key...)
	} else {
		scan.normalised = norm.NFC.Append(scan.normalised, key...)
	}
	added := span{from, len(scan.normalised)}
	open := &scan.frames[len(scan.frames)-1]
	held, holds := scan.heldKey(scan.normalised[added.start:added.end])
	dropsAdded := false
	if holds {
		switch {
		case !scan.rules.foreign:
			return scan.refuse("a duplicate key " + strconv.QuoteToASCII(string(key)))
		case scan.rules.repeats == FirstWins:
			dropsAdded = true
		default:
			scan.members[open.keysFrom+held].dropped = true
		}
	}
	scan.keys = append(scan.keys, added)
	if scan.rules.foreign {
		scan.members = append(scan.members, member{start: start, dropped: dropsAdded})
	}
	index := len(scan.keys) - 1 - open.keysFrom
	if open.seen == nil && len(scan.keys)-open.keysFrom > maxKeysScanned {
		open.seen = make(map[string]int, 2*maxKeysScanned)
		for position, kept := range scan.keys[open.keysFrom:] {
			if !scan.dropped(open.keysFrom + position) {
				open.seen[string(scan.normalised[kept.start:kept.end])] = position
			}
		}
	} else if open.seen != nil && !dropsAdded {
		open.seen[string(scan.normalised[added.start:added.end])] = index
	}
	return nil
}

// heldKey is the index, from the innermost object's first key, of the kept key whose compared
// form is normalised, if the object holds one.
func (scan *scanner) heldKey(normalised []byte) (int, bool) {
	open := &scan.frames[len(scan.frames)-1]
	if open.seen != nil {
		index, held := open.seen[string(normalised)]
		return index, held
	}
	for index, held := range slices.Backward(scan.keys[open.keysFrom:]) {
		if !scan.dropped(open.keysFrom+index) && bytes.Equal(scan.normalised[held.start:held.end], normalised) {
			return index, true
		}
	}
	return 0, false
}

// dropped says whether the member of the key at index in scan.keys is left out.
func (scan *scanner) dropped(index int) bool {
	return scan.rules.foreign && scan.members[index].dropped
}

func isASCII(text []byte) bool {
	for _, char := range text {
		if char >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

// text checks a string, its opening quote at the scan's offset, and leaves the offset past its
// closing quote. For a key it gives back the string's text after its escapes, valid until the next
// key; for a value, nothing.
func (scan *scanner) text(isKey bool) ([]byte, error) {
	scan.offset++
	scan.decoded = scan.decoded[:0]
	for decodedBytes := 0; ; {
		if decodedBytes > scan.bounds.stringBytes {
			return nil, scan.refuse(fmt.Sprintf("a string of more than %d bytes", scan.bounds.stringBytes))
		}
		if scan.takes(`"`) {
			return scan.decoded, nil
		}
		piece, err := scan.character()
		if err != nil {
			return nil, err
		}
		decodedBytes += len(piece)
		if isKey {
			scan.decoded = append(scan.decoded, piece...)
		}
	}
}

// character checks one character of a string at the scan's offset, raw or escaped, and gives back
// its UTF-8, valid until the next character.
func (scan *scanner) character() ([]byte, error) {
	if scan.offset >= len(scan.data) {
		return nil, scan.malformed("the document ends inside a string")
	}
	switch char := scan.data[scan.offset]; {
	case char == 0 && !scan.rules.foreign:
		return nil, scan.refuse("a NUL in a string")
	case char < ' ':
		return nil, scan.malformed("a control character in a string")
	case char == '\\':
		decoded, err := scan.escape()
		if err != nil {
			return nil, err
		}
		size := utf8.EncodeRune(scan.escaped[:], decoded)
		return scan.escaped[:size], nil
	}
	decoded, size := utf8.DecodeRune(scan.data[scan.offset:])
	if decoded == utf8.RuneError && size == 1 {
		if !scan.rules.foreign {
			return nil, scan.refuse("invalid UTF-8")
		}
		// encoding/json reads the byte as U+FFFD.
		scan.offset++
		size = utf8.EncodeRune(scan.escaped[:], utf8.RuneError)
		return scan.escaped[:size], nil
	}
	raw := scan.data[scan.offset : scan.offset+size]
	scan.offset += size
	return raw, nil
}

// escape reads one escape, its backslash at the scan's offset, and gives back the rune it stands
// for: a surrogate pair's two escapes are one rune, and a surrogate alone is refused.
func (scan *scanner) escape() (rune, error) {
	if scan.offset+1 >= len(scan.data) {
		return 0, scan.malformed("the document ends inside an escape")
	}
	if decoded, isSimple := simpleEscape(scan.data[scan.offset+1]); isSimple {
		scan.offset += 2
		return decoded, nil
	}
	first, err := scan.hexEscape()
	if err != nil {
		return 0, err
	}
	switch {
	case first == 0 && !scan.rules.foreign:
		return 0, scan.refuse("a NUL in a string")
	case 0xDC00 <= first && first <= 0xDFFF:
		return scan.loneSurrogate("a lone low surrogate")
	case first < 0xD800 || first > 0xDBFF:
		return first, nil
	}
	if !bytes.HasPrefix(scan.data[scan.offset:], []byte{'\\', 'u'}) {
		return scan.loneSurrogate("a lone high surrogate")
	}
	second, err := scan.hexEscape()
	if err != nil {
		return 0, err
	}
	if second < 0xDC00 || second > 0xDFFF {
		// encoding/json reads the high surrogate as U+FFFD and the escape after it on its own.
		scan.offset -= len(`\uXXXX`)
		return scan.loneSurrogate("a high surrogate not followed by a low one")
	}
	return utf16.DecodeRune(first, second), nil
}

// loneSurrogate is what a surrogate without its pair stands for: refused by D456, and U+FFFD from a
// foreign sender, as encoding/json reads it.
func (scan *scanner) loneSurrogate(rule string) (rune, error) {
	if scan.rules.foreign {
		return utf8.RuneError, nil
	}
	return 0, scan.refuse(rule)
}

// hexEscape reads a backslash, a u and four hex digits at the scan's offset.
func (scan *scanner) hexEscape() (rune, error) {
	if scan.offset+6 > len(scan.data) || scan.data[scan.offset+1] != 'u' {
		return 0, scan.malformed("an unknown escape")
	}
	var value rune
	for _, digit := range scan.data[scan.offset+2 : scan.offset+6] {
		var nibble byte
		switch {
		case '0' <= digit && digit <= '9':
			nibble = digit - '0'
		case 'a' <= digit && digit <= 'f':
			nibble = digit - 'a' + 10
		case 'A' <= digit && digit <= 'F':
			nibble = digit - 'A' + 10
		default:
			return 0, scan.malformed("an escape without four hex digits")
		}
		value = value<<4 | rune(nibble)
	}
	scan.offset += 6
	return value, nil
}

// simpleEscape is the rune a one-letter escape stands for.
func simpleEscape(letter byte) (rune, bool) {
	switch letter {
	case '"', '\\', '/':
		return rune(letter), true
	case 'b':
		return '\b', true
	case 'f':
		return '\f', true
	case 'n':
		return '\n', true
	case 'r':
		return '\r', true
	case 't':
		return '\t', true
	default:
		return 0, false
	}
}

// fieldClaim is one struct field claiming a name, and whether its tag gives the name.
type fieldClaim struct {
	fieldType reflect.Type
	tagged    bool
}

// structFields are the names encoding/json decodes a struct's fields from, each with its field's
// type, and the names it ignores because two fields at one depth claim them.
type structFields struct {
	byName map[string]reflect.Type
	hidden map[string]struct{}
	names  []string
}

// fieldsOf is target's fields as encoding/json finds them: a tag's name or the field's own, none
// for "-", and an embedded struct's fields promoted unless a shallower field has the name.
func (scan *scanner) fieldsOf(target reflect.Type) *structFields {
	if found, met := scan.fieldsByType[target]; met {
		return found
	}
	fields := &structFields{byName: map[string]reflect.Type{}, hidden: map[string]struct{}{}}
	visited := map[reflect.Type]bool{}
	for level := []reflect.Type{target}; len(level) > 0; {
		claimsByName, deeper := claimsAtLevel(level, visited)
		for name, claims := range claimsByName {
			fields.settle(name, claims)
		}
		level = deeper
	}
	if scan.fieldsByType == nil {
		scan.fieldsByType = map[reflect.Type]*structFields{}
	}
	scan.fieldsByType[target] = fields
	return fields
}

// claimsAtLevel is every name the structs of one depth claim, and the embedded structs one deeper.
func claimsAtLevel(
	level []reflect.Type, visited map[reflect.Type]bool,
) (claimsByName map[string][]fieldClaim, deeper []reflect.Type) {
	claimsByName = map[string][]fieldClaim{}
	for _, structType := range level {
		if visited[structType] {
			continue
		}
		visited[structType] = true
		for field := range structType.Fields() {
			embedded, name, fieldType := promotedOrNamed(field)
			if embedded != nil {
				deeper = append(deeper, embedded)
			}
			if fieldType != nil {
				tagName, _, _ := strings.Cut(field.Tag.Get("json"), ",")
				claimsByName[name] = append(claimsByName[name], fieldClaim{fieldType, tagName != ""})
			}
		}
	}
	return claimsByName, deeper
}

// settle gives name to the field encoding/json would fill from it, unless a shallower depth
// settled it already: the only claimant, or else the only tagged one; with neither, no field.
func (fields *structFields) settle(name string, claims []fieldClaim) {
	if _, named := fields.byName[name]; named {
		return
	}
	if _, hidden := fields.hidden[name]; hidden {
		return
	}
	var winner *fieldClaim
	for index, said := range claims {
		switch {
		case len(claims) == 1 || said.tagged && winner == nil:
			winner = &claims[index]
		case said.tagged:
			fields.hidden[name] = struct{}{}
			return
		}
	}
	if winner == nil {
		fields.hidden[name] = struct{}{}
		return
	}
	fields.byName[name] = winner.fieldType
	fields.names = append(fields.names, name)
}

// promotedOrNamed is what one struct field gives encoding/json: an embedded struct whose fields it
// promotes, or a name and the field's type; neither for an unexported field or one tagged "-".
func promotedOrNamed(field reflect.StructField) (embedded reflect.Type, name string, fieldType reflect.Type) {
	tag := field.Tag.Get("json")
	if tag == "-" {
		return nil, "", nil
	}
	name, _, _ = strings.Cut(tag, ",")
	base := field.Type
	if base.Kind() == reflect.Pointer {
		base = base.Elem()
	}
	if field.Anonymous && name == "" && base.Kind() == reflect.Struct {
		return base, "", nil
	}
	if !field.IsExported() {
		return nil, "", nil
	}
	if name == "" {
		name = field.Name
	}
	return nil, name, field.Type
}

// freeForm is the type of a value whose keys are the sender's: anything an interface takes.
func freeForm() reflect.Type { return reflect.TypeFor[any]() }

// concreteOrNil is the type a value decodes into once pointers are followed, or nil when the
// value is free-form: an interface, or a type that decodes itself.
func concreteOrNil(target reflect.Type) reflect.Type {
	jsonUnmarshaler := reflect.TypeFor[json.Unmarshaler]()
	textUnmarshaler := reflect.TypeFor[encoding.TextUnmarshaler]()
	for target != nil {
		decodesItself := target.Implements(jsonUnmarshaler) || target.Implements(textUnmarshaler) ||
			target.Kind() != reflect.Pointer && (reflect.PointerTo(target).Implements(jsonUnmarshaler) ||
				reflect.PointerTo(target).Implements(textUnmarshaler))
		switch {
		case decodesItself || target.Kind() == reflect.Interface:
			return nil
		case target.Kind() == reflect.Pointer:
			target = target.Elem()
		default:
			return target
		}
	}
	return nil
}
