// SPDX-License-Identifier: MPL-2.0

/**
 * Settings' pages as a real browser runs them (D538 §11, pages-spec-3): the index leads to a
 * section's page and its trail back up; a search leads to a setting's row, focus arriving there; a
 * Stepper saves at each step, and a text field on Enter or on leaving it, never as the page closes;
 * a changed setting goes back to its default by its row's Use the default, asked first; by a mouse,
 * a finger, the keyboard and a remote's arrows and OK alike, at a phone's, a computer's
 * and a television's width, the ring showing and focus never on nothing.
 */

import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import type { JSX } from "react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { page, userEvent } from "vitest/browser";

import { startInput, type Input } from "./input-commands.ts";
import type { Setting } from "./server-settings.tsx";
import { useRoute } from "./services.tsx";
import { Settings } from "./settings.tsx";
import {
  bodyField,
  mount,
  servedBy,
  settleAnswers,
  type Answer,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { isRingShown, partWidths, tapOn } from "./test-parts.ts";
import { withServices } from "./test-services.tsx";

let input: Input | undefined;
let mounted: Mounted | undefined;

/** A General setting as the server sends it, important and at its starting value. */
function generalSetting(
  changed: Partial<Setting> & Pick<Setting, "key" | "kind" | "value">,
): Setting {
  return {
    description: "",
    startingValue: changed.value,
    isModified: false,
    isImportant: true,
    section: "general",
    setAt: 0,
    ...changed,
  };
}

/** A Stepper's setting, and a text field's. */
const mostTitles = generalSetting({
  key: "continueWatchingMaxWorks",
  kind: "integer",
  value: 20,
  minimum: 1,
  maximum: 50,
});
const timeZone = generalSetting({ key: "timeZone", kind: "string", value: "Europe/Berlin" });

/** Settings' pages at the route the tab is at, as the app shows them. */
function SettingsHere(): JSX.Element {
  const route = useRoute();
  return (
    <main>
      {route.screen === "settings" ? <Settings page={route.page} setting={route.setting} /> : null}
    </main>
  );
}

/**
 * Mounts Settings' pages at its index, every request they make answered, settings those it reads, and
 * answers more; heard keeps them.
 */
async function mountSettings(
  heard: Heard[] = [],
  settings: readonly Setting[] = [mostTitles, timeZone],
  answers: Readonly<Record<string, Answer>> = {},
): Promise<Mounted> {
  const services = servedBy(
    {
      ...answers,
      "GET /config": [200, { settings }],
      "GET /config/unused": [200, { unusedSettings: [] }],
      "GET /secrets": [200, { secrets: [] }],
      "GET /needs-attention": [200, { pages: {} }],
      "PUT /config": [200, { settings: [{ ...mostTitles, value: 21 }, timeZone] }],
    },
    heard,
    "token",
  );
  services.navigation.go({ screen: "settings" });
  return await mount(withServices(<SettingsHere />, services));
}

/** The page's heading. */
function heading(): string | undefined {
  return document.querySelector("main h1")?.textContent;
}

/** The link named text, wherever it shows. */
function link(text: string): HTMLElement {
  const found = [...document.querySelectorAll<HTMLElement>("main a")].find((each) =>
    each.textContent.trim().startsWith(text),
  );
  if (found === undefined) {
    throw new Error(`no link ${text}`);
  }
  return found;
}

/** The trail's way up to Settings. */
function trailUp(): HTMLElement {
  const up = document.querySelector<HTMLElement>("main .up-trail a");
  if (up === null) {
    throw new Error("no trail");
  }
  return up;
}

/** The button named label. */
function button(label: string): HTMLElement {
  const found = document.querySelector<HTMLElement>(`main button[aria-label="${label}"]`);
  if (found === null) {
    throw new Error(`no button ${label}`);
  }
  return found;
}

/** The index's search field. */
function searchField(): HTMLInputElement {
  const found = document.querySelector<HTMLInputElement>('main input[type="search"]');
  if (found === null) {
    throw new Error("no search");
  }
  return found;
}

/** Whether focus is on something, not on nothing. */
function isFocusOnSomething(): boolean {
  return document.activeElement !== null && document.activeElement !== document.body;
}

/** Whether focus is on the first setting of General, where it arrives as the page opens. */
function isOnFirstSetting(): boolean {
  return (document.activeElement?.closest("#setting-continueWatchingMaxWorks") ?? null) !== null;
}

/** A remote's arrows. */
type Arrow = "ArrowDown" | "ArrowUp" | "ArrowLeft" | "ArrowRight";

/** Presses a remote's arrow until isThere, at most presses times, focus never on nothing; whether it is. */
async function arrowUntil(arrow: Arrow, isThere: () => boolean, presses = 30): Promise<boolean> {
  for (let press = 0; press < presses; press += 1) {
    if (isThere()) {
      return true;
    }
    await userEvent.keyboard(`{${arrow}}`);
    expect(isFocusOnSomething()).toBe(true);
  }
  return isThere();
}

/** Presses a remote's arrow until focus is on target, at most 30 times; whether it got there. */
async function arrowTo(arrow: Arrow, target: () => Element): Promise<boolean> {
  return await arrowUntil(arrow, () => document.activeElement === target());
}

/** The PUT /config bodies heard, each its values by key. */
function savedValues(heard: readonly Heard[]): unknown[] {
  return heard
    .filter((each) => each.methodAndPath === "PUT /config")
    .map((each) => bodyField(each, "valuesByKey"));
}

beforeEach(async () => {
  input = startInput({ goBack: () => false, isNavigating: () => true });
  await page.viewport(1280, 800);
});

afterEach(() => {
  input?.stop();
  input = undefined;
  mounted?.unmount();
  mounted = undefined;
  localStorage.clear();
});

describe("Settings' index and a section's page", () => {
  it.each(partWidths)(
    "leads to General and back up its trail at %i px, by a click, a tap, the keyboard and a remote",
    async (width) => {
      await page.viewport(width, 800);
      mounted = await mountSettings();
      await expect.poll(heading).toBe("Settings");
      // A click, and the trail's way up.
      await userEvent.click(link("General"));
      await expect.poll(heading).toBe("General");
      await userEvent.click(trailUp());
      await expect.poll(heading).toBe("Settings");
      // A tap.
      tapOn(link("General"));
      await expect.poll(heading).toBe("General");
      tapOn(trailUp());
      await expect.poll(heading).toBe("Settings");
      // The keyboard: Tab from the search to General, Enter; focus arrives on its first setting
      // (the owner, 2026-11-23, revamp question 20); Shift+Tab back to the trail, Enter. The index
      // marks no arrival and leaves focus where the page left took it, so the first press after it
      // opens is what must land on something.
      searchField().focus();
      await userEvent.keyboard("{Tab}{Tab}");
      expect(document.activeElement).toBe(link("General"));
      expect(isRingShown()).toBe(true);
      await userEvent.keyboard("{Enter}");
      await expect.poll(heading).toBe("General");
      await expect.poll(isOnFirstSetting).toBe(true);
      expect(isRingShown()).toBe(true);
      for (let press = 0; press < 4 && document.activeElement !== trailUp(); press += 1) {
        await userEvent.keyboard("{Shift>}{Tab}{/Shift}");
        expect(isFocusOnSomething()).toBe(true);
      }
      expect(document.activeElement).toBe(trailUp());
      expect(isRingShown()).toBe(true);
      await userEvent.keyboard("{Enter}");
      await expect.poll(heading).toBe("Settings");
      await userEvent.keyboard("{Tab}");
      expect(isFocusOnSomething()).toBe(true);
      // A remote: Down from the search to General, OK; Up to the trail, OK; every press on something.
      searchField().focus();
      expect(await arrowTo("ArrowDown", () => link("General"))).toBe(true);
      expect(isRingShown()).toBe(true);
      await userEvent.keyboard("{Enter}");
      await expect.poll(heading).toBe("General");
      await expect.poll(isOnFirstSetting).toBe(true);
      expect(await arrowTo("ArrowUp", trailUp)).toBe(true);
      expect(isRingShown()).toBe(true);
      await userEvent.keyboard("{Enter}");
      await expect.poll(heading).toBe("Settings");
      await userEvent.keyboard("{ArrowDown}");
      expect(isFocusOnSomething()).toBe(true);
    },
  );

  it("leads a search to the setting's row, focus arriving there, by the keyboard and by a remote", async () => {
    mounted = await mountSettings();
    await expect.poll(heading).toBe("Settings");
    for (const press of ["{Tab}", "{ArrowDown}"]) {
      await userEvent.click(searchField());
      await userEvent.keyboard("time");
      await expect.poll(() => link("Time zone").textContent).toContain("General");
      await userEvent.keyboard(press);
      expect(document.activeElement).toBe(link("Time zone"));
      expect(isRingShown()).toBe(true);
      await userEvent.keyboard("{Enter}");
      await expect.poll(heading).toBe("General");
      await expect
        .poll(() => document.activeElement?.closest("#setting-timeZone") !== null)
        .toBe(true);
      expect(isFocusOnSomething()).toBe(true);
      await userEvent.click(trailUp());
      await expect.poll(heading).toBe("Settings");
    }
  });
});

describe("a changed setting's Use the default (the owner, 2026-11-23, revamp question 16)", () => {
  /** The time zone, changed from its default. */
  const changedZone: Setting = { ...timeZone, value: "UTC", isModified: true };

  /** The row's Use the default, where it shows. */
  function shownDefaultButton(): HTMLElement | null {
    return document.querySelector<HTMLElement>(
      'main button[aria-label="Use the default for Time zone"]',
    );
  }

  /** The row's Use the default. */
  function defaultButton(): HTMLElement {
    const found = shownDefaultButton();
    if (found === null) {
      throw new Error("no Use the default");
    }
    return found;
  }

  /** The confirm open over the page. */
  function confirm(): HTMLDialogElement | null {
    return document.querySelector<HTMLDialogElement>("dialog[open]");
  }

  /** The confirm's button saying text. */
  function confirmButton(text: string): HTMLElement {
    const found = [...(confirm()?.querySelectorAll<HTMLElement>("button") ?? [])].find(
      (each) => each.textContent.trim() === text,
    );
    if (found === undefined) {
      throw new Error(`no ${text} in the confirm`);
    }
    return found;
  }

  /** The resets heard. */
  function resets(heard: readonly Heard[]): number {
    return heard.filter((each) => each.methodAndPath === "POST /config/timeZone/reset").length;
  }

  it.each(partWidths)(
    "shows beside the changed setting alone at %i px, asks by a click, a tap and the keyboard, focus on Cancel, and puts it back by a remote, focus kept in its row",
    async (width) => {
      await page.viewport(width, 800);
      const heard: Heard[] = [];
      mounted = await mountSettings(heard, [mostTitles, changedZone], {
        "POST /config/timeZone/reset": [200, timeZone],
      });
      await userEvent.click(link("General"));
      await expect.poll(shownDefaultButton).not.toBeNull();
      // Beside the changed setting alone, inside the page's width.
      expect(
        document.querySelectorAll('main button[aria-label^="Use the default for"]'),
      ).toHaveLength(1);
      const box = defaultButton().getBoundingClientRect();
      expect(box.right).toBeLessThanOrEqual(width);
      expect(box.height).toBeGreaterThanOrEqual(44);
      expect(defaultButton().closest("#setting-timeZone")).not.toBeNull();
      // A click asks, plain, focus on Cancel; Cancel sends nothing.
      await userEvent.click(defaultButton());
      await expect.poll(confirm).not.toBeNull();
      expect(document.activeElement).toBe(confirmButton("Cancel"));
      expect(confirmButton("Use the default").className).not.toContain("irreversible");
      expect(confirm()?.textContent).toContain("It goes from UTC to Europe/Berlin, its default.");
      await userEvent.click(confirmButton("Cancel"));
      await expect.poll(confirm).toBeNull();
      // A tap asks; Escape sends nothing.
      tapOn(defaultButton());
      await expect.poll(confirm).not.toBeNull();
      expect(document.activeElement).toBe(confirmButton("Cancel"));
      await userEvent.keyboard("{Escape}");
      await expect.poll(confirm).toBeNull();
      // The keyboard: Enter asks, Enter on Cancel sends nothing, focus back on the button.
      defaultButton().focus();
      await userEvent.keyboard("{Enter}");
      await expect.poll(confirm).not.toBeNull();
      expect(document.activeElement).toBe(confirmButton("Cancel"));
      await userEvent.keyboard("{Enter}");
      await expect.poll(confirm).toBeNull();
      await expect.poll(() => document.activeElement).toBe(shownDefaultButton());
      await settleAnswers();
      expect(resets(heard)).toBe(0);
      // A remote: Down from the trail into the time zone's row, Right along it to the button, OK
      // asks, Right to Use the default, OK puts it back.
      trailUp().focus();
      const isInRow = (): boolean =>
        (document.activeElement?.closest("#setting-timeZone") ?? null) !== null;
      expect(await arrowUntil("ArrowDown", isInRow)).toBe(true);
      const isOnButton = (): boolean => document.activeElement === shownDefaultButton();
      expect(await arrowUntil("ArrowRight", isOnButton, 4)).toBe(true);
      expect(isRingShown()).toBe(true);
      await userEvent.keyboard("{Enter}");
      await expect.poll(confirm).not.toBeNull();
      expect(document.activeElement).toBe(confirmButton("Cancel"));
      // Beside Cancel, or under or over it where a phone's width stacks the confirm's foot.
      const isOnUse = (): boolean => document.activeElement === confirmButton("Use the default");
      expect(
        (await arrowUntil("ArrowRight", isOnUse, 1)) ||
          (await arrowUntil("ArrowDown", isOnUse, 1)) ||
          (await arrowUntil("ArrowUp", isOnUse, 1)),
      ).toBe(true);
      await userEvent.keyboard("{Enter}");
      await expect.poll(() => resets(heard)).toBe(1);
      // At the default it has nothing to put back, and focus stays in the setting's row.
      await expect.poll(shownDefaultButton).toBeNull();
      await expect.poll(isInRow).toBe(true);
      expect(isFocusOnSomething()).toBe(true);
    },
  );
});

describe("a section's settings, saving as they change (D538 §9)", () => {
  /** Opens General from the index, its settings drawn. */
  async function openGeneral(heard: Heard[]): Promise<void> {
    mounted = await mountSettings(heard);
    await userEvent.click(link("General"));
    await expect.poll(() => document.querySelector("#setting-timeZone input")).not.toBeNull();
  }

  /** The text field of the time zone. */
  function timeZoneField(): HTMLInputElement {
    const found = document.querySelector<HTMLInputElement>("#setting-timeZone input");
    if (found === null) {
      throw new Error("no time zone field");
    }
    return found;
  }

  it("saves a Stepper at each step, by a click, a tap, the keyboard and a remote, and says Saved", async () => {
    const heard: Heard[] = [];
    await openGeneral(heard);
    const more = (): HTMLElement => button("Most titles in Continue watching: one more");
    await userEvent.click(more());
    await expect.poll(() => savedValues(heard)).toEqual([{ continueWatchingMaxWorks: 21 }]);
    await expect
      .poll(() => document.querySelector("#setting-continueWatchingMaxWorks")?.textContent)
      .toContain("Saved");
    tapOn(more());
    await expect.poll(() => savedValues(heard)).toHaveLength(2);
    // A remote: Down from the trail into the Stepper's row, along it to "one more", OK.
    trailUp().focus();
    const isInRow = (): boolean =>
      (document.activeElement?.closest("#setting-continueWatchingMaxWorks") ?? null) !== null;
    expect(await arrowUntil("ArrowDown", isInRow)).toBe(true);
    const isOnMore = (): boolean => document.activeElement === more();
    expect(
      (await arrowUntil("ArrowRight", isOnMore, 3)) || (await arrowUntil("ArrowLeft", isOnMore, 3)),
    ).toBe(true);
    expect(isRingShown()).toBe(true);
    await userEvent.keyboard("{Enter}");
    await expect.poll(() => savedValues(heard)).toHaveLength(3);
    // The keyboard's Space, focus and its ring staying.
    await userEvent.keyboard(" ");
    await expect.poll(() => savedValues(heard)).toHaveLength(4);
    expect(document.activeElement).toBe(more());
    expect(isRingShown()).toBe(true);
  });

  it("saves a text field on Enter and on leaving it, never as the page closes", async () => {
    const heard: Heard[] = [];
    await openGeneral(heard);
    await userEvent.click(timeZoneField());
    expect(isFocusOnSomething()).toBe(true);
    await userEvent.clear(timeZoneField());
    await userEvent.keyboard("UTC");
    await settleAnswers();
    expect(savedValues(heard)).toEqual([]);
    await userEvent.keyboard("{Enter}");
    await expect.poll(() => savedValues(heard)).toEqual([{ timeZone: "UTC" }]);
    // Leaving it, by Tab.
    await userEvent.keyboard("{Backspace}");
    await userEvent.keyboard("{Tab}");
    expect(isFocusOnSomething()).toBe(true);
    await expect.poll(() => savedValues(heard)).toHaveLength(2);
    expect(savedValues(heard)[1]).toEqual({ timeZone: "UT" });
    // Typed, then the page closes: nothing is sent.
    await userEvent.click(timeZoneField());
    await userEvent.keyboard("C+1");
    mounted?.unmount();
    mounted = undefined;
    await settleAnswers();
    expect(savedValues(heard)).toHaveLength(2);
  });
});
