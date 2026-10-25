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

/** Services whose server answers answers by path, and every other request as not found. */
export function walkServices(answers: WalkAnswers, path = "/films"): Services {
  return testServices("token", path, async (request) => {
    await Promise.resolve();
    const url = new URL(request.url);
    const methodAndPath = `${request.method} ${url.pathname.slice(apiPrefix.length)}`;
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

/** The name of the film or series whose tile has focus, or "" where no tile has it. */
export function focusedTile(): string {
  return document.activeElement?.querySelector(".poster-grid__name")?.textContent ?? "";
}

/** Whether the page draws an element by selector, for a walk to wait on. */
export function isDrawn(selector: string): boolean {
  return document.querySelector(selector) !== null;
}
