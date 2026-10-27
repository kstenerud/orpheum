// SPDX-License-Identifier: MPL-2.0

import type { JSX } from "react";

import { BackButton } from "./back-button.tsx";
import { useBackTarget } from "./input-layer.tsx";
import { itemRoute, type Route } from "./router.ts";
import { episodeUp } from "./title.tsx";
import { isTelevision } from "./tv-device.ts";
import type { PlaybackDecision } from "./watching.ts";

/**
 * A way back to the list the film was chosen from, up to what it plays from when the page was
 * opened from outside (the owner, 2026-10-27: "when I play a video, there's no way to return to the
 * list of videos"). A television's player has the remote's Back instead, which goes up there too,
 * so it shows no button.
 */
export function BackToTheList(props: { readonly plays: PlaybackDecision["plays"] }): JSX.Element {
  const up = playUp(props.plays);
  return isTelevision(navigator.userAgent) ? <UpByTheRemote up={up} /> : <BackButton up={up} />;
}

/** Tells a remote's Back where up is, with no button to show: a television's player. */
function UpByTheRemote(props: { readonly up: Route }): null {
  useBackTarget(props.up);
  return null;
}

/**
 * The page above a Play page, the one it plays from (the owner, 2026-10-27: "Would it be better to
 * go back to the non-fullscreen page?"): a film's or an extra's own page, an episode's series where
 * it is in the person's ordering, an other video's library's list; home where the server does not
 * say what it plays, or names no library of the person's over an other video's file (D363 §6).
 */
export function playUp(plays: PlaybackDecision["plays"]): Route {
  if (plays === undefined) {
    return { screen: "home" };
  }
  if (plays.episode !== undefined) {
    return episodeUp(plays.episode);
  }
  if (plays.kind !== "other") {
    return itemRoute(plays.kind, plays.titleId);
  }
  return plays.libraryId === undefined
    ? { screen: "home" }
    : { screen: "browse", library: plays.libraryId };
}
