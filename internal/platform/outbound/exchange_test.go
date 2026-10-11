// SPDX-License-Identifier: AGPL-3.0-or-later

package outbound

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/bounded"
)

// exchangeWith sends a GET of path to server through a client of this package.
func exchangeWith(ctx context.Context, t *testing.T, server, path string, maxBytes int64) (Answer, error) {
	t.Helper()
	client, err := NewClient(5*time.Second, "test")
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server+path, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	return Exchange(client, request, maxBytes)
}

func TestExchange_AnUnansweredRequestIsUnavailableAndDoesNotRepeatItsURL(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.NotFoundHandler())
	server.Close()
	_, err := exchangeWith(t.Context(), t, server.URL, "/x?api_key=the-secret-key", 64)
	if !errors.Is(err, ErrUnavailable) || strings.Contains(err.Error(), "the-secret-key") {
		t.Errorf("a refused connection: %v, want ErrUnavailable without the URL", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := exchangeWith(ctx, t, server.URL, "/x", 64); !errors.Is(err, context.Canceled) ||
		errors.Is(err, ErrUnavailable) {
		t.Errorf("a request its caller cancelled: %v, want its context's error, not the provider's", err)
	}
}

func TestExchange_ReadsTheWholeAnswerOrRefusesOneLargerThanItsBound(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("X-Said", "yes")
		writer.WriteHeader(http.StatusTeapot)
		_, _ = writer.Write([]byte("0123456789")) //nolint:errcheck // a test server
	}))
	t.Cleanup(server.Close)
	answer, err := exchangeWith(t.Context(), t, server.URL, "/", 10)
	if err != nil || answer.StatusCode != http.StatusTeapot || string(answer.Body) != "0123456789" ||
		answer.Header.Get("X-Said") != "yes" {
		t.Errorf("an answer within its bound: %+v, %v", answer, err)
	}
	if _, err := exchangeWith(t.Context(), t, server.URL, "/", 9); !errors.Is(err, bounded.ErrTooLarge) {
		t.Errorf("an answer past its bound: %v, want ErrTooLarge", err)
	}
}

func TestStatusError_NamesA429AsARateLimitA502To504AsUnavailableAndA500AsTheRequestsOwn(t *testing.T) {
	t.Parallel()
	if err := StatusError("TMDB", Answer{StatusCode: http.StatusOK}); err != nil {
		t.Errorf("200: %v, want nil", err)
	}
	limited, isLimited := errors.AsType[*RateLimitedError](StatusError("TMDB", Answer{
		StatusCode: http.StatusTooManyRequests, Header: http.Header{"Retry-After": {"7"}},
	}))
	if !isLimited || limited.Provider != "TMDB" || limited.RetryAfter != 7*time.Second {
		t.Errorf("429: %v, want TMDB's RateLimitedError after 7s", limited)
	}
	for _, status := range []int{http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout} {
		err := StatusError("TMDB", Answer{StatusCode: status, Body: []byte("down for maintenance")})
		if !errors.Is(err, ErrUnavailable) || !strings.Contains(err.Error(), "down for maintenance") {
			t.Errorf("%d: %v, want ErrUnavailable quoting the body", status, err)
		}
	}
	// A plain 500 is the provider failing on what was asked, not down (D231 §5).
	err := StatusError("TMDB", Answer{StatusCode: http.StatusInternalServerError, Body: []byte("oops")})
	if err == nil || errors.Is(err, ErrUnavailable) || !strings.Contains(err.Error(), "500: oops") {
		t.Errorf("500: %v, want its status, not unavailable", err)
	}
	if err := StatusError("TMDB", Answer{StatusCode: http.StatusBadRequest}); err == nil ||
		errors.Is(err, ErrUnavailable) || !strings.Contains(err.Error(), "400") {
		t.Errorf("400: %v, want its status, not unavailable", err)
	}
}
