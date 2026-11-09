// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"slices"

	"github.com/kstenerud/orpheum/internal/core/library"
)

// kindFilm is a film's kind, and a film library's, as the API says it: every key says film
// (filmMs, filmOfExtra, filmsWithoutSound), so the kind does too (the owner, 2026-11-09). The
// library keeps "movie" (library.TitleKindMovie, library.LibraryKindMovie) in the database and the
// seed file, so the API turns one into the other where a kind crosses it, and nowhere else.
const kindFilm = "film"

// saidKind is kept, a kind as the library keeps it, as the API says it.
func saidKind(kept string) string {
	if kept == library.TitleKindMovie {
		return kindFilm
	}
	return kept
}

// keptKind is said, a kind as the API says it, as the library keeps it.
func keptKind(said string) string {
	if said == kindFilm {
		return library.TitleKindMovie
	}
	return said
}

// keptKinds is each of said as the library keeps it.
func keptKinds(said []string) []string {
	kept := slices.Clone(said)
	for index, each := range kept {
		kept[index] = keptKind(each)
	}
	return kept
}
