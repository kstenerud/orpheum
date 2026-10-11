// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"strconv"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/providers"
)

// DestinationBody is a destination that is down, and what waits on it says: "waiting for TMDB —
// unreachable since 14:02, next check 14:10" (D231 §12).
type DestinationBody struct {
	// Destination is its name as the queue keeps it: "tmdb-api", "node:3", "root:3:/media".
	Destination string `json:"destination"`
	// Name is the destination as a person reads it: "TMDB", "node home", "/media on node home".
	Name   string `json:"name"`
	Reason string `doc:"Why it is down: unreachable, not connected, missing, its key is not set" json:"reason"`
	// DownAt is when it went down, in Unix milliseconds.
	DownAt int64 `json:"downAt"`
	// NextCheckAt is when it is looked at next, in Unix milliseconds; absent when only a signal
	// brings it back: a node's hello, a scan that finds a root, a key changed, room made.
	NextCheckAt *int64 `json:"nextCheckAt,omitempty"`
	IsChecking  bool   `doc:"Whether it is being looked at now" json:"isChecking"`
}

// destinationBody is a parked destination's state as the API shows it, a node named by nodeName.
func destinationBody(state jobs.DestinationState, nodeName func(nodeID int64) string) DestinationBody {
	body := DestinationBody{
		Destination: string(state.Destination), Name: jobs.DescribeDestination(state.Destination, nodeName),
		Reason: state.Reason, DownAt: state.Since.UnixMilli(), IsChecking: state.IsProbing,
	}
	if !state.NextProbeAt.IsZero() {
		body.NextCheckAt = new(state.NextProbeAt.UnixMilli())
	}
	return body
}

// nodeNamer names a node by its id, as an admin named it, or by its id when it has no name.
func (server *Server) nodeNamer(ctx context.Context, dependencies *Dependencies) func(nodeID int64) string {
	names := map[int64]string{}
	if listed, err := dependencies.Libraries.NodeRoots(ctx); err == nil {
		for _, node := range listed {
			names[node.ID] = node.Name
		}
	} else {
		server.logger.WarnContext(ctx, "could not list the nodes to name them; naming them by id", "err", err)
	}
	return func(nodeID int64) string {
		if name, found := names[nodeID]; found {
			return name
		}
		return strconv.FormatInt(nodeID, 10)
	}
}

// ProviderBody is an outside host's provider service: its health, the rate we keep to, its quota
// and its credentials (D231 §5).
type ProviderBody struct {
	Name        string `json:"name"`
	Destination string `json:"destination"`
	// RatePerSecond is the request rate we keep to, and BackgroundRatePerSecond the share of it
	// background work may use.
	RatePerSecond           float64 `json:"ratePerSecond"`
	BackgroundRatePerSecond float64 `json:"backgroundRatePerSecond"`
	// WaitUntil is when the host said it may be called again, in Unix milliseconds; absent when
	// it has not asked us to wait.
	WaitUntil   *int64 `json:"waitUntil,omitempty"`
	Credentials string `enum:"unknown,accepted,missing,refused" json:"credentials"`
	// Quota is what the host last said of its day of counted requests; absent before it said.
	Quota *QuotaBody `json:"quota,omitempty"`
	// Down is the host's state while it is down; absent while it is up.
	Down *DestinationBody `json:"down,omitempty"`
}

// QuotaBody is a host's day of counted requests: OpenSubtitles' downloads.
type QuotaBody struct {
	Remaining int `json:"remaining"`
	// ResetAt is when the count starts again, in Unix milliseconds; absent when the host did not say.
	ResetAt *int64 `json:"resetAt,omitempty"`
	Allowed int    `doc:"How many the host allows a day; 0 before it said" json:"allowed"`
	// ReservedForPeoplePercent is the percent of Allowed held back for people, and
	// ReservedForPeople how many downloads that is, rounded up: background work never spends them.
	ReservedForPeoplePercent int `json:"reservedForPeoplePercent"`
	ReservedForPeople        int `json:"reservedForPeople"`
}

// providerBody is a provider service's state as the API shows it.
func providerBody(state providers.State, nodeName func(nodeID int64) string) ProviderBody {
	body := ProviderBody{
		Name: state.Name, Destination: string(state.Destination), RatePerSecond: state.RatePerSecond,
		BackgroundRatePerSecond: state.BackgroundRatePerSecond, Credentials: string(state.Credentials),
	}
	if !state.WaitUntil.IsZero() {
		body.WaitUntil = new(state.WaitUntil.UnixMilli())
	}
	if quota := state.Quota; quota != nil {
		body.Quota = &QuotaBody{
			Remaining: quota.Remaining, Allowed: quota.Allowed,
			ReservedForPeoplePercent: quota.ReservedForPeoplePercent, ReservedForPeople: quota.ReservedForPeople,
		}
		if !quota.ResetAt.IsZero() {
			body.Quota.ResetAt = new(quota.ResetAt.UnixMilli())
		}
	}
	if state.Parked != nil {
		down := destinationBody(*state.Parked, nodeName)
		body.Down = &down
	}
	return body
}
