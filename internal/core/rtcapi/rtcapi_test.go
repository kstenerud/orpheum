// SPDX-License-Identifier: AGPL-3.0-or-later

package rtcapi_test

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/rtcapi"
	"github.com/kstenerud/orpheum/internal/core/rtcapi/rtcapitest"
)

// echo answers each request with what it was asked: its method, path, header, body and address.
var echo = http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
	body, err := io.ReadAll(request.Body)
	if err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}
	if request.URL.Path == "/big" {
		writer.Header().Set("Content-Type", "application/octet-stream")
		_, _ = writer.Write(bytes.Repeat([]byte("orpheum!"), 5<<20/8)) //nolint:errcheck // the test reads it
		return
	}
	writer.Header().Set("X-Echo", request.Header.Get("Authorization"))
	writer.WriteHeader(http.StatusTeapot)
	//nolint:errcheck // the test reads it
	_, _ = io.WriteString(writer, strings.Join([]string{
		request.Method, request.URL.RequestURI(), string(body),
		request.RemoteAddr,
	}, "|"))
})

func newServer(t *testing.T) *rtcapi.Server {
	t.Helper()
	server := rtcapi.New(t.Context(), echo, slog.New(slog.DiscardHandler))
	t.Cleanup(func() { _ = server.Close() }) //nolint:errcheck // the test is over
	return server
}

// The header is named in lower case, as a browser names it, and the handler reads it by its canonical
// name.
func TestChannel_ARequestReachesTheHandlerAndItsResponseComesBackWhole(t *testing.T) {
	t.Parallel()
	// Both ends on the loopback, which no container network's masquerade rewrites, so the address the
	// server sees is the one the client holds.
	server := newServer(t)
	rtcapi.AnswerOnLoopback(server)
	link := rtcapitest.Connect(t, server.Answer, rtcapitest.OnLoopback())
	got := link.Do(rtcapi.RequestHead{
		Method: http.MethodPost, Path: "/api/v1/things?x=1",
		Headers: http.Header{"authorization": {"Bearer t"}},
	}, []byte(`{"a":1}`))
	parts := strings.Split(string(got.Body), "|")
	if got.Head.Status != http.StatusTeapot || got.Head.Headers.Get("X-Echo") != "Bearer t" || len(parts) != 4 {
		t.Fatalf("the response was %+v %q", got.Head, got.Body)
	}
	if parts[0] != http.MethodPost || parts[1] != "/api/v1/things?x=1" || parts[2] != `{"a":1}` {
		t.Errorf("the handler saw %q; want the method, the path and query, and the body as sent", parts[:3])
	}
	if parts[3] != link.Address() {
		t.Errorf("the request's remote address is %q, want the client's end of the connection, %q", parts[3],
			link.Address())
	}
}

func TestChannel_ALargeResponseArrivesWholeInMessagesABrowserCarries(t *testing.T) {
	t.Parallel()
	got := rtcapitest.Connect(t, newServer(t).Answer).Do(rtcapi.RequestHead{Method: http.MethodGet, Path: "/big"}, nil)
	want := bytes.Repeat([]byte("orpheum!"), 5<<20/8)
	if got.Head.Status != http.StatusOK || !bytes.Equal(got.Body, want) {
		t.Errorf("a 5 MiB response arrived as %d bytes with status %d; want it whole", len(got.Body), got.Head.Status)
	}
	if got.LargestMessageBytes > rtcapi.MaxMessageBytes {
		t.Errorf("a message held %d bytes, over %d", got.LargestMessageBytes, rtcapi.MaxMessageBytes)
	}
}

func TestChannel_RequestsOnTheirOwnChannelsAreAnsweredAtOnce(t *testing.T) {
	t.Parallel()
	link := rtcapitest.Connect(t, newServer(t).Answer)
	var waiting sync.WaitGroup
	paths := []string{"/one", "/two", "/three", "/big"}
	got := make([]rtcapitest.Reply, len(paths))
	for index, path := range paths {
		waiting.Go(func() { got[index] = link.Do(rtcapi.RequestHead{Method: http.MethodGet, Path: path}, nil) })
	}
	waiting.Wait()
	for index, path := range paths[:3] {
		if !strings.Contains(string(got[index].Body), "|"+path+"|") {
			t.Errorf("%s was answered %q", path, got[index].Body)
		}
	}
}
