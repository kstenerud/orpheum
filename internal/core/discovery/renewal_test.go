// SPDX-License-Identifier: AGPL-3.0-or-later

package discovery

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
)

func TestRenewal_CallersAskingAtOnceShareOneRenewal(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		var (
			one      renewal
			opened   atomic.Int32
			released = make(chan struct{})
			tokens   = make([]string, 3)
			asking   sync.WaitGroup
		)
		renew := func(ctx context.Context) (string, error) {
			opened.Add(1)
			select {
			case <-released:
				return "the token", nil
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}
		for index := range tokens {
			asking.Go(func() {
				token, err := one.do(t.Context(), renew)
				if err != nil {
					t.Error(err)
				}
				tokens[index] = token
			})
		}
		synctest.Wait() // all three are asking: one renews, the others wait on it
		close(released)
		asking.Wait()
		if count := opened.Load(); count != 1 {
			t.Errorf("three callers at once opened %d sessions; want one", count)
		}
		for index, token := range tokens {
			if token != "the token" {
				t.Errorf("caller %d got %q; want the one renewal's token", index, token)
			}
		}
	})
}
