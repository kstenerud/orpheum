// SPDX-License-Identifier: AGPL-3.0-or-later

package identify

// This package finds the images an item has — the ones its provider names and the ones beside
// the media — and reports them; it never writes the artwork table. The server's wiring hands the
// report to the artwork service, which holds them and fetches what is owed (schema.md, "Who
// writes each table").

// NamedImages are the images one item's source names (D212).
type NamedImages struct {
	ItemID int64
	Source string
	Refs   []ImageRef
}

// LocalImage is an image found beside the media for an item (D215): its kind, and its file with
// the size and modification time by which a changed file is owed again.
type LocalImage struct {
	ItemID       int64
	Kind         string
	FileID       int64
	SizeBytes    int64
	ModifiedAtMs int64
}

// LocalImages are every image a pass found beside one library's media.
type LocalImages struct {
	LibraryID int64
	Images    []LocalImage
}

// reportImages reports the images an item's source names, every one: which are owed, where the
// library prefers images beside the media, is the artwork service's to say, for a pass and a
// match alike (D215 §1). An NFO names none of its own, and an item with no source has none.
func (filer *libraryFiler) reportImages(itemID int64, source string, refs []ImageRef) {
	if len(refs) == 0 || source == "" || source == sourceNFO || source == sourceNone {
		return
	}
	filer.report.NamedImages = append(filer.report.NamedImages, NamedImages{ItemID: itemID, Source: source, Refs: refs})
}
