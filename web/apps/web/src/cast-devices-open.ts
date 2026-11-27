// SPDX-License-Identifier: MPL-2.0

/** The person's other devices Cast's menu lists (cast-to.tsx's CastTargets). */

import { useMemo } from "react";

import { listMyDevices, myDevicesKey, type Device } from "./accounts.ts";
import { useServices } from "./services.tsx";
import { heldAnswer, useResource, type Resource } from "./use-resource.ts";

/**
 * The person's other devices open now, from the list of their devices the tab holds (one key with
 * the account's, "my-devices"), which the live connection's ids pick from: opening the menu asks
 * nothing while that list names every device connected (D480 §3). A device connected that it does
 * not name, or no list held, reads the list again, behind what it holds. While isReconnecting, a
 * read not answered, or failed, shows the list held (D564 §4).
 */
export function useDevicesOpen(
  connectedDeviceIds: readonly number[],
  isReconnecting: boolean,
): Resource<Device[]> {
  const { api } = useServices();
  const held = heldAnswer<readonly Device[]>(myDevicesKey);
  const isEveryOneNamed =
    held !== undefined && connectedDeviceIds.every((id) => held.some((each) => each.id === id));
  const [listed] = useResource(myDevicesKey, () => listMyDevices(api), {
    given: isEveryOneNamed ? { value: held } : undefined,
  });
  const kept = listed.state !== "loaded" && isReconnecting ? held : undefined;
  return useMemo(() => {
    if (listed.state === "loaded") {
      return { state: "loaded", value: otherDevicesOpen(listed.value, connectedDeviceIds) };
    }
    return kept === undefined
      ? listed
      : { state: "loaded", value: otherDevicesOpen(kept, connectedDeviceIds) };
  }, [listed, kept, connectedDeviceIds]);
}

/**
 * The person's devices open now but this one, as their list orders them, most recently used first,
 * and none in the Watch Together session they are in, whose cast would take it from the session (D384
 * §3: the owner's S23, casting to the Mac that hosted the session, ended it, 2026-10-30).
 */
export function otherDevicesOpen(
  devices: readonly Device[],
  connectedDeviceIds: readonly number[],
): Device[] {
  return devices.filter(
    (device) =>
      !device.isThisDevice &&
      device.isSignedIn &&
      device.isInYourWatchTogether !== true &&
      connectedDeviceIds.includes(device.id),
  );
}
