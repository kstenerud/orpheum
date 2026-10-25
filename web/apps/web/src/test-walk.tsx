// SPDX-License-Identifier: MPL-2.0

/**
 * The whole signed-in app for a browser-mode walk with a remote's keys or a controller
 * (input-walk.browser.test.tsx): the client as client-main.tsx mounts it, its API answered by a
 * stand-in server that answers what the walk gives and refuses the rest as not found, so a page's
 * every background read is answered at once and none reaches the network (AGENTS.md rule 16).
 */

import { apiPrefix } from "@orpheum/sdk";
import { act } from "react";
import { createRoot } from "react-dom/client";

import { App } from "./app.tsx";
// The screens the client loads as they are first shown (later-screens.tsx), loaded with the walk as
// they were before they were split: a walk's time is its own, not the dev server's first build of a
// screen, which under load took the player's walk past the 15 s a test has (2026-10-25).
import "./cast-remote.tsx";
import "./lab.tsx";
import "./lab-extend-player.tsx";
import "./play.tsx";
import "./series.tsx";
import "./settings.tsx";
import "./title.tsx";
import "./together-viewer.tsx";
import "./ui-gallery.tsx";
import "./watch-together.tsx";
import { ServicesContext, type Services } from "./services.tsx";
import { testServices } from "./test-services.tsx";

/** What the stand-in server answers, by "METHOD /path", the path as the API names it. */
export type WalkAnswers = Readonly<Record<string, unknown>>;

/** A person signed in with no admin's rights, who may host Watch Together. */
export const viewer = {
  id: 1,
  name: "Karl",
  role: "viewer",
  isAdmin: false,
  canHostWatchTogether: true,
  canManageAccount: true,
  hasNoLibrary: false,
};

/** Films for a list, Film 1 to Film count, ids from 101. */
export function films(count: number): Record<string, unknown>[] {
  return Array.from({ length: count }, (_, index) => ({
    id: 101 + index,
    kind: "movie",
    name: `Film ${String(index + 1)}`,
    year: 2000 + index,
  }));
}

/** A film's page as the server sends it, with one file to play and one extra. */
export function filmDetail(id: number, name: string): Record<string, unknown> {
  return {
    id,
    kind: "movie",
    name,
    year: 2001,
    identificationState: "identified",
    libraryId: 1,
    art: {},
    versions: [{ id: 1, files: [{ id: id * 10, path: `/m/${name}.mkv`, streams: [] }] }],
    extras: [{ id: id + 1000, name: `${name} trailer`, extraType: "trailer" }],
  };
}

/**
 * Services whose server answers answers by path, and every other request as not found; each request
 * is added to heard as "METHOD /path".
 */
export function walkServices(
  answers: WalkAnswers,
  path = "/films",
  heard: string[] = [],
): Services {
  return testServices("token", path, async (request) => {
    await Promise.resolve();
    const url = new URL(request.url);
    const methodAndPath = `${request.method} ${url.pathname.slice(apiPrefix.length)}`;
    heard.push(methodAndPath);
    if (!(methodAndPath in answers)) {
      return Response.json(
        { error: { code: "not_found", message: "the walk serves no such thing" } },
        { status: 404 },
      );
    }
    const body = answers[methodAndPath];
    return body === null
      ? new Response(null, { status: 204 })
      : Response.json(body, { status: 200 });
  });
}

/** The app mounted, and taking it down. */
export interface Walking {
  readonly services: Services;
  readonly unmount: () => void;
}

/** Mounts the whole client over services, as client-main.tsx does. */
export async function mountApp(services: Services): Promise<Walking> {
  Reflect.set(globalThis, "IS_REACT_ACT_ENVIRONMENT", false);
  const container = document.createElement("div");
  document.body.append(container);
  const root = createRoot(container);
  await act(async () => {
    root.render(
      <ServicesContext value={services}>
        <App />
      </ServicesContext>,
    );
    await Promise.resolve();
  });
  return {
    services,
    unmount: () => {
      root.unmount();
      container.remove();
    },
  };
}

