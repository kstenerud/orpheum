// SPDX-License-Identifier: AGPL-3.0-or-later

package identify

import (
	"context"
	"fmt"
)

// FilmWithoutSound is a film whose video file was read and holds a picture but no sound: a video-only
// download whose sound was never merged in, as Mister 517's was (the owner, 2026-11-02).
type FilmWithoutSound struct {
	TitleID int64
	Name    string
}

// filmsWithoutSoundRead bounds how many films' files a look lists.
const filmsWithoutSoundRead = 500

// FilmsWithoutSound are the live films with no audio track, in the order of their ids, one for each
// film however many of its files lack sound, and how many there are in all, which the list may fall
// short of (it reads at most filmsWithoutSoundRead files); the count is its own query's (D432 §4).
func (chooser *Chooser) FilmsWithoutSound(ctx context.Context) ([]FilmWithoutSound, int, error) {
	reader := chooser.matcher.database.Read()
	rows, err := reader.ListFilmsWithoutSound(ctx, filmsWithoutSoundRead)
	if err != nil {
		return nil, 0, fmt.Errorf("list the films with no sound: %w", err)
	}
	total, err := reader.CountFilmsWithoutSound(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count the films with no sound: %w", err)
	}
	films := make([]FilmWithoutSound, 0, len(rows))
	for _, row := range rows {
		if last := len(films) - 1; last >= 0 && films[last].TitleID == row.WorkID {
			continue
		}
		films = append(films, FilmWithoutSound{TitleID: row.WorkID, Name: row.WorkName})
	}
	return films, int(total), nil
}
