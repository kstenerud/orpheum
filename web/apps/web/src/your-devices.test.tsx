// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

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
