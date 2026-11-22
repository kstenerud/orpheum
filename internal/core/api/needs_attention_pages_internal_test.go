// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/discovery"
)

// D538 §12: a page lists its problems before its notices, keeping each group's order, at most 100
// of them, itemCount saying how many in all; a page with no item is absent.
func TestAttentionPage_ListsItsProblemsFirstAndAHundredAtMost(t *testing.T) {
	if body := (attentionPage{}).body(); body != nil {
		t.Errorf("an empty page = %+v; want it absent", body)
	}
	page := attentionPage{{Kind: attentionNeedsMatch, Count: 1}, {Kind: attentionWorkFailing, IsProblem: true}}
	for range attentionListedAtMost {
		page = append(page, AttentionItemBody{Kind: attentionDeadEnds})
	}
	page = append(page, AttentionItemBody{Kind: attentionRemoved, IsProblem: true})
	body := page.body()
	if !body.IsProblem || body.ItemCount != attentionListedAtMost+3 || len(body.Items) != attentionListedAtMost {
		t.Fatalf("the page is a problem %v with %d items of %d; want a problem, %d of %d",
			body.IsProblem, len(body.Items), body.ItemCount, attentionListedAtMost, attentionListedAtMost+3)
	}
	if got := body.Items[:3]; got[0].Kind != attentionWorkFailing || got[1].Kind != attentionRemoved ||
		got[2].Kind != attentionNeedsMatch {
		t.Errorf("the page begins %+v; want work failing, removed, then the notice needing a match", got)
	}
	if notices := (attentionPage{{Kind: attentionDeadEnds, Count: 2}}).body(); notices.IsProblem {
		t.Errorf("a page of notices = %+v; want it no problem", notices)
	}
}

// D538 §12: the Network page's items are what the discovery's status says the Network page shows:
// standing aside, the addresses expired and the identity removed are problems; a copy refused, the
// dead ends and the protocol ending are notices; a discovery with nothing to say makes none.
func TestAttentionPages_NetworkListsWhatTheDiscoverySaysNeedsTheAdmin(t *testing.T) {
	if items := networkAttention(discovery.Status{}); len(items) != 0 {
		t.Errorf("a discovery with nothing to say makes %+v; want nothing", items)
	}
	moment := time.UnixMilli(1_790_000_000_000)
	items := networkAttention(discovery.Status{
		StoodAsideAt: moment, CopyRefusedFrom: "203.0.113.9", CopyRefusedAt: moment, DocumentStaleSince: moment,
		RemovedAt: moment, DeadEndsLastWeek: 3, DeprecatedUntil: "2027-01-01",
	})
	want := []AttentionItemBody{
		{Kind: attentionStoodAside, IsProblem: true, Since: moment.UnixMilli()},
		{Kind: attentionCopyRefused, Since: moment.UnixMilli(), Name: "203.0.113.9"},
		{Kind: attentionAddressesExpired, IsProblem: true, Since: moment.UnixMilli()},
		{Kind: attentionRemoved, IsProblem: true, Since: moment.UnixMilli()},
		{Kind: attentionDeadEnds, Count: 3},
		{Kind: attentionProtocolEnding, Until: "2027-01-01"},
	}
	if len(items) != len(want) {
		t.Fatalf("the discovery's status makes %+v; want %+v", items, want)
	}
	for index := range want {
		if items[index] != want[index] {
			t.Errorf("item %d = %+v; want %+v", index, items[index], want[index])
		}
	}
}

// D538 §12 (pages-spec-3, Update): a compose file pinning a tag is a notice on the Update page; a
// server running latest with no release record has nothing there.
func TestAttentionPages_UpdateNoticesAPinnedTag(t *testing.T) {
	if items := updateAttention(&Dependencies{}); len(items) != 0 {
		t.Errorf("with nothing known of an update: %+v; want nothing", items)
	}
	items := updateAttention(&Dependencies{Installation: Installation{ImageTag: "0.8.0"}})
	if len(items) != 1 || items[0] != (AttentionItemBody{Kind: attentionComposePinned, Version: "0.8.0"}) {
		t.Errorf("with the tag 0.8.0 pinned: %+v; want one notice of it", items)
	}
}
