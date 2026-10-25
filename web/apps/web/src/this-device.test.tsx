// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";

import { chooseOk, chosenOkButton, okChoiceKey } from "./controller-choice.ts";
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
});
