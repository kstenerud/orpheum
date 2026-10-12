// SPDX-License-Identifier: AGPL-3.0-or-later

package playback

import "sync"

// pictureOfFilm is a player, as playerOf names it, and a film it played.
type pictureOfFilm struct {
	player string
	fileID int64
}

// unplayablePictures are the films whose own picture each player gave up as one its device cannot
// play (D266), so that its later playbacks of the film are not offered it: Firefox on a Mac gave up
// Flow's own picture anew at every playback, 1.5 to 5 s each (the owner's test, 2026-10-12). Kept in
// memory, for the server's life: a browser's update may come to play it, and a restart tries again.
type unplayablePictures struct {
	mutex sync.Mutex
	// films are those given up; guarded by mutex.
	films map[pictureOfFilm]bool
}

func newUnplayablePictures() *unplayablePictures {
	return &unplayablePictures{films: map[pictureOfFilm]bool{}}
}

// remember keeps that player gave up fileID's own picture.
func (pictures *unplayablePictures) remember(player string, fileID int64) {
	pictures.mutex.Lock()
	defer pictures.mutex.Unlock()
	pictures.films[pictureOfFilm{player: player, fileID: fileID}] = true
}

// has is whether player gave up fileID's own picture.
func (pictures *unplayablePictures) has(player string, fileID int64) bool {
	pictures.mutex.Lock()
	defer pictures.mutex.Unlock()
	return pictures.films[pictureOfFilm{player: player, fileID: fileID}]
}
