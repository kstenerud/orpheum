// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";

import { chooseOk, chosenOkButton, okChoiceKey } from "./controller-choice.ts";
import { DeviceTestsOfferedContext } from "./lab-offered.ts";
import {
  buttonWithText,
  click,
  enter,
  fire,
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

const laptop = television({ id: 5, name: "Karl's laptop", isThisDevice: true });

describe("the This device page", () => {
  it("shows this device alone, and changes its name, asking nothing of how it makes sound (D336 §4)", async () => {
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
    expect(text).not.toContain("Headphones");
    expect(mounted.container.querySelector("button[aria-pressed]")).toBeNull();
    await click(buttonWithText(mounted.container, "Rename"));
    await enter(mounted.container.querySelector("input"), "Desk");
    await submit(mounted.container.querySelector("form"));
    expect(
      heard.filter((each) => each.methodAndPath === "PUT /me/devices/5").map((each) => each.body),
    ).toEqual([{ name: "Desk" }]);
  });

  it("saves a new name as it is left, with no Save, and says Saved (Q5)", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "GET /me/devices": [200, { devices: [laptop] }],
        "PUT /me/devices/5": [204, undefined],
      },
      heard,
    );
    services.session.signIn("token", { deviceId: 5 });
    mounted = await mount(withServices(<ThisDevice />, services));
    await click(buttonWithText(mounted.container, "Rename"));
    const field = mounted.container.querySelector("input");
    expect(document.activeElement).toBe(field);
    expect(buttonWithText(mounted.container, "Save")).toBeUndefined();
    expect(mounted.container.textContent).not.toContain("Saved");
    await enter(field, "Desk");
    await fire(field, new FocusEvent("focusout", { bubbles: true }));
    expect(
      heard.filter((each) => each.methodAndPath === "PUT /me/devices/5").map((each) => each.body),
    ).toEqual([{ name: "Desk" }]);
    expect(mounted.container.querySelector("output")?.textContent).toBe("Saved");
  });

  it("saves once on Enter and gives focus back to Rename; an unchanged name saves nothing", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "GET /me/devices": [200, { devices: [laptop] }],
        "PUT /me/devices/5": [204, undefined],
      },
      heard,
    );
    services.session.signIn("token", { deviceId: 5 });
    mounted = await mount(withServices(<ThisDevice />, services));
    await click(buttonWithText(mounted.container, "Rename"));
    await submit(mounted.container.querySelector("form"));
    expect(heard.filter((each) => each.methodAndPath === "PUT /me/devices/5")).toEqual([]);
    expect(document.activeElement).toBe(buttonWithText(mounted.container, "Rename"));
    await click(buttonWithText(mounted.container, "Rename"));
    const field = mounted.container.querySelector("input");
    await enter(field, "Desk");
    await submit(mounted.container.querySelector("form"));
    await fire(field, new FocusEvent("focusout", { bubbles: true }));
    expect(heard.filter((each) => each.methodAndPath === "PUT /me/devices/5")).toHaveLength(1);
    expect(document.activeElement).toBe(buttonWithText(mounted.container, "Rename"));
  });

  it("says so when this browser is not one of the person's devices", async () => {
    const services = servedBy({ "GET /me/devices": [200, { devices: [television()] }] });
    services.session.signIn("token");
    mounted = await mount(withServices(<ThisDevice />, services));
    expect(mounted.container.textContent).toContain("This browser is not one of your devices");
  });

  it("keeps which face button of a controller is OK in the browser, automatic until chosen", async () => {
    chooseOk("automatic");
    const services = servedBy({ "GET /me/devices": [200, { devices: [laptop] }] });
    services.session.signIn("token");
    mounted = await mount(withServices(<ThisDevice />, services));
    const choice = (value: string): HTMLInputElement | null =>
      mounted?.container.querySelector<HTMLInputElement>(`input[value="${value}"]`) ?? null;
    expect(choice("automatic")?.checked).toBe(true);
    expect(mounted.container.textContent).toContain("The right button");
    await click(choice("right"));
    expect(choice("right")?.checked).toBe(true);
    expect(localStorage.getItem(okChoiceKey)).toBe("right");
    expect(chosenOkButton()).toBe("right");
    await click(choice("automatic"));
    expect(localStorage.getItem(okChoiceKey)).toBeNull();
    expect(chosenOkButton()).toBeUndefined();
  });

  // The owner, 2026-11-04 (ui-audit-2026-11-04.md Q8): Device tests move from the menu to here,
  // behind "For testers", shown as the menu showed them: while the server offers them.
  it("leads to Device tests under For testers only while the server offers them", async () => {
    const drawnWith = async (isOffered: boolean | undefined): Promise<Element | null> => {
      const services = servedBy({ "GET /me/devices": [200, { devices: [laptop] }] });
      services.session.signIn("token");
      mounted?.unmount();
      mounted = await mount(
        withServices(
          <DeviceTestsOfferedContext value={isOffered}>
            <ThisDevice />
          </DeviceTestsOfferedContext>,
          services,
        ),
      );
      return mounted.container.querySelector('a[href="/lab"]');
    };
    expect((await drawnWith(true))?.textContent).toBe("Device tests");
    expect(mounted?.container.textContent).toContain("For testers");
    expect(await drawnWith(false)).toBeNull();
    expect(mounted?.container.textContent).not.toContain("For testers");
    expect(await drawnWith(undefined)).toBeNull();
  });
});
