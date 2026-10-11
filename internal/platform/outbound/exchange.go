// SPDX-License-Identifier: AGPL-3.0-or-later

package outbound

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/kstenerud/orpheum/internal/platform/bounded"
)

// ErrUnavailable is a provider that could not be reached or answered that it is down: a refused
// connection, DNS, a timeout, 502, 503 or 504. It says nothing of what was asked, so asking later
// may succeed (D231 §5). A plain 500 is not it: that is the provider failing on what was asked.
var ErrUnavailable = errors.New("unavailable")

// Answer is a provider's answer, its body read whole.
type Answer struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}

// BodyExcerptBytes is how much of an answer's body an error quotes: enough for a refusal's own
// words, which are a line, and little of a page a proxy or a crash put there instead.
const BodyExcerptBytes = 200

// Exchange sends request through client and reads its answer's body, at most maxBytes of it: a
// longer one is bounded.ErrTooLarge, never read cut short. Every provider's request goes through
// it. A request that reached no answer, or lost it partway, is ErrUnavailable; its error keeps
// only the cause, since url.Error repeats the URL and a URL may carry a key. A request whose own
// context ended is that context's error, not the provider's.
func Exchange(client *http.Client, request *http.Request, maxBytes int64) (Answer, error) {
	// Each caller builds the address from its provider's base, or checks the host it was given.
	response, err := client.Do(request) //nolint:gosec // G704: see above
	if err != nil {
		return Answer{}, unanswered(request, err)
	}
	defer func() { _ = response.Body.Close() }() //nolint:errcheck // a read-only body
	body, err := bounded.ReadAll(response.Body, maxBytes)
	if errors.Is(err, bounded.ErrTooLarge) {
		return Answer{}, fmt.Errorf("the answer: %w", err)
	}
	if err != nil {
		return Answer{}, unanswered(request, err)
	}
	return Answer{StatusCode: response.StatusCode, Header: response.Header, Body: body}, nil
}

// unanswered is a request that got no whole answer, without the URL url.Error repeats.
func unanswered(request *http.Request, err error) error {
	if urlErr, isURL := errors.AsType[*url.Error](err); isURL {
		err = urlErr.Err
	}
	if request.Context().Err() != nil {
		return err
	}
	return fmt.Errorf("%w: %w", ErrUnavailable, err)
}

// StatusError is what an answer that is not 200 means once its provider has read the statuses it
// gives a meaning of its own: 429 is a RateLimitedError, 502, 503 and 504 are ErrUnavailable (a
// gateway or the service down), and anything else, a plain 500 included, is its status: a failure
// of what was asked (D231 §5). An error quotes the start of the body when there is one. The
// provider is named as a person knows it: "TMDB", "OpenSubtitles".
func StatusError(provider string, answer Answer) error {
	switch answer.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusTooManyRequests:
		return RateLimited(provider, answer.Header)
	}
	said := fmt.Sprintf("status %d", answer.StatusCode)
	if len(answer.Body) > 0 {
		said += ": " + string(answer.Body[:min(len(answer.Body), BodyExcerptBytes)])
	}
	switch answer.StatusCode {
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return fmt.Errorf("%w: %s", ErrUnavailable, said)
	}
	return errors.New(said)
}
