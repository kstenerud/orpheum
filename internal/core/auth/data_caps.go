// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"context"

	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// NetworkKind is the kind of network a device's browser says it is on (navigator.connection's
// type), which of its data caps holds there: Wi-Fi, a wired link with it, mobile data, or one the
// browser does not say. The empty kind is none said, as a cast's receiver says none (D489).
type NetworkKind string

// The kinds of network a data cap is kept for (D488).
const (
	NetworkWifi    NetworkKind = "wifi"
	NetworkMobile  NetworkKind = "mobile"
	NetworkUnknown NetworkKind = "unknown"
)

// DataCaps are the most a film may use on a device, kbit/s, a picture's average with its sound's,
// on each kind of network its browser says (D488, D491), kept by the core (D493); 0 is No limit.
type DataCaps struct {
	WifiKbps, MobileKbps, UnknownKbps int64
}

// DataCapsOf are the caps as a device's row keeps them, NULL for No limit.
func DataCapsOf(wifiKbps, mobileKbps, unknownKbps *int64) DataCaps {
	valueOf := func(kbps *int64) int64 {
		if kbps == nil {
			return 0
		}
		return *kbps
	}
	return DataCaps{WifiKbps: valueOf(wifiKbps), MobileKbps: valueOf(mobileKbps), UnknownKbps: valueOf(unknownKbps)}
}

// Of is the cap on a network of kind, 0 for No limit and for a kind not said.
func (caps DataCaps) Of(kind NetworkKind) int64 {
	switch kind {
	case NetworkWifi:
		return caps.WifiKbps
	case NetworkMobile:
		return caps.MobileKbps
	case NetworkUnknown:
		return caps.UnknownKbps
	}
	return 0
}

// SetDataCaps keeps one of a person's devices' caps, the three at once, as its person chose them on
// it (D493).
func (service *Service) SetDataCaps(ctx context.Context, userID, deviceID int64, caps DataCaps) error {
	nullOf := func(kbps int64) *int64 {
		if kbps <= 0 {
			return nil
		}
		return &kbps
	}
	return service.onOwnDevice(ctx, userID, deviceID, func(queries *sqlcgen.Queries) error {
		return queries.SetDeviceDataCaps(ctx, sqlcgen.SetDeviceDataCapsParams{
			WifiKbps: nullOf(caps.WifiKbps), MobileKbps: nullOf(caps.MobileKbps),
			UnknownKbps: nullOf(caps.UnknownKbps), ID: deviceID,
		})
	})
}
