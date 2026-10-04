// SPDX-License-Identifier: GPL-3.0-or-later

package nodes

import (
	"context"
	"errors"
	"fmt"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// readerSetting is the setting that says how many may read a device of a kind at once: an unknown
// device's for a kind this core does not know.
func readerSetting(kind nodeproto.DeviceKind) settings.Key {
	switch kind {
	case nodeproto.DeviceSpinning:
		return settings.SpinningDiskReaderLimit
	case nodeproto.DeviceSSD:
		return settings.SSDReaderLimit
	case nodeproto.DeviceNetwork:
		return settings.NetworkShareReaderLimit
	case nodeproto.DeviceUnknown:
	}
	return settings.UnknownDiskReaderLimit
}

// Unpark brings back the work parked on a destination (Sender).
func (service *Service) Unpark(ctx context.Context, destination jobs.Destination) {
	service.destinations.Unpark(ctx, destination)
}

// Resources are what a connected node can run at once, as its last hello said and the settings say
// now (D240 §5), and false for a node not connected.
func (service *Service) Resources(ctx context.Context, nodeID int64) (Resources, bool, error) {
	service.mutex.Lock()
	node, isConnected := service.connectedNodes[nodeID]
	var hello nodeproto.Hello
	if isConnected {
		hello = node.hello
	}
	service.mutex.Unlock()
	if !isConnected {
		return Resources{}, false, nil
	}
	limits, err := service.readerLimits(ctx, nodeID, hello.Devices)
	if err != nil {
		return Resources{}, false, err
	}
	return Resources{Capacity: hello.Capacity, Devices: hello.Devices, ReaderLimits: limits}, true, nil
}

// devicesOf are the devices a connected node's last hello listed.
func (service *Service) devicesOf(nodeID int64) []nodeproto.Device {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	if node, isConnected := service.connectedNodes[nodeID]; isConnected {
		return node.hello.Devices
	}
	return nil
}

// readerLimits are how many may read each of a node's devices at once, by its key: the limit an
// admin set for the device, else the setting for its kind, and an unknown device's for a kind this
// core does not know (D240 §5).
func (service *Service) readerLimits(
	ctx context.Context, nodeID int64, devices []nodeproto.Device,
) (map[string]int, error) {
	set, err := service.setReaderLimits(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	limits := make(map[string]int, len(devices))
	for _, device := range devices {
		if limit, isSet := set[device.Key]; isSet {
			limits[device.Key] = limit
			continue
		}
		key := readerSetting(device.Kind)
		limit, err := service.settings.Int(ctx, key)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", key, err)
		}
		limits[device.Key] = int(limit)
	}
	return limits, nil
}

// setReaderLimits are the reader limits an admin set for a node's devices, by key.
func (service *Service) setReaderLimits(ctx context.Context, nodeID int64) (map[string]int, error) {
	rows, err := service.database.Read().ListDeviceReaderLimits(ctx, nodeID)
	if err != nil {
		return nil, fmt.Errorf("read node %d's device reader limits: %w", nodeID, err)
	}
	set := make(map[string]int, len(rows))
	for _, row := range rows {
		set[row.DeviceKey] = int(row.ReaderLimit)
	}
	return set, nil
}

// ErrNoNode is a node name no live node has.
var ErrNoNode = errors.New("no node has that name")

// SetReaderLimit sets how many may read one of a node's devices at once, over its kind's setting,
// or, with limit nil, gives it back to its kind's (D240 §5). The node hears it with its next poll.
func (service *Service) SetReaderLimit(ctx context.Context, nodeName, deviceKey string, limit *int) error {
	err := service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		node, err := store.OrNotFound(queries.GetLiveNodeByName(ctx, nodeName))
		if errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("node %q: %w", nodeName, ErrNoNode)
		}
		if err != nil {
			return err
		}
		nodeID := node.ID
		if limit == nil {
			return queries.ClearDeviceReaderLimit(ctx, sqlcgen.ClearDeviceReaderLimitParams{
				NodeID: nodeID, DeviceKey: deviceKey,
			})
		}
		return queries.SetDeviceReaderLimit(ctx, sqlcgen.SetDeviceReaderLimitParams{
			NodeID: nodeID, DeviceKey: deviceKey, ReaderLimit: int64(*limit), SetAt: service.clock.Now().UnixMilli(),
		})
	})
	if err != nil {
		return fmt.Errorf("set node %s's reader limit of %s: %w", nodeName, deviceKey, err)
	}
	return nil
}

// DeviceState is one of a node's devices as its last hello said, with how many may read it at once
// and whether an admin set that.
type DeviceState struct {
	nodeproto.Device
	ReaderLimit      int
	IsReaderLimitSet bool
}

// DeviceStates are the devices a connected node's last hello listed, each with its reader limit;
// none for a node not connected.
func (service *Service) DeviceStates(ctx context.Context, nodeID int64) ([]DeviceState, error) {
	devices := service.devicesOf(nodeID)
	set, err := service.setReaderLimits(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	limits, err := service.readerLimits(ctx, nodeID, devices)
	if err != nil {
		return nil, err
	}
	states := make([]DeviceState, 0, len(devices))
	for _, device := range devices {
		_, isSet := set[device.Key]
		states = append(states, DeviceState{Device: device, ReaderLimit: limits[device.Key], IsReaderLimitSet: isSet})
	}
	return states, nil
}
