// SPDX-License-Identifier: AGPL-3.0-or-later

package relay

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/slogx"
)

// fileStamp is what tells a file changed: its modification time and its size.
type fileStamp struct {
	modified time.Time
	size     int64
}

// certificateFiles is the TLS listener's certificate, read from its two files and read again at a
// handshake after either changes, so a renewal needs no restart.
type certificateFiles struct {
	ctx                        context.Context //nolint:containedctx // a handshake's callback carries none to log under
	certificateFile, keyFile   string
	logger                     *slog.Logger
	mutex                      sync.Mutex
	certificate                *tls.Certificate
	certificateStamp, keyStamp fileStamp
}

// loadCertificateFiles reads the certificate and its key, refused when they cannot be read.
func loadCertificateFiles(
	ctx context.Context, certificateFile, keyFile string, logger *slog.Logger,
) (*certificateFiles, error) {
	files := &certificateFiles{ctx: ctx, certificateFile: certificateFile, keyFile: keyFile, logger: logger}
	if err := files.reload(); err != nil {
		return nil, err
	}
	return files, nil
}

// get is the certificate, read again first when either file changed. A change that cannot be read
// keeps the certificate read before, and logs why.
func (files *certificateFiles) get(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	files.mutex.Lock()
	defer files.mutex.Unlock()
	certificateStamp, certificateErr := stampOf(files.certificateFile)
	keyStamp, keyErr := stampOf(files.keyFile)
	if certificateErr == nil && keyErr == nil &&
		(certificateStamp != files.certificateStamp || keyStamp != files.keyStamp) {
		if err := files.reloadLocked(); err != nil {
			files.logger.WarnContext(files.ctx, "the relay's new certificate is unread; it keeps the one before",
				slog.String(slogx.EventKey, "relayCertificateUnread"), slog.Any("err", err))
		}
	}
	return files.certificate, nil
}

// reload reads both files.
func (files *certificateFiles) reload() error {
	files.mutex.Lock()
	defer files.mutex.Unlock()
	return files.reloadLocked()
}

// reloadLocked reads both files; the caller holds the mutex.
func (files *certificateFiles) reloadLocked() error {
	certificateStamp, err := stampOf(files.certificateFile)
	if err != nil {
		return err
	}
	keyStamp, err := stampOf(files.keyFile)
	if err != nil {
		return err
	}
	certificate, err := tls.LoadX509KeyPair(files.certificateFile, files.keyFile)
	if err != nil {
		return fmt.Errorf("read the relay's certificate: %w", err)
	}
	files.certificate, files.certificateStamp, files.keyStamp = &certificate, certificateStamp, keyStamp
	return nil
}

// stampOf is file's stamp.
func stampOf(file string) (fileStamp, error) {
	info, err := os.Stat(file)
	if err != nil {
		return fileStamp{}, fmt.Errorf("the relay's certificate files: %w", err)
	}
	return fileStamp{modified: info.ModTime(), size: info.Size()}, nil
}
