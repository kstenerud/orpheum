// SPDX-License-Identifier: GPL-3.0-or-later

package outbound

import (
	"net/http"
	"testing"
	"time"
)

func TestRetryAfter_ReadsSecondsOrADateAgainstTheReplysOwnClockAndCapsAtAnHour(t *testing.T) {
	t.Parallel()
	for _, check := range []struct {
		value, date string
		want        time.Duration
		found       bool
	}{
		{"120", "", 2 * time.Minute, true},
		{"86400", "", time.Hour, true},
		{"Fri, 25 Sep 2026 12:05:00 GMT", "Fri, 25 Sep 2026 12:00:00 GMT", 5 * time.Minute, true},
		{"Fri, 25 Sep 2026 11:00:00 GMT", "Fri, 25 Sep 2026 12:00:00 GMT", 0, true},
		{"Fri, 25 Sep 2026 12:05:00 GMT", "", 0, false},
		{"soon", "", 0, false},
		{"-5", "", 0, false},
	} {
		header := http.Header{}
		header.Set("Retry-After", check.value)
		if check.date != "" {
			header.Set("Date", check.date)
		}
		if after, found := RetryAfter(header); after != check.want || found != check.found {
			t.Errorf("Retry-After %q (Date %q) = %v, %v; want %v, %v", check.value, check.date, after, found,
				check.want, check.found)
		}
	}
}
