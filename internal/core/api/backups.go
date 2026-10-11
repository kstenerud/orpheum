// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/backup"
)

// BackupBody is one file in the backups directory: a backup, or a copy kept before a restore or a
// migration.
type BackupBody struct {
	Name      string `json:"name"`
	SizeBytes int64  `json:"sizeBytes"`
	MadeAt    int64  `doc:"Unix milliseconds" json:"madeAt"`
}

// BackupsOutput lists the backups directory, newest first, and says where it is.
type BackupsOutput struct {
	Body struct {
		Directory string       `doc:"Where the backups are, on the server's machine" json:"directory"`
		Backups   []BackupBody `json:"backups"`
	}
}

// MakeBackupInput asks for a backup now.
type MakeBackupInput struct {
	AuthorizationHeader
	Body struct {
		IncludesKey bool `doc:"Put secret.key in it, for moving to another machine (D194 §4)" json:"includesKey"`
	}
}

// MadeBackupOutput says the backup is queued: the job view shows it, and the list the backup once
// it is made (D243 §11).
type MadeBackupOutput struct {
	Status int
	Body   struct {
		IncludesKey bool `json:"includesKey"`
	}
}

func (server *Server) registerBackups() {
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "listBackups", Method: http.MethodGet, Path: "/backups", Tags: []string{tagRunningIt},
		Summary: "List the backups directory, newest first (admin; D242 §5)",
	}, server.listBackups)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "makeBackup", Method: http.MethodPost, Path: "/backups", Tags: []string{tagRunningIt},
		DefaultStatus: http.StatusAccepted,
		Summary: "Back the persistent directory up now, into the backups directory, as a job the job view " +
			"shows (admin; D242 §5, D243 §11)",
	}, server.makeBackup)
}

func (server *Server) listBackups(ctx context.Context, input *AuthorizationHeader) (*BackupsOutput, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	files, err := dependencies.Backups.List()
	if err != nil {
		return nil, server.logInternalError(ctx, "list the backups", err)
	}
	output := &BackupsOutput{}
	output.Body.Directory = dependencies.Backups.Directory()
	output.Body.Backups = make([]BackupBody, 0, len(files))
	for _, file := range files {
		output.Body.Backups = append(output.Body.Backups, BackupBody{
			Name: file.Name, SizeBytes: file.SizeBytes, MadeAt: file.ModifiedAt.UnixMilli(),
		})
	}
	return output, nil
}

func (server *Server) makeBackup(ctx context.Context, input *MakeBackupInput) (*MadeBackupOutput, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	err = dependencies.Backups.Request(ctx, input.Body.IncludesKey)
	if errors.Is(err, backup.ErrNoKeyToInclude) {
		return nil, newError(http.StatusConflict, CodeConflict, err.Error())
	}
	if err != nil {
		return nil, server.logInternalError(ctx, "ask for a backup", err)
	}
	output := &MadeBackupOutput{Status: http.StatusAccepted}
	output.Body.IncludesKey = input.Body.IncludesKey
	return output, nil
}
