// SPDX-License-Identifier: GPL-3.0-or-later

package tmdb

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/artwork"
	"github.com/kstenerud/orpheum/internal/platform/outbound"
)

func TestImageFetcher_FetchesOnlyTMDBsImagePathsFromItsImageServer(t *testing.T) {
	t.Parallel()
	server, seen := fakeTMDB(t, map[string]string{"/qJ2tW6WMUDux911r6m7haRef0WH.jpg": "not really a JPEG"})
	httpClient, err := outbound.NewClient(5*time.Second, "test")
	if err != nil {
		t.Fatal(err)
	}
	images := NewImageFetcher(httpClient, server.URL)
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

func TestImageFetcher_TurnsTheImageServersRefusalsIntoTheErrorsTheAPIsAre(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/limited.jpg":
			writer.Header().Set("Retry-After", "7")
			writer.WriteHeader(http.StatusTooManyRequests)
		case "/unsaid.jpg":
			writer.WriteHeader(http.StatusTooManyRequests)
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(server.Close)
	httpClient, err := outbound.NewClient(5*time.Second, "test")
	if err != nil {
		t.Fatal(err)
	}
	images := NewImageFetcher(httpClient, server.URL)
	for ref, want := range map[string]time.Duration{"/limited.jpg": 7 * time.Second, "/unsaid.jpg": time.Second} {
		_, err := images.Fetch(t.Context(), ref)
		if limited, isLimited := errors.AsType[*outbound.RateLimitedError](err); !isLimited ||
			limited.RetryAfter != want || limited.Provider != "TMDB" {
			t.Errorf("a 429 for %s: %v, want TMDB's RateLimitedError after %s", ref, err, want)
		}
	}
}

func TestImageFetcher_A503IsTheHostDownAndA500TheImagesOwnFailure(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/down.jpg" {
			writer.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		writer.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	httpClient, err := outbound.NewClient(5*time.Second, "test")
	if err != nil {
		t.Fatal(err)
	}
	images := NewImageFetcher(httpClient, server.URL)
	// Neither says the image is gone (D231 §5). Each names its status.
	for ref, status := range map[string]string{"/broken.jpg": "500", "/down.jpg": "503"} {
		_, err := images.Fetch(t.Context(), ref)
		if err == nil || errors.Is(err, outbound.ErrUnavailable) != (status == "503") ||
			!strings.Contains(err.Error(), status) || errors.Is(err, artwork.ErrNoImage) {
			t.Errorf("a %s: %v, want the host unavailable for a 503 only, naming the status", status, err)
		}
	}
}

func TestImageFetcher_AnAnswerThatIsNotAnImageIsTheHostFailing(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = writer.Write([]byte("<html>Sign in to the hotel's Wi-Fi</html>")) //nolint:errcheck // a test server
	}))
	t.Cleanup(server.Close)
	httpClient, err := outbound.NewClient(5*time.Second, "test")
	if err != nil {
		t.Fatal(err)
	}
	image, err := NewImageFetcher(httpClient, server.URL).Fetch(t.Context(), "/portal.jpg")
	if !errors.Is(err, outbound.ErrUnavailable) || errors.Is(err, artwork.ErrNoImage) || image.Len() != 0 {
		t.Errorf("a 200 that is a web page: %v, want the host failing, not the image missing (D213 §5)", err)
	}
}
