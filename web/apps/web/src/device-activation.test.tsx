// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";

import { Account } from "./account.tsx";
import { ActivateDevice } from "./device-activation.tsx";
import {
  buttonWithText,
  click,
  enter,
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
});

/** Types text into the input labelled label, as a person would. */
async function type(container: HTMLElement, label: string, text: string): Promise<void> {
  await enter(
    [...container.querySelectorAll("label")]
      .find((each) => each.textContent.startsWith(label))
      ?.querySelector("input"),
    text,
  );
}

/** The account page, served by answers, with a television's code and name entered. */
async function accountWithCodeEntered(answers: Answers, heard: Heard[]): Promise<Mounted> {
  const shown = await mount(
    withServices(
      <Account />,
      servedBy(
        {
          "GET /me/devices": [200, { devices: [] }],
          "POST /activations": [200, { checkCode: "wxyz-0123", deviceName: "Living room TV" }],
          "POST /activations/answer": [204, undefined],
          ...answers,
        },
        heard,
      ),
    ),
  );
  await type(shown.container, "Code", "ABCD EFGH");
  await type(shown.container, "Its name", "Living room TV");
  await submit(shown.container.querySelector(".device-activation form"));
  return shown;
}

describe("activating a device by the code it shows (D288)", () => {
  it("shows the check code with the warning, and only Yes activates it and lists it", async () => {
    const heard: Heard[] = [];
    mounted = await accountWithCodeEntered({}, heard);
    const { container } = mounted;
    expect(container.querySelector(".device-activation__check-code")?.textContent).toBe(
      "wxyz-0123",
    );
    expect(container.textContent).toContain("Look at the device's screen yourself.");
    expect(container.textContent).toContain("Never go on what someone tells you");
    await click(buttonWithText(container, "Yes, they match"));
    expect(container.textContent).toContain("Living room TV is activated and signed in.");
    expect(heard.filter((request) => request.methodAndPath !== "GET /me/devices")).toEqual([
      {
        methodAndPath: "POST /activations",
        body: { code: "ABCD EFGH", deviceName: "Living room TV" },
      },
      { methodAndPath: "POST /activations/answer", body: { code: "ABCD EFGH", isMatch: true } },
    ]);
    expect(heard.filter((request) => request.methodAndPath === "GET /me/devices")).toHaveLength(2);
  });

  it("activates nothing when the check codes do not match", async () => {
    const heard: Heard[] = [];
    mounted = await accountWithCodeEntered({}, heard);
    await click(buttonWithText(mounted.container, "They don't match"));
    expect(mounted.container.textContent).toContain("Nothing was activated.");
    expect(heard.at(-1)).toEqual({
      methodAndPath: "POST /activations/answer",
      body: { code: "ABCD EFGH", isMatch: false },
    });
    expect(heard.filter((request) => request.methodAndPath === "GET /me/devices")).toHaveLength(1);
  });

  it("says when no device shows the code entered", async () => {
    mounted = await accountWithCodeEntered({ "POST /activations": refusal(404, "not_found") }, []);
    expect(mounted.container.querySelector('[role="alert"]')?.textContent).toContain(
      "No device is showing that code.",
    );
    expect(mounted.container.querySelector(".device-activation__check")).toBeNull();
  });

  it("activates a device for the person an admin is looking at (D288 §2)", async () => {
    const heard: Heard[] = [];
    mounted = await mount(
      withServices(
        <ActivateDevice userId={7} onActivated={() => undefined} />,
        servedBy(
          { "POST /activations": [200, { checkCode: "wxyz-0123", deviceName: "Gran's TV" }] },
          heard,
        ),
      ),
    );
    await type(mounted.container, "Code", "abcd-efgh");
    await type(mounted.container, "Its name", "Gran's TV");
    await submit(mounted.container.querySelector("form"));
    expect(heard).toEqual([
      {
        methodAndPath: "POST /activations",
        body: { code: "abcd-efgh", deviceName: "Gran's TV", userId: 7 },
      },
    ]);
  });
});
