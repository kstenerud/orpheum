// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/artwork"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// artCacheControl lets a client keep an image for good: its URL names its content (D212,
// artwork.md, Content-addressed).
const artCacheControl = "private, max-age=31536000, immutable"

// ArtInput asks for one size of one image.
type ArtInput struct {
	MediaCredentials
	Hash    string `doc:"The image's hash, from a title's artwork" path:"hash" pattern:"^[0-9a-f]{64}$"`
	Variant string `doc:"A size and a format: portrait-m.webp, landscape-l.jpeg" path:"variant"`
}

// ArtOutput is the image, or a temporary redirect to the nearest size while the one asked for
// is made.
type ArtOutput struct {
	Status       int
	ContentType  string `header:"Content-Type"`
	CacheControl string `header:"Cache-Control"`
	Location     string `header:"Location"`
	Body         []byte
}

func (server *Server) registerArt() {
	huma.Register(server.api, huma.Operation{
		OperationID: "getArt", Method: http.MethodGet, Path: "/bytes/art/{hash}/{variant}",
		Summary: "One size of one image, in WebP or JPEG (D212)", Tags: []string{tagBrowse},
		Responses: map[string]*huma.Response{
			"200": {Description: "The image", Content: map[string]*huma.MediaType{
				"image/webp": {}, "image/jpeg": {},
			}},
			"307": {Description: "The nearest size, while this one is made or when it cannot be"},
		},
	}, server.getArt)
}

func (server *Server) getArt(ctx context.Context, input *ArtInput) (*ArtOutput, error) {
	dependencies, err := server.signedInForMedia(ctx, input.MediaCredentials)
	if err != nil {
		return nil, err
	}
	name, format, found := strings.Cut(input.Variant, ".")
	if !found {
		return nil, newError(http.StatusNotFound, CodeNotFound, "no such image")
	}
	served, err := dependencies.Artwork.Serve(ctx, input.Hash, nodeproto.ArtSizeName(name), format)
	if errors.Is(err, artwork.ErrNoArt) {
		return nil, newError(http.StatusNotFound, CodeNotFound, "no such image")
	}
	if err != nil {
		return nil, server.internal(ctx, "serve an image", err)
	}
	if served.Redirect != "" {
		return &ArtOutput{
			Status: http.StatusTemporaryRedirect, CacheControl: "no-store",
			Location: Prefix + "/bytes/art/" + input.Hash + "/" + served.Redirect,
		}, nil
	}
	return &ArtOutput{
		Status: http.StatusOK, ContentType: "image/" + format, CacheControl: artCacheControl, Body: served.Bytes,
	}, nil
}
