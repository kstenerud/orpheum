// SPDX-License-Identifier: AGPL-3.0-or-later

package release

import (
	"cmp"
	"strconv"
	"strings"
)

// version is a semantic version's parts: "v1.2.3-beta.5" is 1, 2, 3 and beta.5.
type version struct {
	numbers    [3]int
	preRelease []string
}

// parse reads a release's tag, "v1.2.3" or "v1.2.3-beta.5", its build metadata ignored; false for
// anything else.
func parse(tag string) (version, bool) {
	rest, isTag := strings.CutPrefix(tag, "v")
	if !isTag {
		return version{}, false
	}
	rest, _, _ = strings.Cut(rest, "+")
	core, preRelease, hasPreRelease := strings.Cut(rest, "-")
	parts := strings.Split(core, ".")
	var parsed version
	if len(parts) != len(parsed.numbers) {
		return version{}, false
	}
	for index, part := range parts {
		number, err := strconv.Atoi(part)
		if err != nil || number < 0 || part != strconv.Itoa(number) {
			return version{}, false
		}
		parsed.numbers[index] = number
	}
	if hasPreRelease {
		if preRelease == "" {
			return version{}, false
		}
		parsed.preRelease = strings.Split(preRelease, ".")
	}
	return parsed, true
}

// Compare orders two releases' tags as semantic versions: negative when a is older than b, zero when
// they are one release, positive when a is newer; false when either is no release's tag.
func Compare(a, b string) (int, bool) {
	first, isFirst := parse(a)
	second, isSecond := parse(b)
	if !isFirst || !isSecond {
		return 0, false
	}
	for index := range first.numbers {
		if order := cmp.Compare(first.numbers[index], second.numbers[index]); order != 0 {
			return order, true
		}
	}
	return comparePreRelease(first.preRelease, second.preRelease), true
}

// comparePreRelease orders two pre-releases of one version, as semantic versioning's §11 does: none
// is the release, after every pre-release; numeric parts compare as numbers, before any word; a
// longer one is after a shorter one it begins with.
func comparePreRelease(first, second []string) int {
	switch {
	case len(first) == 0 && len(second) == 0:
		return 0
	case len(first) == 0:
		return 1
	case len(second) == 0:
		return -1
	}
	for index := range min(len(first), len(second)) {
		if order := comparePart(first[index], second[index]); order != 0 {
			return order
		}
	}
	return cmp.Compare(len(first), len(second))
}

// comparePart orders one part of two pre-releases.
func comparePart(first, second string) int {
	firstNumber, firstErr := strconv.Atoi(first)
	secondNumber, secondErr := strconv.Atoi(second)
	switch {
	case firstErr == nil && secondErr == nil:
		return cmp.Compare(firstNumber, secondNumber)
	case firstErr == nil:
		return -1
	case secondErr == nil:
		return 1
	}
	return strings.Compare(first, second)
}
