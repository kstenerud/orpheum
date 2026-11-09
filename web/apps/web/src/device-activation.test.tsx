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
  settleUntil,
  submit,
  type Answers,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { createNavigation } from "./navigation.ts";
import { parseRoute, routeHref } from "./router.ts";
import { fakePlace, withServices } from "./test-services.tsx";

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

/** The person signed in, as GET /me answers. */
const owner = { id: 1, name: "owner", role: "admin", isAdmin: true, canHostWatchTogether: true };

/** The account page, served by answers, with a television's code and name entered. */
async function accountWithCodeEntered(answers: Answers, heard: Heard[]): Promise<Mounted> {
  const shown = await mount(
    withServices(
      <Account />,
      servedBy(
        {
          "GET /me/devices": [200, { devices: [] }],
          "GET /me": [200, { ...owner, canManageAccount: true }],
          "GET /me/own-link": refusal(404, "notFound"),
          "GET /me/preferences": [
            200,
            { isDialogueBoostOn: false, isLevellingOn: true, isAutoplayOn: true },
          ],
          "GET /libraries": [200, { libraries: [] }],
          "POST /pairings/code": [
            200,
            {
              checkCode: "wxyz-0123",
              deviceName: "Living room TV",
              platform: "Tizen",
              askedSecondsAgo: 30,
            },
          ],
          "POST /pairings/check": [204, undefined],
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
    // What asked for the code, and when (the device code audit's M3); not a while ago.
    expect(container.textContent).toContain(
      "It says it is Tizen, and asked for its code 30 s ago.",
    );
    expect(container.querySelector(".device-activation__long-ago")).toBeNull();
    await click(buttonWithText(container, "Yes, they match"));
    expect(container.textContent).toContain("Living room TV is activated and signed in.");
    expect(
      heard.filter(
        (request) =>
          !request.methodAndPath.startsWith("GET /me") &&
          request.methodAndPath !== "GET /libraries",
      ),
    ).toEqual([
      {
        methodAndPath: "POST /pairings/code",
        body: { code: "ABCD EFGH", deviceName: "Living room TV" },
      },
      { methodAndPath: "POST /pairings/check", body: { code: "ABCD EFGH", isMatch: true } },
    ]);
    expect(heard.filter((request) => request.methodAndPath === "GET /me/devices")).toHaveLength(2);
  });

  it("warns where the code is entered, and again when the code was asked for a while ago (M3)", async () => {
    mounted = await accountWithCodeEntered(
      {
        "POST /pairings/code": [
          200,
          { checkCode: "wxyz-0123", deviceName: "TV", platform: "web", askedSecondsAgo: 300 },
        ],
      },
      [],
    );
    expect(mounted.container.querySelector(".device-activation__long-ago")?.textContent).toContain(
      "That was a while ago.",
    );
    expect(mounted.container.textContent).toContain("asked for its code 5 min ago");
    mounted.unmount();
    mounted = await mount(
      withServices(<ActivateDevice onActivated={() => undefined} />, servedBy({})),
    );
    expect(mounted.container.textContent).toContain(
      "Only enter a code shown on a screen in front of you.",
    );
  });

  it("activates nothing when the check codes do not match", async () => {
    const heard: Heard[] = [];
    mounted = await accountWithCodeEntered({}, heard);
    await click(buttonWithText(mounted.container, "They don't match"));
    expect(mounted.container.textContent).toContain("Nothing was activated.");
    expect(heard.at(-1)).toEqual({
      methodAndPath: "POST /pairings/check",
      body: { code: "ABCD EFGH", isMatch: false },
    });
    expect(heard.filter((request) => request.methodAndPath === "GET /me/devices")).toHaveLength(1);
  });

  it("says when no device shows the code entered", async () => {
    mounted = await accountWithCodeEntered({ "POST /pairings/code": refusal(404, "notFound") }, []);
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
          { "POST /pairings/code": [200, { checkCode: "wxyz-0123", deviceName: "Gran's TV" }] },
          heard,
        ),
      ),
    );
    await type(mounted.container, "Code", "abcd-efgh");
    await type(mounted.container, "Its name", "Gran's TV");
    await submit(mounted.container.querySelector("form"));
    expect(heard).toEqual([
      {
        methodAndPath: "POST /pairings/code",
        body: { code: "abcd-efgh", deviceName: "Gran's TV", userId: 7 },
      },
    ]);
  });

  it("names the loader's short address under a discovery, which a TV remote types (D293 §2)", async () => {
    mounted = await mount(
      withServices(<ActivateDevice onActivated={() => undefined} />, {
        ...servedBy({}),
        codeAddress: "app.orpheum-app.com",
      }),
    );
    expect(mounted.container.textContent).toContain(
      "open app.orpheum-app.com and choose Get a code",
    );
  });
});

describe("activating a device whose QR code was scanned (D321)", () => {
  const scannedAddress = "/account?code=abcd-efgh&key=rTBVo-NI60s-RZ3ENfKqkw";

  it("reads the scanned code from the account's address, and writes it back the same", () => {
    const route = parseRoute("/account", "?code=abcd-efgh&key=rTBVo-NI60s-RZ3ENfKqkw");
    expect(route).toEqual({
      screen: "account",
      scanned: { code: "abcd-efgh", keyHash: "rTBVo-NI60s-RZ3ENfKqkw" },
    });
    expect(routeHref(route)).toBe(scannedAddress);
    expect(parseRoute("/account", "?code=abcd-efgh")).toEqual({ screen: "account" });
  });

  it("enters it at once with the key's hash, asks one question with no check code, and activates on a tap", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "GET /me/devices": [200, { devices: [] }],
        "GET /me": [200, { ...owner, canManageAccount: true }],
        "GET /me/own-link": refusal(404, "notFound"),
        "GET /me/preferences": [
          200,
          { isDialogueBoostOn: false, isLevellingOn: true, isAutoplayOn: true },
        ],
        "GET /libraries": [200, { libraries: [] }],
        "POST /pairings/code": [
          200,
          { checkCode: "", deviceName: "Tizen", platform: "Tizen", askedSecondsAgo: 5 },
        ],
        "POST /pairings/check": [204, undefined],
      },
      heard,
    );
    const navigation = createNavigation(fakePlace(scannedAddress));
    mounted = await mount(withServices(<Account />, { ...services, navigation }));
    const { container } = mounted;
    await settleUntil(() => container.textContent.includes("Activate Tizen?"));
    expect(heard.find((each) => each.methodAndPath === "POST /pairings/code")?.body).toEqual({
      code: "abcd-efgh",
      deviceName: "",
      keyHash: "rTBVo-NI60s-RZ3ENfKqkw",
    });
    expect(container.textContent).toContain("Activate Tizen? It will be signed in as you.");
    expect(container.textContent).toContain("Only activate a device you can see in front of you");
    expect(container.querySelector(".device-activation__check-code")).toBeNull();
    await click(buttonWithText(container, "Activate"));
    await settleUntil(() => container.textContent.includes("Tizen is activated and signed in."));
    expect(heard.find((each) => each.methodAndPath === "POST /pairings/check")?.body).toEqual({
      code: "abcd-efgh",
      isMatch: true,
    });
    // The address no longer carries the code, so a reload enters nothing again.
    expect(navigation.route()).toEqual({ screen: "account" });
  });
});
