// SPDX-License-Identifier: AGPL-3.0-or-later

package playback

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
)

// speedKey is who measured a link speed, a device or, for one with no device row, its session, and
// on which network (clientnet.Of).
type speedKey struct {
	player, network string
}

// measuredSpeed is a link speed a player measured, the spread of the natural logs of the samples it
// rested on, 0 where the player said none, and when.
type measuredSpeed struct {
	kbps      int64
	logSpread float64
	at        time.Time
}

// highestSteadiness is the player's steadiest level (highestSteadiness in steadiness.ts): a level
// reported above it is kept as it.
const highestSteadiness = 3

// rememberedLinkSamples is how many of its own samples a remembered link counts as in a player's
// estimate of its link while it is under rememberedLinkWholeFor old, and rememberedLinkWholeFor how
// long it counts so much: from then it counts less, in step with its age, and nothing at
// link_speed_fresh_hours (bandwidth-sampling-2026-10-29.md P2, the owner, 2026-10-29). Textbook
// starting values, a Bayesian prior's weight and a Kalman filter's lesson that an old prior is
// worth less, measured on no link of ours; the player's calibration log is to tune them. Constants,
// not settings: they tune the player's estimator against its other constants (D352's Values).
const (
	rememberedLinkSamples  = 2.0
	rememberedLinkWholeFor = time.Hour
)

// countsAsSamples is how many samples speed counts as in its player's estimate at now, a speed
// fresh for freshFor: rememberedLinkSamples while under rememberedLinkWholeFor old, then less in step
// with its age, to none at freshFor.
func (speed measuredSpeed) countsAsSamples(now time.Time, freshFor time.Duration) float64 {
	age := now.Sub(speed.at)
	if age <= rememberedLinkWholeFor {
		return rememberedLinkSamples
	}
	if age >= freshFor || freshFor <= rememberedLinkWholeFor {
		return 0
	}
	return rememberedLinkSamples * float64(freshFor-age) / float64(freshFor-rememberedLinkWholeFor)
}

// linkSpeeds are what players said of their links on real video, the latest of each player on each
// network (D257 §1): the speed with its samples' spread, and the steadiness level a chunked player
// ended at (D352 Q2). They are kept in the store (link_memory), so a restart, a deploy among them,
// keeps them: kept in memory, each deploy forgot them, and the next start trusted two samples of a
// fresh connection over a link measured minutes before (bandwidth-sampling-2026-10-29.md §1.2, P2).
type linkSpeeds struct {
	database *store.Cache
}

func newLinkSpeeds(database *store.Cache) *linkSpeeds { return &linkSpeeds{database: database} }

// remember keeps a player's measurement on a network, in place of the last, and forgets every link
// said nothing of within freshFor: each player on each network it ever played from was kept for the
// server's life (the M4 closing audit, 2026-10-08).
func (speeds *linkSpeeds) remember(
	ctx context.Context, key speedKey, speed measuredSpeed, freshFor time.Duration,
) error {
	if speed.kbps <= 0 || key.network == "" {
		return nil
	}
	var spread *float64
	if speed.logSpread > 0 {
		spread = &speed.logSpread
	}
	measuredAtMs := speed.at.UnixMilli()
	return speeds.keep(ctx, speed.at, freshFor, func(queries *cachegen.Queries) error {
		return queries.UpsertLinkSpeed(ctx, cachegen.UpsertLinkSpeedParams{
			Player: key.player, Network: key.network, SpeedKbps: &speed.kbps, SpeedLogSpread: spread,
			SpeedAt: &measuredAtMs,
		})
	})
}

