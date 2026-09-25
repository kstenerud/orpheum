// SPDX-License-Identifier: GPL-3.0-or-later

//go:build integration

package artwork

import (
	"bytes"
	"image/jpeg"
	"testing"

	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/testutil"
)

func TestServe_AJPEGIsCutFromTheWebPOfItsSizeAndKept(t *testing.T) {
	t.Parallel()
	bench := newBench(t, fetched{})
	hash := Received([]byte("poster")).hash()
	webp := testutil.GenerateImage(t, "240x360", "libwebp", "webp")
	if err := bench.art.WriteArt(hash, "portrait-s.webp", webp); err != nil {
		t.Fatal(err)
	}
	served, err := bench.service.Serve(t.Context(), hash, nodeproto.PortraitS, FormatJPEG)
	if err != nil || served.Redirect != "" {
		t.Fatalf("Serve = %+v, %v", served.Redirect, err)
	}
	config, err := jpeg.DecodeConfig(bytes.NewReader(served.Bytes))
	if err != nil || config.Width != 240 || config.Height != 360 {
		t.Errorf("the JPEG: %+v, %v; want 240×360", config, err)
	}
	if kept := bench.art[hash+"/portrait-s.jpeg"]; !bytes.Equal(kept, served.Bytes) {
		t.Error("the JPEG was not kept")
	}
}
