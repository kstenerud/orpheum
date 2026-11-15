// SPDX-License-Identifier: MPL-2.0

import { useEffect } from "react";

import { setMyDeviceDataCaps } from "./accounts.ts";
import { type CoreDataCaps, deviceDataCaps } from "./data-cap.ts";
import { useServices } from "./services.tsx";

/**
 * Settles this device's caps with the core's at each read of who is signed in (D493): the core's
 * taken into this browser's copy, each choice after kept on the core too, and caps chosen here
 * before the core kept them carried over once (DataCaps.settleWithCore). A session with no device,
 * a guest's, keeps its caps here alone.
 */
export function useDataCapsSettledWithTheCore(
  device: { readonly id: number; readonly dataCaps: CoreDataCaps } | undefined,
): void {
  const { api } = useServices();
  useEffect(() => {
    if (device === undefined) {
      return;
    }
    deviceDataCaps().settleWithCore(device.dataCaps, async (caps) => {
      await setMyDeviceDataCaps(api, device.id, caps);
    });
  }, [api, device]);
}
