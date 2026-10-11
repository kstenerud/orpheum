// SPDX-License-Identifier: AGPL-3.0-or-later

package turn

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/pion/logging"
)

// slogFactory gives pion's TURN server loggers that write its warnings and errors to the server's
// log; its trace, debug and info are a protocol's chatter, and are dropped.
type slogFactory struct {
	ctx    context.Context //nolint:containedctx // the relay's life, which pion's callbacks do not carry
	logger *slog.Logger
}

// NewLogger is a logger for one of pion's scopes, which its lines name.
func (factory slogFactory) NewLogger(scope string) logging.LeveledLogger {
	return slogLogger{ctx: factory.ctx, logger: factory.logger.With(slog.String("scope", scope))}
}

// slogLogger is one of pion's scoped loggers, on slog.
type slogLogger struct {
	ctx    context.Context //nolint:containedctx // as slogFactory's
	logger *slog.Logger
}

// Trace drops a trace line.
func (slogLogger) Trace(string) {}

// Tracef drops a trace line.
func (slogLogger) Tracef(string, ...any) {}

// Debug drops a debug line.
func (slogLogger) Debug(string) {}

// Debugf drops a debug line.
func (slogLogger) Debugf(string, ...any) {}

// Info drops an info line.
func (slogLogger) Info(string) {}

// Infof drops an info line.
func (slogLogger) Infof(string, ...any) {}

// Warn logs a warning.
func (each slogLogger) Warn(message string) {
	each.logger.WarnContext(each.ctx, "the TURN relay warned", slog.String("detail", message))
}

// Warnf logs a warning.
func (each slogLogger) Warnf(format string, args ...any) { each.Warn(fmt.Sprintf(format, args...)) }

// Error logs a failure.
func (each slogLogger) Error(message string) {
	each.logger.ErrorContext(each.ctx, "the TURN relay failed", slog.String("detail", message))
}

// Errorf logs a failure.
func (each slogLogger) Errorf(format string, args ...any) { each.Error(fmt.Sprintf(format, args...)) }