/**
 * The deadline of a walk's expect.poll, for what a screen draws once its answers come, or where a
 * press leaves focus: expect.poll's own second passed under the gate's load, with nothing wrong
 * but a slow machine (four browser-mode runs at once, 2026-10-25; test-weight.md). Inside the test's
 * own 15 s, so a walk that never gets there still fails as itself.
 */
export const walkPoll = { timeout: 10_000 } as const;

/** What has focus, by its text or name, for a walk's checks. */
export function focusedName(): string {
  const focused = document.activeElement;
  if (focused === null || focused === document.body) {
    return "";
  }
  return (focused.getAttribute("aria-label") ?? focused.textContent).trim();
}

/** Whether what has focus is inside the element selector names. */
export function isFocusIn(selector: string): boolean {
  return document.querySelector(selector)?.contains(document.activeElement) === true;
}

/** How far a box may pass the screen's edge and still count as on it: layout's rounding. */
const roundingPx = 1;

/** Whether what has focus is wholly on the screen, as a person must see it to know where they are. */
export function isFocusInView(): boolean {
  const focused = document.activeElement;
  if (focused === null || focused === document.body) {
    return false;
  }
  const box = focused.getBoundingClientRect();
  return (
    box.top >= -roundingPx &&
    box.left >= -roundingPx &&
    box.bottom <= window.innerHeight + roundingPx &&
    box.right <= window.innerWidth + roundingPx
  );
}

/** The name of the film or series whose tile in the grid has focus, or "" where none has it. */
export function focusedTile(): string {
  const focused = document.activeElement;
  return focused?.closest(".poster-grid") === null
    ? ""
    : (focused?.querySelector(".poster-card__name")?.textContent ?? "");
}

/** Whether the page draws an element by selector, for a walk to wait on. */
export function isDrawn(selector: string): boolean {
  return document.querySelector(selector) !== null;
}

/** The arrows a walk tries from each control it reaches. */
const arrows = ["{ArrowUp}", "{ArrowDown}", "{ArrowLeft}", "{ArrowRight}"] as const;

/**
 * How many presses of one arrow a walk makes while each only scrolls the page through text, before
 * it takes the arrow to lead nowhere more: three screens' reading.
 */
const readingPresses = 4;

/** Presses arrow until focus moves or a press scrolls nothing, at most readingPresses times. */
async function pressThroughText(
  arrow: string,
  press: (keys: string) => Promise<void>,
): Promise<void> {
  const before = document.activeElement;
  for (let pressed = 0; pressed < readingPresses; pressed += 1) {
    const scrolled = window.scrollY;
    await press(arrow);
    if (document.activeElement !== before || window.scrollY === scrolled) {
      return;
    }
  }
}

/**
 * Every element an arrow reaches from start, following each arrow from each element reached in turn,
 * pressed as a remote's keys are by press, again while a press scrolls through text: what a person
 * can get to on the page.
 */
export async function reachedFrom(
  start: HTMLElement,
  press: (keys: string) => Promise<void>,
): Promise<Set<Element>> {
  const reached = new Set<Element>([start]);
  const waiting: HTMLElement[] = [start];
  // A walk whose page is gone, its test over (timed out), presses nothing more: walking on, it
  // pressed keys into the next test's page, which then saw seeks it never made (2026-10-25).
  const pressOnItsPage = async (keys: string): Promise<void> => {
    if (start.isConnected) {
      await press(keys);
    }
  };
  for (
    let next = waiting.shift();
    next !== undefined && start.isConnected;
    next = waiting.shift()
  ) {
    for (const arrow of arrows) {
      next.focus();
      await pressThroughText(arrow, pressOnItsPage);
      const now = document.activeElement;
      if (now instanceof HTMLElement && now !== document.body && !reached.has(now)) {
        reached.add(now);
        waiting.push(now);
      }
    }
  }
  return reached;
}
