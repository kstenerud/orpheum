// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"sync/atomic"

	"github.com/kstenerud/orpheum/internal/core/together"
)

// What the external tests, package api_test, read of the package's internals: they are external so
// they may build the server's dependencies by internal/core/assembly, which imports this package
// (D351 §4).

// The package's own names, under exported aliases.
const (
	CacheForADay             = cacheForADay
	ClientPingEvery          = clientPingEvery
	IdempotentAnswerKept     = idempotentAnswerKept
	LabResultBytesAtMost     = labResultBytesAtMost
	LabResultIDLength        = labResultIDLength
	ProblemBodySchemaName    = problemBodySchemaName
	SchemaRefPrefix          = schemaRefPrefix
	SilenceLimit             = silenceLimit
	SmallestCompressedAnswer = smallestCompressedAnswer
	TogetherSocketPath       = togetherSocketPath
	SocketPath               = socketPath
)

var (
	ByteCookie           = byteCookie
	Codes                = codes
	CompressedOperations = compressedOperations
	DiscoveryLink        = discoveryLink
	FilmReadingFinding   = filmReadingFinding
	HairpinFinding       = hairpinFinding
	HardwareFinding      = hardwareFinding
	IsDirectPath         = isDirectPath
	NodeBuildFinding     = nodeBuildFinding
	OperationsIn         = operationsIn
	ProblemDetails       = problemDetails
	ProviderReasons      = providerReasons
	ServerFacts          = serverFacts
	ShareFinding         = shareFinding
	TogetherLink         = togetherLink
)

// DocumentedOperation is one operation the API document holds (OperationsIn).
type DocumentedOperation = documentedOperation

// ID is the operation's ID.
func (operation documentedOperation) ID() string { return operation.id }

// Method is the operation's HTTP method.
func (operation documentedOperation) Method() string { return operation.method }

// Path is the operation's path.
func (operation documentedOperation) Path() string { return operation.path }

// Dependencies are what the server answers with, once it is ready.
func (server *Server) Dependencies() *atomic.Pointer[Dependencies] { return &server.dependencies }

// ReadyDependencies are the server's dependencies, or the error a request is answered with before it
// is ready.
func (server *Server) ReadyDependencies() (*Dependencies, error) { return server.readyDependencies() }

// NamedClientOrigin is the client origin a test names in place of the discovery's.
func (server *Server) NamedClientOrigin() *atomic.Pointer[string] { return &server.namedClientOrigin }

// NamedLoaderOrigin is the loader origin a test names in place of the discovery's.
func (server *Server) NamedLoaderOrigin() *atomic.Pointer[string] { return &server.namedLoaderOrigin }

// TogetherSilence is how long a Watch Together device may send nothing, in nanoseconds.
func (server *Server) TogetherSilence() *atomic.Int64 { return &server.togetherSilence }

// TogetherTuning is how a device follows its group, as the settings say now.
func (server *Server) TogetherTuning(ctx context.Context, dependencies *Dependencies) (together.Tuning, error) {
	return server.togetherTuning(ctx, dependencies)
}

// ChunksError is the refusal a chunk's failure answers with.
func (server *Server) ChunksError(ctx context.Context, action string, err error) error {
	return server.chunksError(ctx, action, err)
}
