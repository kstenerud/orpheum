// SPDX-License-Identifier: MPL-2.0

import { createContext, use, useEffect, useMemo, type JSX, type ReactNode } from "react";

import { noteEpisodeReached } from "./episode-reached.ts";
import { i18n } from "./i18n.ts";
import { useBackTarget } from "./input-layer.tsx";
import { BackIcon } from "./player-icons.tsx";
import { routeHref, routeOfHref, type Route } from "./router.ts";
import { useServices } from "./services.tsx";
import { isTelevision } from "./tv-device.ts";
import { UpLink, UpTrail, useLibraryContext } from "./up-trail.tsx";
import {
  episodeCrumbs,
  episodeHere,
  libraryCrumb,
  parentOf,
  seasonOfEpisode,
  titleCrumb,
  type Crumb,
  type LibraryContext,
  type Upward,
} from "./upward.ts";
import type { PlaybackDecision } from "./watching.ts";

/** What a playback plays, as its decision says it. */
type Plays = PlaybackDecision["plays"];

/** Home, the page above a Play page whose playback says nothing of what it plays. */
function homeCrumb(context: LibraryContext): Crumb {
  return {
    label: context.t("trail.home"),
    to: { screen: "home" },
    isIt: (route) => route.screen === "home",
  };
}

/**
 * Where a Play page sits (D367 §2): under the info page of what plays, as one goes through it to play
 * (the owner, 2026-10-28: "it always leaves a trail that goes back to the info page before going any
 * further back"): an episode's under its own page, at its season of its series, under the library; a
 * film's under the film's page, and an extra's under its own page, under its film's; an other
 * video's under its own page, under its library's list; home where the server does not say what it
 * plays, or names no library of the person's over an other video's file (D363 §6).
 */
export function playUpward(
  plays: Plays,
  titleName: string | undefined,
  context: LibraryContext,
): Upward {
  const name = titleName ?? "";
  if (plays === undefined) {
    return { crumbs: [homeCrumb(context)], here: name };
  }
  const libraries =
    plays.libraryId === undefined
      ? undefined
      : [{ id: plays.libraryId, name: plays.libraryName ?? context.t("browse.library") }];
  const here = context.t("trail.playing");
  if (plays.episode !== undefined) {
    const episode = titleCrumb(plays.titleId, episodeHere(context.t, plays.episode, name));
    return { crumbs: [...episodeCrumbs(plays.episode, libraries, context), episode], here };
  }
  const page = titleCrumb(plays.titleId, name);
  if (plays.kind === "other") {
    const list =
      libraries === undefined ? homeCrumb(context) : libraryCrumb("other", libraries, context);
    return { crumbs: [list, page], here };
  }
  const library = libraryCrumb("movie", libraries, context);
  const film = plays.filmOfExtra;
  if (film !== undefined) {
    return { crumbs: [library, titleCrumb(film.id, film.name), page], here };
  }
  return { crumbs: [library, page], here };
}

/** The Play page's place, the same object while what it plays and the history behind it are. */
export function usePlayUpward(plays: Plays, titleName: string | undefined): Upward {
  const { t, behind } = useLibraryContext();
  // The history's routes are read anew at each render: their addresses say when they changed.
  const behindHrefs = behind.map(routeHref).join(" ");
  return useMemo(
    () => playUpward(plays, titleName, { t, behind: routesOf(behindHrefs) }),
    [plays, titleName, t, behindHrefs],
  );
}

/** The routes of addresses of this app's, each a whole address, apart by spaces. */
function routesOf(hrefs: string): Route[] {
  return hrefs === "" ? [] : hrefs.split(" ").map(routeOfHref);
}

/** The page above a Play page, where nothing of the app is behind it. */
export function playUp(plays: Plays, context: LibraryContext = { t: i18n.t, behind: [] }): Route {
  return parentOf(playUpward(plays, undefined, context))?.to ?? { screen: "home" };
}

