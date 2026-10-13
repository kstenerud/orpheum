// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";

import {
  buttonWithText,
  click,
  enter,
  mount,
  servedBy,
  submit,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { television } from "./test-fixtures.ts";
import { withServices } from "./test-services.tsx";
import { ThisDevice } from "./this-device.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

const laptop = television({
  id: 5,
  name: "Karl's laptop",
  isThisDevice: true,
  audioOutput: "built_in_speakers",
});

describe("the This device page (D222 §1)", () => {
  it("shows this device alone, and changes how it makes sound and its name", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "GET /me/devices": [200, { devices: [television(), laptop] }],
        "PUT /me/devices/5": [204, undefined],
      },
      heard,
    );
    services.session.signIn("token", { deviceId: 5 });
    mounted = await mount(withServices(<ThisDevice />, services));
    const text = mounted.container.textContent;
    expect(text).toContain("Karl's laptop");
    expect(text).not.toContain("Living room TV");
    const current = mounted.container.querySelector('button[aria-pressed="true"]');
    expect(current?.textContent).toContain("Built-in speakers");
    await click(
      [...mounted.container.querySelectorAll("button")].find((each) =>
        each.textContent.startsWith("Headphones"),
      ),
    );
    await click(buttonWithText(mounted.container, "Rename"));
    await enter(mounted.container.querySelector("input"), "Desk");
    await submit(mounted.container.querySelector("form"));
    expect(
      heard.filter((each) => each.methodAndPath === "PUT /me/devices/5").map((each) => each.body),
    ).toEqual([{ audioOutput: "headphones" }, { name: "Desk" }]);
  });

  it("says so when this browser is not one of the person's devices", async () => {
    const services = servedBy({ "GET /me/devices": [200, { devices: [television()] }] });
    services.session.signIn("token");
    mounted = await mount(withServices(<ThisDevice />, services));
    expect(mounted.container.textContent).toContain("This browser is not one of your devices");
  });
});
