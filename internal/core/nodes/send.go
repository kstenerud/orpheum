// SPDX-License-Identifier: GPL-3.0-or-later

package nodes

import (
	"context"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// Send queues a command for a node that is connected, numbering it, and wakes a waiting
// long-poll. To a node that is not, it queues nothing and answers that the job is parked on the
// node until its hello (D231 §11).
func (service *Service) Send(ctx context.Context, nodeID int64, command nodeproto.Command) error {
	connection, lastHeard := service.NodeConnection(ctx, nodeID)
	if connection != Connected {
		return jobs.ParkOnNode(nodeID, lastHeard, ErrNodeSilent)
	}
	service.Enqueue(nodeID, command)
	return nil
}

// Unanswered is a job's wait for a node's answer that ran out: the node going away, which parks
// the job on it, when it is no longer connected; otherwise err, the job's own failure.
func (service *Service) Unanswered(ctx context.Context, nodeID int64, err error) error {
	if connection, lastHeard := service.NodeConnection(ctx, nodeID); connection != Connected {
		return jobs.ParkOnNode(nodeID, lastHeard, err)
	}
	return err
}

// Enqueue queues a command for a node that has said hello, numbering it, and wakes a waiting
// long-poll; one for a node that has not is dropped. Work goes through Send, which parks it on a
// node that is not connected; a cancel goes straight here.
func (service *Service) Enqueue(nodeID int64, command nodeproto.Command) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	node, found := service.connectedNodes[nodeID]
	if !found {
		return
	}
	command.CommandSeq = node.nextCommandSeq
	node.nextCommandSeq++
	node.commands = append(node.commands, command)
	close(node.wake)
	node.wake = make(chan struct{})
}
