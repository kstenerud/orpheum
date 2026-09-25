// SPDX-License-Identifier: GPL-3.0-or-later

package tmdb

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/artwork"
	"github.com/kstenerud/orpheum/internal/platform/outbound"
)

func TestImages_FetchesOnlyTMDBsImagePathsFromItsImageServer(t *testing.T) {
	t.Parallel()
	server, seen := fake(t, map[string]string{"/qJ2tW6WMUDux911r6m7haRef0WH.jpg": "not really a JPEG"})
	httpClient, err := outbound.NewClient(5*time.Second, "test")
	if err != nil {
		t.Fatal(err)
	}
	images := NewImages(httpClient, server.URL)
	image, err := images.Fetch(t.Context(), "/qJ2tW6WMUDux911r6m7haRef0WH.jpg")
	if err != nil || image.Len() != len("not really a JPEG") {
		t.Fatalf("Fetch = %d bytes, %v", image.Len(), err)
	}
	if _, err := images.Fetch(t.Context(), "/gone.jpg"); !errors.Is(err, artwork.ErrNoImage) {
		t.Errorf("a 404: %v, want ErrNoImage", err)
	}
	asked := len(*seen)
	for _, ref := range []string{
		"/../3/configuration.jpg", "/a/b.jpg", "//evil.example/x.jpg", "http://evil.example/x.jpg",
		"/x.jpg?api_key=1", "/x.svg", "x.jpg", "",
	} {
		if _, err := images.Fetch(t.Context(), ref); !errors.Is(err, ErrBadImageRef) ||
			!errors.Is(err, artwork.ErrNoImage) {
			t.Errorf("Fetch(%q) = %v, want ErrBadImageRef", ref, err)
		}
	}
	if len(*seen) != asked {
		t.Errorf("a reference not shaped like TMDB's was requested: %v", (*seen)[asked:])
	}
}

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
		if after, found := retryAfter(header); after != check.want || found != check.found {
			t.Errorf("Retry-After %q (Date %q) = %v, %v; want %v, %v", check.value, check.date, after, found,
				check.want, check.found)
		}
	}
}
