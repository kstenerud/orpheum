// SPDX-License-Identifier: GPL-3.0-or-later

package identify

import (
	"testing"

	"github.com/kstenerud/orpheum/internal/platform/ptr"
)

// D162's second evidence: named for its folder's title with a vocabulary suffix. The suffix alone
// never makes an extra, and nothing at a library root is one.
func TestPass_AFileNamedForItsFoldersTitleWithASuffixIsAnExtra(t *testing.T) {
	t.Parallel()
	lib := newWorld(t, "movies", "nfo")
	report := lib.scan(
		video("Quiet Harbour (1999)/Quiet Harbour (1999).mkv"),
		video("Quiet Harbour (1999)/Quiet Harbour (1999)-featurette.mkv"),
		video("Quiet Harbour (1999)/Something Else-trailer.mkv"),
		video("Loose Ends-trailer.mkv"),
	)
	files := lib.files()
	film := files["Quiet Harbour (1999)/Quiet Harbour (1999).mkv"]
	featurette := files["Quiet Harbour (1999)/Quiet Harbour (1999)-featurette.mkv"]
	if featurette.Kind != kindExtra || featurette.TitleID == film.TitleID || featurette.Filing != filingExtra ||
		ptr.ValueOrZero(featurette.FilingReason) != "featurette of Quiet Harbour (1999), by its name" {
		t.Errorf("the featurette is %+v (%s); want an extra of the film, by its name", featurette,
			ptr.ValueOrZero(featurette.FilingReason))
	}
	for _, rel := range []string{"Quiet Harbour (1999)/Something Else-trailer.mkv", "Loose Ends-trailer.mkv"} {
		if file := files[rel]; file.Kind == kindExtra || file.Filing == filingExtra {
			t.Errorf("%s is %+v; a suffix alone, or at the root, makes no extra", rel, file)
		}
	}
	if report.Extras != 1 {
		t.Errorf("report %+v, want one extra", report)
	}
}
