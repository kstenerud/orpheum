// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"net/http"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/testutil"
)

// The server wires D309 §8 end to end: a player's first chunk of a quality makes it the one it
// wants, which the answers to the node's poll then say is in use, and the node keeps its stores by
// it. That its runs follow it is internal/core/api's TestInterest_ARunOfAQualityThePlayerLeftStops,
// over the playback service that wires it.
func TestPlayback_TheNodeKeepsTheChunkStoresByWhatThePlayersWant(t *testing.T) {
	t.Parallel()
	const nodeKept = "chunkStoresKept"
	captured := newCapturedLog("ready", filmIdentified, nodeKept)
	film := cutFilmPlaying(t, func(t *testing.T, shelved filmFixture) (runningServer, string, int64) {
		t.Helper()
		return playableFilmLogged(t, shelved, captured)
	})
	root := strings.TrimSuffix(film.server.baseURL, "/api/v1")
	chunks := root + "/api/v1/bytes/media/chunks/" + film.playing.MediaToken + "/"
	requestAPI(t, http.MethodGet, chunks+"picture/00001.m4s", film.token, nil)
	testutil.Receive(t, captured.signals[nodeKept], "the node to keep the stores by the players' tiers")
}
