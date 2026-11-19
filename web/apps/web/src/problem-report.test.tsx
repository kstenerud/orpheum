// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";

import { AppShell } from "./app-shell.tsx";
import { deviceFacts, playbackNow } from "./problem-report.tsx";
import {
  bodyField,
  buttonWithText,
  click,
  enter,
  mount,
  refusal,
  servedBy,
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

/** The shell, signed in as karl unless signedIn is false, its server answering answers. */
async function shell(answers: Answers, heard: Heard[], isSignedIn = true): Promise<Mounted> {
  mounted = await mount(
    withServices(
      <AppShell
        isAdmin={false}
        canHostWatchTogether={false}
        currentScreen="films"
        onSignOut={() => undefined}
        signedInAs={isSignedIn ? { name: "karl", deviceName: "Laptop" } : undefined}
      >
        <p>screen</p>
      </AppShell>,
      servedBy({ "GET /libraries": [200, { libraries: [] }], ...answers }, heard),
    ),
  );
  return mounted;
}

/** A sent fact's name, or undefined for something that is not one. */
function factName(fact: unknown): unknown {
  return typeof fact === "object" && fact !== null ? Reflect.get(fact, "name") : undefined;
}

/** The report's dialog, once open. */
function dialogOf(on: Mounted): HTMLElement | null {
  return on.container.querySelector<HTMLElement>('dialog[aria-label="Report a problem"]');
}

/** Opens the menu, chooses Report a problem, and writes words in its dialog. */
async function reportFromTheMenu(on: Mounted, words: string): Promise<HTMLElement | null> {
  await click(on.container.querySelector(".app-shell__menu-button"));
  await click(buttonWithText(on.container, "Report a problem"));
  const dialog = dialogOf(on);
  await enter(dialog?.querySelector("textarea"), words);
  return dialog;
}

describe("Report a problem (D524 §2)", () => {
  it("sends the person's words from the menu with the page and the device's facts, and says it went", async () => {
    const heard: Heard[] = [];
    const on = await shell({ "POST /problem-reports": [201, { id: 7 }] }, heard);
    const dialog = await reportFromTheMenu(on, "  the film stalls at the start  ");
    await click(buttonWithText(dialog ?? on.container, "Send"));
    const sent = heard.find((each) => each.methodAndPath === "POST /problem-reports");
    expect(sent).toBeDefined();
    expect(bodyField(sent!, "words")).toBe("the film stalls at the start");
    expect(bodyField(sent!, "page")).toBe("/films");
    expect(bodyField(sent!, "playback")).toBeUndefined();
    const facts: unknown = bodyField(sent!, "device");
    const names = Array.isArray(facts) ? facts.map((fact: unknown) => factName(fact)) : [];
    expect(names).toEqual(expect.arrayContaining(["Screen", "Window", "Language", "Time zone"]));
    expect(dialogOf(on)?.textContent).toContain(
      "This household's admin finds it on the System page",
    );
  });

  it("says why it was refused past the hour's reports, and keeps the words to send again", async () => {
    const heard: Heard[] = [];
    const on = await shell({ "POST /problem-reports": refusal(429, "rate_limited") }, heard);
    const dialog = await reportFromTheMenu(on, "no sound");
    await click(buttonWithText(dialog ?? on.container, "Send"));
    expect(dialog?.querySelector('[role="alert"]')?.textContent).toBe(
      "You have reported many problems in the last hour. Wait a while, then report again.",
    );
    expect(dialog?.querySelector("textarea")?.value).toBe("no sound");
  });

  it("is not offered to a page with no one signed in, whom the server refuses", async () => {
    const on = await shell({}, []);
    await click(on.container.querySelector(".app-shell__menu-button"));
    expect(buttonWithText(on.container, "Report a problem")).toBeDefined();
    on.unmount();
    mounted = undefined;
    const signedOut = await shell({}, [], false);
    await click(signedOut.container.querySelector(".app-shell__menu-button"));
    expect(buttonWithText(signedOut.container, "Report a problem")).toBeUndefined();
  });
});

describe("what a report says of the device and the player", () => {
  it("names the screen and the window by their sizes", () => {
    const facts = deviceFacts();
    expect(facts.find((fact) => fact.name === "Screen")?.value).toMatch(/^\d+×\d+$/u);
    expect(facts.find((fact) => fact.name === "Window")?.value).toBe(
      `${String(window.innerWidth)}×${String(window.innerHeight)}`,
    );
  });

  it("says where the film stands, how it plays and whether it is paused", () => {
    const video = document.createElement("video");
    video.currentTime = 90.5;
    expect(
      playbackNow(3, video, { delivery: "converted", qualityKbps: 4000, linkKbps: 12_000 }),
    ).toEqual({
      fileId: 3,
      positionMs: 90_500,
      quality: "converted, 4000 kb/s over 12000 kb/s",
      state: "paused",
    });
    expect(playbackNow(3, null, undefined)).toEqual({
      fileId: 3,
      positionMs: 0,
      quality: "",
      state: "",
    });
  });
});
