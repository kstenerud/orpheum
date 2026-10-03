// SPDX-License-Identifier: GPL-3.0-or-later

package nodes

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/unixsocket"
)

// ErrNodeSilent is a node the core cannot reach: it has not said hello, or its byte socket does
// not answer.
var ErrNodeSilent = errors.New("the node is not answering")

// ByteSocketClient reads from a node's byte socket (cut-a §4): the core's only way to a media file's
// bytes, since it never opens one itself (D193 §1). Where each node's socket is, its hello says
// (D242 §7).
type ByteSocketClient struct {
	service *Service

	mutex sync.Mutex
	// clientBySocket is a client for each socket a node's hello named; guarded by mutex.
	clientBySocket map[string]*http.Client
}

// NewByteSocketClient reads from the byte sockets of the nodes the service has heard from.
func NewByteSocketClient(service *Service) *ByteSocketClient {
	return &ByteSocketClient{service: service, clientBySocket: map[string]*http.Client{}}
}

// clientFor is the client for the socket at socketPath, made on first use.
func (socketClient *ByteSocketClient) clientFor(socketPath string) *http.Client {
	socketClient.mutex.Lock()
	defer socketClient.mutex.Unlock()
	client, found := socketClient.clientBySocket[socketPath]
	if !found {
		transport := unixsocket.Transport(socketPath)
		// A player's reads come one range after another; keep the connection for the next. Four is
		// a few players at once, each with a request in flight; more would only hold idle sockets.
		transport.MaxIdleConnsPerHost = 4
		client = &http.Client{Transport: transport}
		socketClient.clientBySocket[socketPath] = client
	}
	return client
}

// Do sends one GET or HEAD to the node's byte socket, for the path and query in target. The
// caller closes the response's body.
func (socketClient *ByteSocketClient) Do(ctx context.Context, nodeID int64, method, target string, header http.Header) (
	*http.Response, error,
) {
	socketPath, found := socketClient.service.byteSocketPath(nodeID)
	if !found || !socketClient.service.IsConnected(ctx, nodeID) {
		return nil, ErrNodeSilent
	}
	request, err := http.NewRequestWithContext(ctx, method, "http://node"+target, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("build a request to the node: %w", err)
	}
	request.Header = header
	response, err := socketClient.clientFor(socketPath).Do(request)
	if err != nil {
		return nil, errors.Join(ErrNodeSilent, err)
	}
	return response, nil
}

// ReadHeldResult fetches a result the node holds, too large to post, from its byte socket by the
// attempt's token, and takes it only whole: at the size and SHA-256 the post named, within
// maxResultBytes, and for the same job and attempt (D234 §5). The post that named it is posted.
func (socketClient *ByteSocketClient) ReadHeldResult(
	ctx context.Context, nodeID int64, posted nodeproto.Result,
) (nodeproto.Result, error) {
	held := posted.Held
	if held.SizeBytes <= 0 || held.SizeBytes > maxResultBytes {
		return nodeproto.Result{}, fmt.Errorf("a held result of %d bytes, past %d: %w", held.SizeBytes,
			maxResultBytes, ErrMalformedResult)
	}
	response, err := socketClient.Do(ctx, nodeID, http.MethodGet, nodeproto.PathBytesResult+posted.AttemptToken, nil)
	if err != nil {
		return nodeproto.Result{}, fmt.Errorf("fetch job %d's held result: %w", posted.JobID, err)
	}
	defer func() { _ = response.Body.Close() }() //nolint:errcheck // read-only
	if response.StatusCode != http.StatusOK {
		return nodeproto.Result{}, fmt.Errorf("fetch job %d's held result: the node answered %s", posted.JobID,
			response.Status)
	}
	encoded, err := io.ReadAll(io.LimitReader(response.Body, held.SizeBytes+1))
	if err != nil {
		return nodeproto.Result{}, fmt.Errorf("read job %d's held result: %w", posted.JobID, err)
	}
	return takeHeld(posted, encoded)
}

// takeHeld is the result a held result's bytes encode, when they are the ones posted named and the
// result is for the same job and attempt.
func takeHeld(posted nodeproto.Result, encoded []byte) (nodeproto.Result, error) {
	sum := sha256.Sum256(encoded)
	if int64(len(encoded)) != posted.Held.SizeBytes || hex.EncodeToString(sum[:]) != posted.Held.Sha256 {
		return nodeproto.Result{}, fmt.Errorf("job %d's held result is not the %d bytes its post named: %w",
			posted.JobID, posted.Held.SizeBytes, ErrMalformedResult)
	}
	var result nodeproto.Result
	if err := json.Unmarshal(encoded, &result); err != nil {
		return nodeproto.Result{}, fmt.Errorf("job %d's held result is not JSON: %w: %w", posted.JobID, err,
			ErrMalformedResult)
	}
	if result.JobID != posted.JobID || result.AttemptToken != posted.AttemptToken || result.Held != nil {
		return nodeproto.Result{}, fmt.Errorf("job %d's held result is another attempt's: %w", posted.JobID,
			ErrMalformedResult)
	}
	return result, nil
}
