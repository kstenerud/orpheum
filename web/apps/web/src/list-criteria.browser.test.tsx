// SPDX-License-Identifier: MPL-2.0

/**
 * A list's Sort and filter (listing-tools.tsx, D417) in a real browser, by a keyboard's keys, on a
 * phone's narrow screen: its pop-up within the screen, its field's caret, and Back from its field.
 * The walks from a remote on a TV's and a desktop's screen are input-pages.browser.test.tsx's.
 */

// Every page's ground, as the client's entry lays it (client-main.tsx): its tokens and base rules.
import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";
import { page, userEvent } from "vitest/browser";

import { sendFailuresTo } from "@orpheum/sdk";

import { focusTargetsIn } from "./focus-engine.ts";
import { createNavigation } from "./navigation.ts";
import { fakePlace } from "./test-services.tsx";
import {
  films,
  isDrawn,
  isFocusIn,
  mountApp,
  viewer,
  walkServices,
  type Walking,
  type WalkAnswers,
} from "./test-walk.tsx";

/** Films, a list with a Sort and filter, and the genres its chips offer. */
const filmsList: WalkAnswers = {
  "GET /me": viewer,
  "GET /client-settings": {},
  "GET /home": { continueWatching: [] },
  "GET /titles": { titles: films(3), nextCursor: "" },
  "GET /titles/letters": { letters: [] },
  "GET /genres": { genres: [{ id: 2, name: "Drama" }] },
};

/** The list's Sort and filter button, on its bar. */
function criteriaButton(): HTMLElement | undefined {
  return document.querySelector<HTMLElement>("main .list-criteria__button") ?? undefined;
}

/** Opens the Sort and filter from its button by Enter, and waits for its field; the button. */
async function openedByEnter(): Promise<HTMLElement | undefined> {
  await expect.poll(() => isDrawn("main .list-criteria__button")).toBe(true);
  const button = criteriaButton();
  button?.focus();
  await userEvent.keyboard("{Enter}");
  await expect.poll(() => isDrawn("dialog input.list-criteria__name")).toBe(true);
  return button;
}

// D417: a list's Sort and filter on a phone's narrow screen, by a keyboard's keys. Its field keeps
// Left and Right for its caret, as a dialog's field does, and does not close the pop-up at its
// value's edges; every other control in it keeps the owner's rule that Left and Right close it
// (2026-10-28).
describe("a list's Sort and filter, on a phone's 360 px screen", () => {
  let walking: Walking | undefined;
  afterEach(() => {
    walking?.unmount();
    walking = undefined;
    sendFailuresTo(() => undefined);
  });

  it("opens within the screen's width, focus on its field, every control inside it", async () => {
    await page.viewport(360, 640);
    walking = await mountApp(walkServices(filmsList, "/films"));
    await openedByEnter();
    expect(document.activeElement?.classList.contains("list-criteria__name")).toBe(true);
    const box = document.querySelector("dialog")?.getBoundingClientRect();
    expect(box?.left).toBeGreaterThanOrEqual(0);
    expect(box?.right).toBeLessThanOrEqual(window.innerWidth);
    const outside = focusTargetsIn(document.querySelector(".list-criteria") ?? document).filter(
      (control) => {
        const each = control.getBoundingClientRect();
        return each.left < 0 || each.right > window.innerWidth;
      },
    );
    expect(outside.map((each) => each.outerHTML)).toEqual([]);
  });

  // browse.css: Unwatched is a toggle drawn as the choosers beside it are, strong while on.
  it("draws its Unwatched toggle as a chooser, its edge and words strong while on", async () => {
    await page.viewport(360, 640);
    walking = await mountApp(walkServices(filmsList, "/films"));
    await openedByEnter();
    const toggle = document.querySelector<HTMLElement>("dialog .list-chips__toggle");
    const looks = (): { weight: number; edge: string; radius: string } => {
      const style = getComputedStyle(toggle ?? document.body);
      return {
        weight: Number(style.fontWeight),
        edge: style.borderTopColor,
        radius: style.borderTopLeftRadius,
      };
    };
    const off = looks();
    expect(off.radius).not.toBe("0px");
    toggle?.click();
    await expect.poll(() => toggle?.getAttribute("aria-pressed")).toBe("true");
    const on = looks();
    expect(on.weight).toBeGreaterThan(off.weight);
    expect(on.edge).not.toBe(off.edge);
  });

  it("moves its field's caret by Left and Right, open at the value's edges, and closes at Left from another control", async () => {
    await page.viewport(360, 640);
    const sent: string[] = [];
    sendFailuresTo((text) => sent.push(text));
    walking = await mountApp(walkServices(filmsList, "/films"));
    const button = await openedByEnter();
    const field = document.querySelector<HTMLInputElement>("input.list-criteria__name");
    await userEvent.keyboard("heat");
    expect(field?.selectionStart).toBe(4);
    await userEvent.keyboard("{ArrowLeft}");
    expect(field?.selectionStart).toBe(3);
    // Past its start: still in the field, the pop-up open.
    await userEvent.keyboard("{ArrowLeft}{ArrowLeft}{ArrowLeft}{ArrowLeft}{ArrowLeft}");
    expect(field?.selectionStart).toBe(0);
    expect(document.activeElement).toBe(field);
    expect(document.querySelector("dialog")).not.toBeNull();
    // Past its end: the same.
    await userEvent.keyboard(
      "{ArrowRight}{ArrowRight}{ArrowRight}{ArrowRight}{ArrowRight}{ArrowRight}",
    );
    expect(field?.selectionStart).toBe(4);
    expect(document.activeElement).toBe(field);
    expect(document.querySelector("dialog")).not.toBeNull();
    // A Left kept by the field is no drop-down stuck open, and is not told as one.
    expect(sent.filter((each) => each.includes("stayed open"))).toEqual([]);
    // Down leaves the field; Left from the control it reaches closes the pop-up, focus on its button.
    await userEvent.keyboard("{ArrowDown}");
    expect(document.activeElement).not.toBe(field);
    expect(isFocusIn(".list-criteria")).toBe(true);
    await userEvent.keyboard("{ArrowLeft}");
    await expect.poll(() => document.querySelector("dialog")).toBeNull();
    await expect.poll(() => document.activeElement).toBe(button);
  });

  it("closes by the browser's Back from its field, focus on its button, what was typed kept", async () => {
    await page.viewport(360, 640);
    const place = fakePlace("/");
    const navigation = createNavigation(place, false);
    navigation.go({ screen: "films" });
    walking = await mountApp({ ...walkServices(filmsList), navigation });
    const button = await openedByEnter();
    await userEvent.keyboard("heat");
    place.history.back();
    await expect.poll(() => document.querySelector("dialog")).toBeNull();
    await expect.poll(() => document.activeElement).toBe(button);
    await expect.poll(() => navigation.route()).toEqual({ screen: "films", query: "heat" });
  });
});
