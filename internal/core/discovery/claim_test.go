// SPDX-License-Identifier: AGPL-3.0-or-later

package discovery

import (
	"errors"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
)

// A pending core is claimed by the key its account made, and then says which account holds it; a
// wrong key is refused as such, and a core not pending takes no key (the protocol's §2.2; D310 §5).
func TestClaim_APendingCoreIsClaimedByItsAccountsKeyAndNamesTheAccount(t *testing.T) {
	t.Parallel()
	built := newWorld(t)
	ctx := t.Context()
	if _, err := built.service.Claim(ctx, fakeKey); !errors.Is(err, ErrNotPending) {
		t.Errorf("a key before connecting: %v; want ErrNotPending", err)
	}
	if _, err := built.service.Connect(ctx, built.issuer); err != nil {
		t.Fatal(err)
	}
	built.service.step(ctx)
	if _, err := built.service.Claim(ctx, "wxyz-wxyz-wxyz-wxyz"); !errors.Is(err, ErrWrongKey) {
		t.Errorf("a wrong key: %v; want ErrWrongKey", err)
	}
	status, err := built.service.Claim(ctx, "  "+fakeKey+" ")
	if err != nil || status.State != discoveryproto.StateClaimed || status.Account != "karl@example.com" {
		t.Fatalf("Claim = %+v, %v; want claimed, naming karl's account", status, err)
	}
	if _, err = built.service.Claim(ctx, fakeKey); !errors.Is(err, ErrNotPending) {
		t.Errorf("a key for a claimed core: %v; want ErrNotPending", err)
	}
}

// A pending core removed from an account, another claimed in its place, says when (D312 §15).
func TestStep_APendingCoreLearnsWhenItWasRemovedFromItsAccount(t *testing.T) {
	t.Parallel()
	built := newWorld(t)
	ctx := t.Context()
	if _, err := built.service.Connect(ctx, built.issuer); err != nil {
		t.Fatal(err)
	}
	removed := time.Date(2026, 10, 19, 8, 0, 0, 0, time.UTC)
	built.fake.mutex.Lock()
	built.fake.removedAt = removed.UnixMilli()
	built.fake.mutex.Unlock()
	built.service.step(ctx)
	if status := built.service.Status(); !status.RemovedAt.Equal(removed) {
		t.Errorf("the status says removed at %s; want %s", status.RemovedAt, removed)
	}
}
