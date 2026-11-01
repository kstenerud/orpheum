// SPDX-License-Identifier: AGPL-3.0-or-later

package rtcapi_test

import (
	"net/http"
	"regexp"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/rtcapi"
	"github.com/kstenerud/orpheum/internal/core/rtcapi/rtcapitest"
)

// A client's video m-line, offered so Chrome grows its socket buffer, is answered with a port and
// VP8 alone, and the core sends nothing on it: no track arrives while a response does (D397).
func TestVideo_AClientsVideoLineIsAnsweredAndNothingSentOnIt(t *testing.T) {
	t.Parallel()
	link := rtcapitest.Connect(t, newServer(t).Answer)
	answer := link.AnswerSDP()
	line := regexp.MustCompile(`(?m)^m=video (\d+) `).FindStringSubmatch(answer)
	if len(line) < 2 || line[1] == "0" {
		t.Fatalf("the answer refused the client's video m-line; want it answered:\n%s", answer)
	}
	// VP8 alone, which every browser offers: pion's default engine answers with every codec it knows.
	codecs := regexp.MustCompile(`(?m)^a=rtpmap:\d+ ([^/\r\n]+)/`).FindAllStringSubmatch(answer, -1)
	if len(codecs) != 1 || codecs[0][1] != "VP8" {
		t.Errorf("the answer's video m-line names %v; want VP8 alone:\n%s", codecs, answer)
	}
	// pion answers a recvonly m-line sendonly, the only answer it makes to one, with no source.
	if !regexp.MustCompile(`(?m)^a=sendonly\r?$`).MatchString(answer) {
		t.Errorf("the answer's video m-line is not sendonly:\n%s", answer)
	}
	if regexp.MustCompile(`(?m)^a=(ssrc|msid):`).MatchString(answer) {
		t.Errorf("the answer names a video source; want none:\n%s", answer)
	}
	link.Do(rtcapi.RequestHead{Method: http.MethodGet, Path: "/big"}, nil)
	if tracks := link.Tracks(); tracks != 0 {
		t.Errorf("the core sent %d tracks; want none", tracks)
	}
}