// rememberSteadiness keeps the steadiness level a player reported on a network, in place of the
// last, as remember keeps its speed; a level of 0 is kept too, over a steadier one before it.
func (speeds *linkSpeeds) rememberSteadiness(
	ctx context.Context, key speedKey, level int, reportedAt time.Time, freshFor time.Duration,
) error {
	if level < 0 || key.network == "" {
		return nil
	}
	kept, reportedAtMs := int64(min(level, highestSteadiness)), reportedAt.UnixMilli()
	return speeds.keep(ctx, reportedAt, freshFor, func(queries *cachegen.Queries) error {
		return queries.UpsertLinkSteadiness(ctx, cachegen.UpsertLinkSteadinessParams{
			Player: key.player, Network: key.network, Steadiness: &kept, SteadinessAt: &reportedAtMs,
		})
	})
}

// keep writes by change, after forgetting every link said nothing of within freshFor of now.
func (speeds *linkSpeeds) keep(
	ctx context.Context, now time.Time, freshFor time.Duration, change func(queries *cachegen.Queries) error,
) error {
	err := speeds.database.Write(ctx, func(queries *cachegen.Queries) error {
		if err := queries.DeleteLinkMemoriesSaidBefore(ctx, now.Add(-freshFor).UnixMilli()); err != nil {
			return err
		}
		return change(queries)
	})
	if err != nil {
		return fmt.Errorf("remember a player's link: %w", err)
	}
	return nil
}

// remembered is what the player key names last said of its link on its network; found false for
// nothing.
func (speeds *linkSpeeds) remembered(ctx context.Context, key speedKey) (cachegen.GetLinkMemoryRow, bool, error) {
	row, err := store.OrNotFound(speeds.database.Read().GetLinkMemory(ctx, cachegen.GetLinkMemoryParams{
		Player: key.player, Network: key.network,
	}))
	if errors.Is(err, store.ErrNotFound) {
		return row, false, nil
	}
	if err != nil {
		return row, false, fmt.Errorf("read a player's remembered link: %w", err)
	}
	return row, true, nil
}

// fresh is a player's speed on a network, and whether one was measured within freshFor of now:
// link_speed_fresh_hours, a day to start (D257 §1).
func (speeds *linkSpeeds) fresh(
	ctx context.Context, key speedKey, now time.Time, freshFor time.Duration,
) (measuredSpeed, bool, error) {
	row, found, err := speeds.remembered(ctx, key)
	if err != nil || !found || row.SpeedKbps == nil || row.SpeedAt == nil {
		return measuredSpeed{}, false, err
	}
	speed := measuredSpeed{
		kbps: *row.SpeedKbps, logSpread: ptr.ValueOrZero(row.SpeedLogSpread), at: time.UnixMilli(*row.SpeedAt),
	}
	if now.Sub(speed.at) > freshFor {
		return measuredSpeed{}, false, nil
	}
	return speed, true, nil
}

// steadiness is the steadiness level a player's next playback on a network starts at (D352 Q2): the
// one it last reported there within freshFor of now, as its speed is kept, relaxed by one for each
// relaxEvery since (link_steadiness_relaxes_seconds, an hour to start: between playbacks nothing is
// watched, so a level relaxes six times slower than a playing player's ten calm minutes, D352 §3, and
// a bad evening is forgotten by the next morning); 0 for none.
func (speeds *linkSpeeds) steadiness(
	ctx context.Context, key speedKey, now time.Time, freshFor, relaxEvery time.Duration,
) (int, error) {
	row, found, err := speeds.remembered(ctx, key)
	if err != nil || !found || row.Steadiness == nil || row.SteadinessAt == nil {
		return 0, err
	}
	since := max(0, now.Sub(time.UnixMilli(*row.SteadinessAt)))
	if since > freshFor {
		return 0, nil
	}
	return max(0, int(*row.Steadiness)-int(since/relaxEvery)), nil
}

// playerOf names who measures a link: a device by its id, or a session with no device row, a Watch
// Together viewer's, by its key.
func playerOf(deviceID int64, sessionKey string) string {
	if deviceID != 0 {
		return "device " + strconv.FormatInt(deviceID, 10)
	}
	return "session " + sessionKey
}
