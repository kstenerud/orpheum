// SPDX-License-Identifier: GPL-3.0-or-later

// Package outbound builds the HTTP client every request the server makes to another host goes
// through: TMDB, OpenSubtitles, the update check. Go's default client and transport take a proxy
// from HTTP_PROXY and HTTPS_PROXY, which is configuration nobody gave the server; this one takes
// none (D204). If a proxy is ever needed, it becomes a config-file key passed in here.
//
// # Invariants
//
//   - A client from NewClient uses no proxy, whatever the environment says, and says it is
//     Orpheum in every request's User-Agent (D213 §6). TestNewClient_TakesNoProxyFromTheEnvironment.
//   - No other package uses http.DefaultClient, http.DefaultTransport, http.Get and its
//     siblings, or http.ProxyFromEnvironment. Enforced by the forbidigo rule tagged [outbound].
//   - A provider's 429 is one RateLimitedError whichever provider answered it, with the wait its
//     Retry-After names or a second when it names none.
//     TestRateLimited_WaitsWhatRetryAfterSaysOrASecondWhenItSaysNothing.
//   - Every provider's request is sent and read by Exchange, and its statuses read last by
//     StatusError: a refused connection, DNS, a timeout, 502, 503 or 504 is ErrUnavailable,
//     whichever provider, a plain 500 the request's own failure (D231 §5),
//     and an error never repeats the URL, which may carry a key; an answer past its bound is
//     refused, never read cut short. TestExchange_AnUnansweredRequestIsUnavailableAndDoesNotRepeatItsURL,
//     TestExchange_ReadsTheWholeAnswerOrRefusesOneLargerThanItsBound,
//     TestStatusError_NamesA429AsARateLimitA502To504AsUnavailableAndA500AsTheRequestsOwn.
//   - This package imports nothing from the module but internal/platform/bounded. Enforced by the
//     depguard rule platform-is-a-leaf.
//
// # Read before changing
//
//   - D204: configuration is explicit and contained.
//   - docs/contributors/standards/go.md §12.
//
// # Where to change
//
//   - client.go: transport settings every outbound request shares. exchange.go: sending a
//     provider's request, reading its answer and the statuses every provider shares.
//     retryafter.go: reading a provider's Retry-After, which every provider client shares.
//     ratelimited.go: the one error a provider's 429 becomes.
package outbound
