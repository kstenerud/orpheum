// SPDX-License-Identifier: MPL-2.0

/**
 * A film warmed up as the person presses what plays it, the way a title's page warms its own as it
 * shows (title.tsx's useWarmsUp, Watching.warmUp): Continue Watching's tiles and a series' next
 * episode sent POST /playback only once the route had changed, the Play page's code had loaded and
 * it had drawn (the web client's architecture audit of 2026-11-10, Playback 1). On the press, never
 * on focus (D258 §2): a remote's walk across a row would otherwise decide every film it passes.
 */

import type { MouseEvent } from "react";

import { routeOfHref } from "./router.ts";
import { useServices } from "./services.tsx";

/**
 * A click handler for an element's capture phase: a plain press (one a link follows in this tab,
 * as Link does) of a link under it to a film's Play page, from where it resumes, warms that film up
 * before the link follows, where isWarmed says so of its file. The film's code is imported as it is
 * pressed, so a page that holds such links loads no player before it draws.
 */
export function useWarmsUpOnPress(
  isWarmed: (fileId: number) => boolean,
): (event: MouseEvent<HTMLElement>) => void {
  const { api, playableFormats, pathMoves } = useServices();
  return (event) => {
    const fileId = playedOnPress(event);
    if (fileId === undefined || !isWarmed(fileId)) {
      return;
    }
    void import("./watching.ts").then(({ watchingOf }) => {
      watchingOf(api).warmUp(fileId, playableFormats(), () => pathMoves?.isOverWebrtc() === true);
    });
  };
}

/** The file a press plays from where it resumes, when it is a plain press of a link to Play. */
function playedOnPress(event: MouseEvent<HTMLElement>): number | undefined {
  const link = event.target instanceof Element ? event.target.closest("a") : null;
  const href = link?.getAttribute("href") ?? null;
  if (!isPlainPress(event) || href === null) {
    return undefined;
  }
  const route = routeOfHref(href);
  return route.screen === "play" && route.atMs === undefined ? route.fileId : undefined;
}

/** Whether a press is one a link follows in this tab, as Link does: the main button, no key held. */
function isPlainPress(event: MouseEvent<HTMLElement>): boolean {
  return event.button === 0 && !event.metaKey && !event.ctrlKey && !event.shiftKey && !event.altKey;
}
