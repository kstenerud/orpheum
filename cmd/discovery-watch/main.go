// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/kstenerud/orpheum/discovery/addressdoc"
	"github.com/kstenerud/orpheum/discovery/release"
	"github.com/kstenerud/orpheum/discovery/trust"
	"github.com/kstenerud/orpheum/internal/platform/outbound"
)

// Exit statuses: everything expected, something not, and a usage error.
const (
	exitExpected   = 0
	exitUnexpected = 1
	exitUsage      = 2
)

// fetchTimeout bounds each fetch.
const fetchTimeout = 30 * time.Second

// maxSignedBytes bounds a signed file and its signature as fetched.
const maxSignedBytes = 128 << 10

// maxVersions bounds how many root statements of one chain it fetches.
const maxVersions = 4096

func main() {
	client, err := outbound.NewClient(fetchTimeout, "discovery-watch")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(exitUsage)
	}
	os.Exit(run(context.Background(), os.Args[1:], client, os.Stdout, os.Stderr))
}

// run fetches every signed object at every place its arguments name and reports each signature the
// owner's ledger does not hold, and each place that did not answer; it returns the exit status.
func run(ctx context.Context, arguments []string, client *http.Client, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("discovery-watch", flag.ContinueOnError)
	flags.SetOutput(stderr)
	expectedFile := flags.String("expected", "", "the ledger's expected file, which discovery-sign writes (required)")
	var places []string
	flags.Func("place", "a place a chain, a document or a record is served under; repeated", func(place string) error {
		places = append(places, strings.TrimSuffix(place, "/"))
		return nil
	})
	if err := flags.Parse(arguments); err != nil || *expectedFile == "" || len(places) == 0 || flags.NArg() != 0 {
		//nolint:errcheck // a terminal
		fmt.Fprintln(stderr, "usage: discovery-watch --expected <ledger>/expected --place <url>...")
		return exitUsage
	}
	expected, err := trust.ReadExpected(*expectedFile)
	if err != nil {
		fmt.Fprintln(stderr, err) //nolint:errcheck // a terminal
		return exitUsage
	}
	watcher := watch{ctx: ctx, client: client, expected: expected}
	for _, place := range places {
		watcher.place(place)
	}
	if len(watcher.reports) == 0 {
		//nolint:errcheck // a terminal
		fmt.Fprintf(stdout, "every signature at %d places is the owner's\n", len(places))
		return exitExpected
	}
	fmt.Fprintln(stdout, strings.Join(watcher.reports, "\n")) //nolint:errcheck // a terminal
	return exitUnexpected
}

// watch is one run: the ledger's signatures, and what it found unexpected.
type watch struct {
	ctx      context.Context //nolint:containedctx // one run's, passed down by run
	client   *http.Client
	expected map[trust.Expected]bool
	reports  []string
}

// place checks every signed object served under place: both chains, every version, the address
// document and the release record.
func (watcher *watch) place(place string) {
	for _, chain := range []struct{ prefix, latest string }{
		{addressdoc.RootPathPrefix, addressdoc.RootLatestPath},
		{release.PublisherRootPathPrefix, release.PublisherRootLatestPath},
	} {
		latest, isFound := watcher.check(place + chain.latest)
		if !isFound {
			continue
		}
		root, err := trust.ParseRoot([]byte(latest.Payload))
		if err != nil {
			watcher.reportf("%s: not a root statement: %v", place+chain.latest, err)
			continue
		}
		for version := int64(1); version <= root.Version && version <= maxVersions; version++ {
			watcher.check(fmt.Sprintf("%s%s%d", place, chain.prefix, version))
		}
	}
	watcher.check(place + addressdoc.DocumentPath)
	watcher.check(place + release.RecordPath)
}

// check fetches the signed object at url and reports each of its signatures the ledger does not
// hold, by its key and counter; whether one is served.
func (watcher *watch) check(url string) (trust.Signed, bool) {
	payload, isFound := watcher.fetch(url)
	if !isFound {
		return trust.Signed{}, false
	}
	signature, isSigned := watcher.fetch(url + addressdoc.SignatureEnd)
	if !isSigned {
		watcher.reportf("%s: served with no signature", url)
		return trust.Signed{}, false
	}
	signed := trust.Signed{Payload: payload, Signature: signature}
	found, err := trust.ExpectedOf(signed)
	if err != nil {
		watcher.reportf("%s: its signature cannot be read: %v", url, err)
		return signed, true
	}
	for _, each := range found {
		if !watcher.expected[each] {
			watcher.reportf("%s: a signature the ledger does not hold, by %s, counter %d", url, each.Fingerprint,
				each.Counter)
		}
	}
	return signed, true
}

// fetch is the body at url, and whether it is served; anything but a not-found that fails is reported.
func (watcher *watch) fetch(url string) (string, bool) {
	request, err := http.NewRequestWithContext(watcher.ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		watcher.reportf("%s: %v", url, err)
		return "", false
	}
	response, err := watcher.client.Do(request)
	if err != nil {
		watcher.reportf("%s: did not answer: %v", url, err)
		return "", false
	}
	defer func() { _ = response.Body.Close() }() //nolint:errcheck // read, and closed
	if response.StatusCode == http.StatusNotFound {
		return "", false
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxSignedBytes))
	if err != nil || response.StatusCode != http.StatusOK {
		watcher.reportf("%s: answered %s", url, response.Status)
		return "", false
	}
	return string(body), true
}

// reportf adds a line to the report.
func (watcher *watch) reportf(format string, values ...any) {
	watcher.reports = append(watcher.reports, fmt.Sprintf(format, values...))
}
