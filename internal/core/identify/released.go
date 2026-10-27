// SPDX-License-Identifier: AGPL-3.0-or-later

package identify

import (
	"strconv"
	"strings"
)

// releasedOn is the day a film was released or a series first aired, as a list by release date sorts
// it (D366 §1): YYYYMMDD, a month or day the source does not give written 00, so 1995 alone is
// 19950000 and comes before every day of 1995. The date is the source's, ISO 8601 (NFO's premiered,
// TMDB's release_date or first_air_date), and the year the title's as resolved, a person's override
// included: where the date is of another year, or there is none, the year alone dates it. A title
// with no year has no day (nil), and a sort by date lists it last.
func releasedOn(year int64, date string) *int64 {
	if year <= 0 {
		return nil
	}
	day := year * 10_000
	parts := strings.SplitN(strings.TrimSpace(date), "-", 3)
	if dateYear, err := strconv.ParseInt(parts[0], 10, 64); err != nil || dateYear != year || len(parts) < 2 {
		return &day
	}
	month := numberWithin(parts[1], 12)
	if month == 0 {
		return &day
	}
	day += month * 100
	if len(parts) == 3 {
		day += numberWithin(leadingDigits(parts[2]), 31)
	}
	return &day
}

// numberWithin is text's number when it is from 1 to most, else 0.
func numberWithin(text string, most int64) int64 {
	number, err := strconv.ParseInt(text, 10, 64)
	if err != nil || number < 1 || number > most {
		return 0
	}
	return number
}

// leadingDigits is text up to its first character that is not a digit: a day with a time after it
// ("15T00:00:00") is its day.
func leadingDigits(text string) string {
	end := strings.IndexFunc(text, func(character rune) bool { return character < '0' || character > '9' })
	if end < 0 {
		return text
	}
	return text[:end]
}
