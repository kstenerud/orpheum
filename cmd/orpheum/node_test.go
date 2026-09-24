// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// inProcessNode runs the node's loop in this process, where run would start a child: the same
// code from runNode on, without a second binary.
type inProcessNode struct {
	done   chan struct{}
	err    error
	cancel context.CancelFunc
}

func (node *inProcessNode) Done() <-chan struct{} { return node.done }
func (node *inProcessNode) Err() error            { return node.err }
func (node *inProcessNode) Kill() error {
	node.cancel()
	<-node.done
	return nil
}

func TestRun_TheNodeScansItsRootAndTheCoreHearsEveryRecord(t *testing.T) {
	t.Parallel()
	config, _ := testConfig(t)
	media := t.TempDir()
	for name, contents := range map[string]string{"Film (2020)/Film (2020).mkv": "film", "notes.txt": "notes"} {
		path := filepath.Join(media, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// One reply serves both runs the node makes: `ffprobe -version`, whose first line becomes
	// the hello's FFmpeg version, and the probe of the film, which parses.
	fake := testutil.InstallFakeFFmpeg(t, testutil.FakeReply{
		Stdout: `{"format":{"format_name":"matroska","duration":"1.000"},"streams":[]}`,
	})
	config.Node.Roots = []string{media}
	config.Node.FFmpegDir = fake.Directory
	config.Node.VideoExtensions = []string{"mkv"}

	captured := newCapturedLog("ready", "a node finished a scan")
	var start nodes.Starter = func(ctx context.Context) (nodes.Process, error) {
		ctx, cancel := context.WithCancel(ctx)
		node := &inProcessNode{done: make(chan struct{}), cancel: cancel}
		go func() {
			defer close(node.done)
			node.err = runNode(ctx, config, captured.logger)
		}()
		return node, nil
	}
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- serve(ctx, config, ConfigReport{}, listener, start, captured.logger) }()
	select {
	case <-captured.signals["a node finished a scan"]:
	case err := <-done:
		cancel()
		t.Fatalf("serve returned before the scan finished: %v\n%s", err, captured)
	}
	cancel()
	if err := <-done; err != nil {
		t.Errorf("serve returned %v", err)
	}

	log := captured.String()
	for _, want := range []string{
		`"msg":"started the node"`,
		`"msg":"a node said hello"`,
		`"ffmpeg":"{\"format\"`,
		`"msg":"a node finished a scan"`,
		`"observed":1,"skipped":1,"errors":0`,
	} {
		if !strings.Contains(log, want) {
			t.Errorf("the log lacks %s:\n%s", want, log)
		}
	}
}

func TestRunNode_RefusesToRunUnconfinedWhenConfinementIsRequired(t *testing.T) {
	t.Parallel()
	config, _ := testConfig(t)
	config.Node.Confinement = "require"
	captured := newCapturedLog()
	if err := runNode(t.Context(), config, captured.logger); err == nil || !strings.Contains(err.Error(), "M3") {
		t.Errorf("runNode = %v, want a refusal saying confinement comes in M3", err)
	}
}
