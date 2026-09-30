// SPDX-License-Identifier: GPL-3.0-or-later

package nodes

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/kstenerud/orpheum/internal/platform/unixsocket"
)

// ErrNodeSilent is a node the core cannot reach: it has not said hello, or its byte socket does
// not answer.
var ErrNodeSilent = errors.New("the node is not answering")

// ByteSocketClient reads from a node's byte socket (cut-a §4): the core's only way to a media file's bytes,
// since it never opens one itself (D193 §1). In Cut A the one node is the co-located child, whose
// socket is beside the core's.
type ByteSocketClient struct {
	service    *Service
	httpClient *http.Client
}

// NewByteSocketClient reads from the node socket at socketPath, for nodes the service has heard from.
func NewByteSocketClient(service *Service, socketPath string) *ByteSocketClient {
	transport := unixsocket.Transport(socketPath)
	// A player's reads come one range after another; keep the connection for the next.
	transport.MaxIdleConnsPerHost = 4
	return &ByteSocketClient{service: service, httpClient: &http.Client{Transport: transport}}
}

// Do sends one GET or HEAD to the node's byte socket, for the path and query in target. The
// caller closes the response's body.
func (socketClient *ByteSocketClient) Do(ctx context.Context, nodeID int64, method, target string, header http.Header) (
	*http.Response, error,
) {
	if !socketClient.service.IsConnected(nodeID) {
		return nil, ErrNodeSilent
	}
	request, err := http.NewRequestWithContext(ctx, method, "http://node"+target, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("build a request to the node: %w", err)
	}
	request.Header = header
	response, err := socketClient.httpClient.Do(request)
	if err != nil {
		return nil, errors.Join(ErrNodeSilent, err)
	}
	return response, nil
}
