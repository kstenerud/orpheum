// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act } from "react";
import { expect, it } from "vitest";

import { buttonWithText, click, mount } from "./test-dom.tsx";
import { television } from "./test-fixtures.ts";
import { withServices } from "./test-services.tsx";
import { DeviceList, YourDevices } from "./your-devices.tsx";

it("removes a device only when asked again, its button in the irreversible red", async () => {
  const removed: number[] = [];
  const mounted = await mount(
    withServices(
      <DeviceList
        devices={[television({ id: 5, isSignedIn: true })]}
        onRename={() => undefined}
        onRemove={(device) => {
          removed.push(device.id);
        }}
      />,
    ),
  );
  const remove = buttonWithText(mounted.container, "Remove");
  expect(remove?.className).toBe("irreversible-outline");
  await click(remove);
  expect(removed).toEqual([]);
  expect(mounted.container.textContent).toContain(
    "Living room TV is signed out, and signs in again only with a password or a code it shows.",
  );
  await click(buttonWithText(mounted.container, "Remove it"));
  expect(removed).toEqual([5]);
  mounted.unmount();
});

/** count devices, the server's order, most recently used first; the first is this one. */
function devicesUsed(count: number): ReturnType<typeof television>[] {
  return Array.from({ length: count }, (_, at) =>
    television({ id: at + 1, name: `Device ${String(at + 1)}`, isThisDevice: at === 0 }),
  );
}

/** The names of the devices listed in container, in its order. */
function listedNames(container: HTMLElement): string[] {
  return [...container.querySelectorAll(".your-devices__device .your-devices__name")].map(
    (name) => name.firstChild?.textContent ?? "",
  );
}

it("shows the five most recently used, and All devices (N) opens every one in their place", async () => {
  const mounted = await mount(
    withServices(
      <YourDevices
        devices={{ state: "loaded", value: devicesUsed(7) }}
        onChanged={() => undefined}
        canManage
      />,
    ),
  );
  expect(listedNames(mounted.container)).toEqual([
    "Device 1",
    "Device 2",
    "Device 3",
    "Device 4",
    "Device 5",
  ]);
  const all = mounted.container.querySelector("details.your-devices__all");
  expect(all?.querySelector("summary")?.textContent).toBe("All devices (7)");
  expect(buttonWithText(mounted.container, "Sign out everywhere else")).toBeDefined();
  await act(async () => {
    all?.setAttribute("open", "");
    all?.dispatchEvent(new Event("toggle"));
    await Promise.resolve();
  });
  expect(listedNames(mounted.container)).toEqual(
    Array.from({ length: 7 }, (_, at) => `Device ${String(at + 1)}`),
  );
  mounted.unmount();
});

it("shows every device, and no All devices, while there are five or fewer", async () => {
  const mounted = await mount(
    withServices(
      <YourDevices
        devices={{ state: "loaded", value: devicesUsed(5) }}
        onChanged={() => undefined}
        canManage
      />,
    ),
  );
  expect(listedNames(mounted.container)).toHaveLength(5);
  expect(mounted.container.querySelector("details")).toBeNull();
  mounted.unmount();
});

it("names each device a code activated, and asks the person to remove one they don't know (M4)", async () => {
  const mounted = await mount(
    withServices(
      <YourDevices
        devices={{
          state: "loaded",
          value: [television({ id: 5, isByCode: true }), television({ id: 6, name: "Phone" })],
        }}
        onChanged={() => undefined}
        canManage
      />,
    ),
  );
  expect(mounted.container.querySelectorAll(".your-devices__by-code")).toHaveLength(1);
  expect(mounted.container.querySelector(".your-devices__by-code-notice")?.textContent).toContain(
    "If you don't know one of those below, remove it.",
  );
  mounted.unmount();
});
