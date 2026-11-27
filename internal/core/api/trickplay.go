// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/trickplay"
)

// trickplaySheetSuffix ends a sheet's address: its image's format.
const trickplaySheetSuffix = ".webp"

// webpType is the media type of a WebP image: a sheet, a key frame, an artwork.
const webpType = "image/webp"

// TrickplayInput asks for a file's scrub-bar pictures.
type TrickplayInput struct {
	AuthorizationHeader
	ID int64 `doc:"The file" path:"id"`
}

// TrickplaySheetBody is one sheet made: a grid of columns × rows pictures, row by row.
type TrickplaySheetBody struct {
	Index int    `doc:"The sheet's place from the film's start: it holds pictures index·columns·rows onwards" json:"index"` //nolint:lll // one struct tag
	URL   string `doc:"The sheet's image, a WebP named by its content and kept for good, under /bytes/"     json:"url"`
}

// TrickplayOutput is a file's scrub-bar pictures as made so far (trickplay.md section 5).
type TrickplayOutput struct {
	Body struct {
		IntervalMs    int64                `doc:"The time between pictures: picture n is of n·intervalMs, the keyframe at or before it" json:"intervalMs"`   //nolint:lll // one struct tag
		PictureWidth  int                  `doc:"Each picture's width in the sheet, in pixels"                                         json:"pictureWidth"`  //nolint:lll // one struct tag
		PictureHeight int                  `doc:"Each picture's height in the sheet, in pixels"                                        json:"pictureHeight"` //nolint:lll // one struct tag
		Columns       int                  `doc:"A sheet's pictures across"                                                            json:"columns"`       //nolint:lll // one struct tag
		Rows          int                  `doc:"A sheet's pictures down"                                                              json:"rows"`          //nolint:lll // one struct tag
		PictureCount  int                  `doc:"The whole film's pictures, made or not"                                               json:"pictureCount"`  //nolint:lll // one struct tag
		Sheets        []TrickplaySheetBody `doc:"Each sheet made, in order; a stretch whose sheet is missing has no pictures yet"      json:"sheets"`        //nolint:lll // one struct tag
	}
}

// TrickplaySheetInput asks for one sheet of a file's scrub-bar pictures.
type TrickplaySheetInput struct {
	MediaCredentials
	ID    int64  `doc:"The file"                                     path:"id"`
	Sheet string `doc:"The sheet's SHA-256 and format, from its list" path:"sheet" pattern:"^[0-9a-f]{64}\\.webp$"`
}

func (server *Server) registerTrickplay() {
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getFileTrickplay", Method: http.MethodGet, Path: "/files/{id}/trickplay",
		Summary: "A file's scrub-bar pictures made so far, their sizes and each sheet's address " +
			"(trickplay.md); 404 while none is made",
		Tags:       []string{tagPlayback},
		Extensions: callableBy(capabilitySignedInOrGuest),
	}, server.getFileTrickplay)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getFileTrickplaySheet", Method: http.MethodGet, Path: "/bytes/trickplay/{id}/{sheet}",
		Summary: "One sheet of a file's scrub-bar pictures, a WebP, content-addressed as an image is (D212)",
		Tags:    []string{tagPlayback},
		Responses: map[string]*huma.Response{
			"200": {Description: "The sheet", Content: map[string]*huma.MediaType{webpType: {}}},
		},
		Extensions: callableBy(capabilityMedia),
	}, server.getFileTrickplaySheet)
}

// getFileTrickplay answers GET /files/{id}/trickplay: the pictures made so far of a file the person
// reaches, authorised as its segments are (getFileSegments). A file they do not reach is one that
// does not exist (D243 §4), and one with none made yet has none.
func (server *Server) getFileTrickplay(ctx context.Context, input *TrickplayInput) (*TrickplayOutput, error) {
	dependencies, principal, err := server.requireSignedInOrGuest(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if principal.IsGuestReceiver() && !guestPlays(dependencies, principal, input.ID) {
		return nil, errGuestReceiver
	}
	if err = dependencies.Browser.ReachFile(ctx, principal.Reach, input.ID); err != nil {
		return nil, server.playbackError(ctx, "reach a file", err)
	}
	pictures, err := dependencies.Trickplay.PicturesOf(ctx, input.ID)
	if errors.Is(err, trickplay.ErrNoPictures) {
		return nil, newError(http.StatusNotFound, CodeNotFound, "no scrub-bar pictures of this file are made yet")
	}
	if err != nil {
		return nil, server.logInternalError(ctx, "read a file's scrub-bar pictures", err)
	}
	output := &TrickplayOutput{}
	output.Body.IntervalMs, output.Body.PictureCount = pictures.IntervalMs, pictures.PictureCount
	output.Body.PictureWidth, output.Body.PictureHeight = pictures.PictureWidth, pictures.PictureHeight
	output.Body.Columns, output.Body.Rows = pictures.Columns, pictures.Rows
	output.Body.Sheets = make([]TrickplaySheetBody, 0, len(pictures.Sheets))
	for _, sheet := range pictures.Sheets {
		output.Body.Sheets = append(output.Body.Sheets, TrickplaySheetBody{
			Index: sheet.Index,
			URL: Prefix + "/bytes/trickplay/" + strconv.FormatInt(input.ID, 10) + "/" +
				sheet.Sha256Hex + trickplaySheetSuffix,
		})
	}
	return output, nil
}

// getFileTrickplaySheet answers GET /bytes/trickplay/{id}/{sheet}: a sheet of a file the person
// reaches, by its content, kept for good as an image size is (getArt). A file they do not reach, or a
// sheet not the file's, is one that does not exist (D243 §4).
func (server *Server) getFileTrickplaySheet(ctx context.Context, input *TrickplaySheetInput) (*ArtOutput, error) {
	dependencies, principal, err := server.requireSignedInForMedia(ctx, input.MediaCredentials)
	if err != nil {
		return nil, err
	}
	if err = dependencies.Browser.ReachFile(ctx, principal.Reach, input.ID); err != nil {
		return nil, server.playbackError(ctx, "reach a file", err)
	}
	sheet, err := dependencies.Trickplay.SheetOf(ctx, input.ID, strings.TrimSuffix(input.Sheet, trickplaySheetSuffix))
	if errors.Is(err, trickplay.ErrNoSheet) {
		return nil, newError(http.StatusNotFound, CodeNotFound, "no such sheet")
	}
	if err != nil {
		return nil, server.logInternalError(ctx, "serve a sheet of scrub-bar pictures", err)
	}
	return &ArtOutput{Status: http.StatusOK, ContentType: webpType, CacheControl: cacheImmutable, Body: sheet}, nil
}
