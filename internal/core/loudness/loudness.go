// SPDX-License-Identifier: AGPL-3.0-or-later

package loudness

import (
	"context"
	"fmt"

	"github.com/kstenerud/orpheum/internal/core/decide"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/store"
)

// Settings are the settings loudness reads.
type Settings interface {
	Int(ctx context.Context, key settings.Key) (int64, error)
}

// Parts are what the loudness service is built from.
type Parts struct {
	Database *store.Database
	Settings Settings
}

// Service says the gain that levels each measured sound of a file's bytes (D226), from the
// measurements a read-through keeps (internal/core/readthrough).
type Service struct {
	database *store.Database
	settings Settings
}

// NewService returns the loudness service.
func NewService(parts Parts) *Service {
	return &Service{database: parts.Database, settings: parts.Settings}
}

// aim reads the level and the most a sound is raised from their settings.
func (service *Service) aim(ctx context.Context) (decide.LoudnessAim, error) {
	target, err := service.settings.Int(ctx, settings.LoudnessTargetLUFS)
	if err != nil {
		return decide.LoudnessAim{}, fmt.Errorf("read loudnessTargetLufs: %w", err)
	}
	maxRaise, err := service.settings.Int(ctx, settings.LoudnessMaxRaiseDB)
	if err != nil {
		return decide.LoudnessAim{}, fmt.Errorf("read loudnessMaxRaiseDb: %w", err)
	}
	return decide.LoudnessAim{TargetLUFS: float64(target), MaxRaiseDB: float64(maxRaise)}, nil
}

// Levels are the gains that level each measured sound of the bytes with payloadHash.
func (service *Service) Levels(ctx context.Context, payloadHash []byte) (decide.Levels, error) {
	aim, err := service.aim(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := service.database.Read().ListLoudnessOfPayload(ctx, payloadHash)
	if err != nil {
		return nil, fmt.Errorf("read the loudness: %w", err)
	}
	levels := make(decide.Levels, len(rows))
	for _, row := range rows {
		measured := decide.Measured{
			IntegratedLUFS: row.IntegratedLUFS, TruePeakDBTP: row.TruePeakDBTP, IsFailure: row.Failure != nil,
		}
		if level, isMeasured := decide.LevellingOf(measured, aim); isMeasured {
			levels[decide.SoundKey{StreamIndex: row.StreamIndex, Mix: row.MeasuredMix}] = level
		}
	}
	return levels, nil
}
