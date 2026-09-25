// SPDX-License-Identifier: GPL-3.0-or-later

package nodes

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
)

// ErrNodeSilent is a node the core cannot reach: it has not said hello, or its byte socket does
// not answer.
var ErrNodeSilent = errors.New("the node is not answering")

// Bytes reads from a node's byte socket (cut-a §4): the core's only way to a media file's bytes,
// since it never opens one itself (D193 §1). In Cut A the one node is the co-located child, whose
// socket is beside the core's.
type Bytes struct {
	service *Service
	client  *http.Client
}

// NewBytes reads from the node socket at socketPath, for nodes the service has heard from.
func NewBytes(service *Service, socketPath string) *Bytes {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, "unix", socketPath)
		},
		// A player's reads come one range after another; keep the connection for the next.
		MaxIdleConnsPerHost: 4,
	}
	return &Bytes{service: service, client: &http.Client{Transport: transport}}
}

// Do sends one GET or HEAD to the node's byte socket, for the path and query in target. The
// caller closes the response's body.
func (reader *Bytes) Do(ctx context.Context, nodeID int64, method, target string, header http.Header) (
	*http.Response, error,
) {
	if !reader.service.IsConnected(nodeID) {
		return nil, ErrNodeSilent
	}
	request, err := http.NewRequestWithContext(ctx, method, "http://node"+target, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("build a request to the node: %w", err)
	}
	request.Header = header
	response, err := reader.client.Do(request)
	if err != nil {
		return nil, errors.Join(ErrNodeSilent, err)
	}
	return response, nil
}