/**
 * A Play page's upward, for the player's full-screen overlay, which shows its Up and what plays
 * (player-overlay.tsx); undefined where the page plays no film of its own, a cast's receiver.
 */
export const PlayUpwardContext = createContext<Upward | undefined>(undefined);

/**
 * The Play page's way up, at its top (D367 §8): on a computer or a phone its trail; on a television
 * none here, its player's overlay holding its Up (OverlayUp), and a remote's Back
 * with nothing of the app behind going up as the Up does. Notes an episode played, whose series'
 * page shows its season when returned to (§9).
 */
export function PlayTrail(props: { readonly upward: Upward; readonly plays: Plays }): JSX.Element {
  useEpisodePlayed(props.plays);
  return isTelevision(navigator.userAgent) ? (
    <UpByTheRemote up={parentOf(props.upward)?.to ?? { screen: "home" }} />
  ) : (
    <UpTrail upward={props.upward} />
  );
}

/**
 * The Play page's top, its trail and the film's name, over children, the player, whose full-screen
 * overlay finds its Up by PlayUpwardContext; a cast's receiver shows neither (D337 §3).
 * Notes on the page's history entry its title's page, which Back or Forward shows in its place.
 */
export function PlayTop(props: {
  readonly plays: Plays;
  readonly titleName: string | undefined;
  readonly isOwn: boolean;
  readonly children: ReactNode;
}): JSX.Element {
  const { plays, titleName, isOwn } = props;
  const upward = usePlayUpward(plays, titleName);
  useTitleNotedInPlace(plays?.titleId);
  return (
    <>
      {isOwn ? <PlayTrail upward={upward} plays={plays} /> : null}
      {titleName === undefined || !isOwn ? null : <h1 className="play__title">{titleName}</h1>}
      <PlayUpwardContext value={isOwn ? upward : undefined}>{props.children}</PlayUpwardContext>
    </>
  );
}

/**
 * Notes on the Play page's history entry its title's page, titleId's, as the page to show in its
 * place when the browser's Back or Forward returns to it (D383 §1), and, after a reload, the page a
 * cast's receiver with no controller goes to (§2); nothing where the playback names no title.
 */
function useTitleNotedInPlace(titleId: number | undefined): void {
  const { navigation } = useServices();
  useEffect(() => {
    // Only on the Play page's own entry: never on a page the person has gone on to.
    if (titleId !== undefined && navigation.route().screen === "play") {
      navigation.noteInPlace({ screen: "title", itemId: titleId });
    }
  }, [navigation, titleId]);
}

/** Notes the episode a playback plays as reached, for its series' page (D367 §9). */
function useEpisodePlayed(plays: Plays): void {
  const place = plays?.episode;
  const episodeId = plays?.titleId;
  const seriesId = place?.seriesId;
  const season = place === undefined ? undefined : seasonOfEpisode(place);
  useEffect(() => {
    if (seriesId !== undefined && episodeId !== undefined) {
      noteEpisodeReached(seriesId, { season, episodeId });
    }
  }, [seriesId, season, episodeId]);
}

/** Tells a remote's Back where up is, with no control to show: a television's player. */
function UpByTheRemote(props: { readonly up: Route }): null {
  useBackTarget(props.up);
  return null;
}

/**
 * The full-screen overlay's way out and what plays (D367 §6, §8; D407 §1): Up, "‹ S1E1 Pilot", the
 * page above a Play page being what plays; the on-screen Back goes, a remote's Back hiding the
 * overlay first (D407 §5, Q5). Nothing on a cast's receiver.
 */
export function OverlayUp(): JSX.Element | null {
  const upward = use(PlayUpwardContext);
  const parent = upward === undefined ? undefined : parentOf(upward);
  if (parent === undefined) {
    return null;
  }
  return (
    <UpLink crumb={parent} className="player-overlay__up">
      <BackIcon />
      <span className="player-overlay__title">{parent.label}</span>
    </UpLink>
  );
}
