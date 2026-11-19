// SPDX-License-Identifier: AGPL-3.0-or-later

package problemreport

import (
	"net/netip"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// segment is a piece of a text: literal words, or a named value read as the form shows it. A
// bare named path is a URL's, which reads without its kind.
type segment struct {
	literal string
	named   *entry
	bare    bool
}

// text is a free text scrubbed: its literal words, and every value that names something as a
// segment of its own.
type text []segment

func literal(words string) text { return text{{literal: words}} }

func named(value *entry) text { return text{{named: value}} }

// read is the text as the form shows it.
func (scrubbed text) read(options Options) string {
	var built strings.Builder
	for _, each := range scrubbed {
		switch {
		case each.named == nil:
			built.WriteString(each.literal)
		case each.bare:
			built.WriteString(strings.TrimPrefix(options.visible(each.named), string(KindPath)+":"))
		default:
			built.WriteString(options.visible(each.named))
		}
	}
	return built.String()
}

// shortRunes is the length below which a value is matched in free text only with its own case: a
// short title must not take a common word's lower-case use. A value of fewer than minRunes is
// not looked for at all: a one-letter name would take every lone letter.
const (
	shortRunes = 5
	minRunes   = 2
)

// candidate is one value a scrubber looks for in free text.
type candidate struct {
	kind  Kind
	value string
	other bool
	// lead is how many bytes of value come before its first word, which the index is keyed by.
	lead int
}

// scrubber replaces what can identify in free text by named segments (D524 "How").
type scrubber struct {
	names *names
	// byFirstWord are the values to look for by their first word in lower case, the longest first
	// once sorted; paths are kept apart, looked for before any other value.
	byFirstWord, pathsByFirstWord map[string][]candidate
	sorted                        bool
	seen                          map[entryKey]bool
}

func newScrubber(names *names) *scrubber {
	return &scrubber{
		names: names, byFirstWord: map[string][]candidate{}, pathsByFirstWord: map[string][]candidate{},
		seen: map[entryKey]bool{},
	}
}

// know adds a value to look for in free text: a value the caller knows names something, or one the
// report pseudonymised in a field.
func (scrubber *scrubber) know(kind Kind, value string, other bool) {
	key := entryKey{kind, value}
	if scrubber.seen[key] || utf8.RuneCountInString(value) < minRunes {
		return
	}
	lead := strings.IndexFunc(value, isWordRune)
	if lead < 0 {
		return
	}
	scrubber.seen[key] = true
	word := strings.ToLower(firstWord(value[lead:]))
	index := scrubber.byFirstWord
	if kind == KindPath {
		index = scrubber.pathsByFirstWord
	}
	index[word] = append(index[word], candidate{kind: kind, value: value, other: other, lead: lead})
	scrubber.sorted = false
}

func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

func firstWord(words string) string {
	end := strings.IndexFunc(words, func(r rune) bool { return !isWordRune(r) })
	if end < 0 {
		return words
	}
	return words[:end]
}

func (scrubber *scrubber) sort() {
	if scrubber.sorted {
		return
	}
	for _, index := range []map[string][]candidate{scrubber.byFirstWord, scrubber.pathsByFirstWord} {
		for word, candidates := range index {
			slices.SortStableFunc(candidates, func(a, b candidate) int { return len(b.value) - len(a.value) })
			index[word] = candidates
		}
	}
	scrubber.sorted = true
}

// scrub is words with, in this order: URLs' hosts and paths named and their user, query and
// fragment dropped (both forms: tokens live there); e-mail addresses named; known paths, then
// any other absolute path, named with their depth and extension; IP addresses named, but for a
// loopback or unspecified one; then every other known value and every value the report named
// elsewhere, the longest first, each as a whole word.
func (scrubber *scrubber) scrub(words string) text {
	scrubber.sort()
	scrubbed := literal(words)
	scrubbed = eachLiteral(scrubbed, scrubber.urls)
	scrubbed = eachLiteral(scrubbed, scrubber.emails)
	scrubbed = eachLiteral(scrubbed, func(words string) text {
		return scrubber.known(words, scrubber.pathsByFirstWord)
	})
	scrubbed = eachLiteral(scrubbed, scrubber.paths)
	scrubbed = eachLiteral(scrubbed, scrubber.addresses)
	return eachLiteral(scrubbed, func(words string) text { return scrubber.known(words, scrubber.byFirstWord) })
}

// eachLiteral replaces each literal segment of scrubbed by what replace makes of it.
func eachLiteral(scrubbed text, replace func(string) text) text {
	var out text
	for _, each := range scrubbed {
		if each.named != nil {
			out = append(out, each)
			continue
		}
		out = append(out, replace(each.literal)...)
	}
	return out
}

// byMatches is words with each match of pattern replaced by what replace makes of it.
func byMatches(words string, pattern *regexp.Regexp, replace func(string) text) text {
	var out text
	cursor := 0
	for _, at := range pattern.FindAllStringIndex(words, -1) {
		out = append(out, segment{literal: words[cursor:at[0]]})
		out = append(out, replace(words[at[0]:at[1]])...)
		cursor = at[1]
	}
	return append(out, segment{literal: words[cursor:]})
}

var urlPattern = regexp.MustCompile(`[A-Za-z][A-Za-z0-9+.-]*://[^\s"'<>` + "`" + `]+`)

// dropped is what a URL's user, query or fragment reads as.
const dropped = "[dropped]"

func (scrubber *scrubber) urls(words string) text {
	return byMatches(words, urlPattern, func(match string) text {
		trimmed := strings.TrimRight(match, ".,;:)]")
		trail := segment{literal: match[len(trimmed):]}
		parsed, err := url.Parse(trimmed)
		if err != nil {
			return append(named(scrubber.names.name(KindText, trimmed, false)), trail)
		}
		out := literal(parsed.Scheme + "://")
		if parsed.User != nil {
			out = append(out, segment{literal: dropped + "@"})
		}
		out = append(out, scrubber.host(parsed)...)
		if parsed.Path != "" && parsed.Path != "/" {
			out = append(out, segment{named: scrubber.names.name(KindPath, parsed.Path, false), bare: true})
		} else {
			out = append(out, segment{literal: parsed.Path})
		}
		if parsed.RawQuery != "" || parsed.ForceQuery {
			out = append(out, segment{literal: "?" + dropped})
		}
		if parsed.Fragment != "" || strings.Contains(trimmed, "#") {
			out = append(out, segment{literal: "#" + dropped})
		}
		return append(out, trail)
	})
}

// host is a URL's host, named unless it names no one, and its port.
func (scrubber *scrubber) host(parsed *url.URL) text {
	var out text
	if host := parsed.Hostname(); keptHost(host) {
		out = append(out, segment{literal: host})
	} else {
		out = append(out, segment{named: scrubber.names.name(KindAddress, host, false)})
	}
	if port := parsed.Port(); port != "" {
		out = append(out, segment{literal: ":" + port})
	}
	return out
}

// keptHost is a host that names no one: this machine, or no address at all.
func keptHost(host string) bool {
	if host == "" || host == "localhost" {
		return true
	}
	address, err := netip.ParseAddr(host)
	return err == nil && (address.IsLoopback() || address.IsUnspecified())
}

var emailPattern = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9-]+(?:\.[A-Za-z0-9-]+)+`)

func (scrubber *scrubber) emails(words string) text {
	return byMatches(words, emailPattern, func(match string) text {
		return named(scrubber.names.name(KindEmail, match, false))
	})
}

var (
	ipv4Pattern = regexp.MustCompile(`\b\d{1,3}(?:\.\d{1,3}){3}\b`)
	ipv6Pattern = regexp.MustCompile(`[0-9A-Fa-f]{0,4}(?::[0-9A-Fa-f]{0,4}){2,7}`)
)

func (scrubber *scrubber) addresses(words string) text {
	address := func(match string) text {
		parsed, err := netip.ParseAddr(match)
		if err != nil || parsed.IsLoopback() || parsed.IsUnspecified() {
			return literal(match)
		}
		// A clock's 10:30:45 or a MAC's colons parse as no address; an IPv6 one has all eight
		// groups or a "::".
		if parsed.Is6() && !strings.Contains(match, "::") && strings.Count(match, ":") != 7 {
			return literal(match)
		}
		return named(scrubber.names.name(KindAddress, match, false))
	}
	scrubbed := byMatches(words, ipv4Pattern, address)
	return eachLiteral(scrubbed, func(words string) text { return byMatches(words, ipv6Pattern, address) })
}

// pathStarts are the characters an absolute path may follow in free text; one following a letter
// or a colon is a URL's or a word's.
const pathStarts = " \t\n\"'`=([,"

// paths names every absolute path in words. A path runs to a line's end, a quote, an angle
// bracket, or a colon, comma or semicolon followed by a space (Go's "open /a/b: no such file"),
// spaces and all: a title's words are not left behind it. A path followed by plain words is taken
// with them: the sense is lost, never the privacy.
func (scrubber *scrubber) paths(words string) text {
	var out text
	cursor := 0
	for start := 0; start < len(words); start++ {
		if !pathStartsAt(words, start) {
			continue
		}
		found := strings.TrimRight(words[start:pathEnd(words, start)], " .,:;")
		out = append(out, segment{literal: words[cursor:start]},
			segment{named: scrubber.names.name(KindPath, found, false)})
		cursor = start + len(found)
		start = cursor - 1
	}
	return append(out, segment{literal: words[cursor:]})
}

// pathStartsAt says whether an absolute path starts at words[start]: a slash, after nothing or one
// of pathStarts, before neither a slash nor a space.
func pathStartsAt(words string, start int) bool {
	if words[start] != '/' || (start > 0 && !strings.ContainsRune(pathStarts, rune(words[start-1]))) {
		return false
	}
	return start+1 < len(words) && words[start+1] != '/' && words[start+1] != ' '
}

func pathEnd(words string, start int) int {
	for at := start; at < len(words); at++ {
		switch words[at] {
		case '\n', '\r', '\t', '"', '\'', '`', '<', '>':
			return at
		case ':', ',', ';':
			if at+1 < len(words) && words[at+1] == ' ' {
				return at
			}
		}
	}
	return len(words)
}

// known names each value of index found in words as a whole word: no letter or digit just before
// or after it. A value of shortRunes or more matches in any case; a shorter one only in its own.
// A path matches only where a path may start, and not as the head of a longer path.
func (scrubber *scrubber) known(words string, index map[string][]candidate) text {
	var out text
	cursor := 0
	previous := rune(-1)
	for offset := 0; offset < len(words); {
		char, size := utf8.DecodeRuneInString(words[offset:])
		if !isWordRune(char) || (previous != -1 && isWordRune(previous)) {
			previous = char
			offset += size
			continue
		}
		matched := false
		for _, each := range index[strings.ToLower(firstWord(words[offset:]))] {
			start := offset - each.lead
			end := start + len(each.value)
			if start < cursor || end > len(words) || !matches(words, start, end, each) {
				continue
			}
			out = append(out, segment{literal: words[cursor:start]},
				segment{named: scrubber.names.name(each.kind, each.value, each.other)})
			cursor, offset, matched = end, end, true
			previous, _ = utf8.DecodeLastRuneInString(words[:end])
			break
		}
		if !matched {
			previous = char
			offset += size
		}
	}
	return append(out, segment{literal: words[cursor:]})
}

func matches(words string, start, end int, each candidate) bool {
	found := words[start:end]
	if each.kind == KindPath || utf8.RuneCountInString(each.value) < shortRunes {
		if found != each.value {
			return false
		}
	} else if !strings.EqualFold(found, each.value) {
		return false
	}
	first, _ := utf8.DecodeRuneInString(each.value)
	last, _ := utf8.DecodeLastRuneInString(each.value)
	if start > 0 {
		before, _ := utf8.DecodeLastRuneInString(words[:start])
		if isWordRune(first) && isWordRune(before) {
			return false
		}
		if each.kind == KindPath && !strings.ContainsRune(pathStarts, before) {
			return false
		}
	}
	if end < len(words) {
		after, _ := utf8.DecodeRuneInString(words[end:])
		if isWordRune(last) && isWordRune(after) {
			return false
		}
		if each.kind == KindPath && after == '/' {
			return false
		}
	}
	return true
}
