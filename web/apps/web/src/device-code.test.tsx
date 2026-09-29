// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it, vi } from "vitest";

import { Account } from "./account.tsx";
import {
  mount,
  refusal,
  servedBy,
  submit,
  type Answers,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
  vi.restoreAllMocks();
  window.sessionStorage.clear();
});

/** The account page, served by answers, with a code made for a new device (its name left empty). */
async function accountWithCodeMade(answers: Answers, heard: Heard[] = []): Promise<Mounted> {
  const shown = await mount(
    withServices(
      <Account />,
      servedBy(
        {
          "GET /me/devices": [200, { devices: [] }],
          "POST /device-codes": [200, { secret: "abcd-efgh", expiresAt: Date.now() + 600_000 }],
          ...answers,
        },
        heard,
      ),
    ),
  );
  const nameInput = shown.container.querySelector("input:not([type=password])");
  if (!(nameInput instanceof HTMLInputElement)) {
    throw new Error("no device name field");
  }
  await submit(nameInput.form);
  return shown;
}

describe("a device code on the account page", () => {
  it("says so when asking whether it was used is refused, and keeps showing it", async () => {
    mounted = await accountWithCodeMade({ "GET /me/device-code": refusal(503, "migrating") });
    expect(mounted.container.querySelector('[role="alert"]')?.textContent).toBe(
      "The server is updating. Try again in a minute.",
    );
    expect(mounted.container.textContent).toContain("abcd-efgh");
  });

  it("goes once it is used, and the device list is asked for again", async () => {
    const heard: Heard[] = [];
    mounted = await accountWithCodeMade(
      { "GET /me/device-code": [200, { isWaiting: false }] },
      heard,
    );
    expect(mounted.container.textContent).not.toContain("abcd-efgh");
    expect(heard.filter((request) => request.methodAndPath === "GET /me/devices")).toHaveLength(2);
  });

  it("is shown when the tab refuses to keep it", async () => {
    // Vitest fails the run on the rejection a refused write would leave unhandled.
    const refusedWrite = vi.spyOn(window.sessionStorage, "setItem").mockImplementation(() => {
      throw new DOMException("the quota is spent", "QuotaExceededError");
    });
    mounted = await accountWithCodeMade({ "GET /me/device-code": [200, { isWaiting: true }] });
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(mounted.container.textContent).toContain("abcd-efgh");
    expect(refusedWrite).toHaveBeenCalled();
  });
});
