// SPDX-License-Identifier: MPL-2.0

import { createContext, use, useEffect, useMemo, type JSX, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { BackButton } from "./back-button.tsx";
import { noteEpisodeReached } from "./episode-reached.ts";
import { i18n } from "./i18n.ts";
import { useBackTarget } from "./input-layer.tsx";
import { routeHref, routeOfHref, type Route } from "./router.ts";
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
 * Where a Play page sits (D367 §2): an episode's playing under its series, at its season, under the
 * library; a film's under the film's page, and an extra's under its film's (the owner, 2026-10-27:
 * "Would it be better to go back to the non-fullscreen page?"); an other video's under its library's
 * list; home where the server does not say what it plays, or names no library of the person's over an
 * other video's file (D363 §6). What plays names its own page, an episode's, an extra's or a video's,
 * where the trail's last part links to (the owner, 2026-10-28: "The last part of the trail should
 * also be selectable to get back to the info page"); a film's page is already its parent.
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
  // What plays, its info page, which the trail's last part links to (the owner, 2026-10-28).
  const herePage = titleCrumb(plays.titleId, name);
  if (plays.episode !== undefined) {
    const here = episodeHere(context.t, plays.episode, name);
    return { crumbs: episodeCrumbs(plays.episode, libraries, context), here, herePage };
  }
  if (plays.kind === "other") {
    return {
      crumbs: [
        libraries === undefined ? homeCrumb(context) : libraryCrumb("other", libraries, context),
      ],
      here: name,
      herePage,
    };
  }
  const library = libraryCrumb("movie", libraries, context);
  const film = plays.filmOfExtra;
  if (film !== undefined) {
    return { crumbs: [library, titleCrumb(film.id, film.name)], here: name, herePage };
  }
  // The film's own page is its parent, under its own name; playing it is the step below.
  return { crumbs: [library, titleCrumb(plays.titleId, name)], here: context.t("trail.playing") };
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
 * A Play page's upward, for the television's player overlay, which shows its Up and its Back
 * (tv-controls.tsx); undefined where the page plays no film of its own, a cast's receiver.
 */
export const PlayUpwardContext = createContext<Upward | undefined>(undefined);

/**
 * The Play page's way up, at its top (D367 §8): on a computer or a phone its trail; on a television
 * none here, its player's overlay holding its Up and Back (OverlayUpAndBack), and a remote's Back
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
 * The Play page's top, its trail and the film's name, over children, the player, whose television
 * overlay finds its Up and Back by PlayUpwardContext; a cast's receiver shows neither (D337 §3).
 */
export function PlayTop(props: {
  readonly plays: Plays;
  readonly titleName: string | undefined;
  readonly isOwn: boolean;
  readonly children: ReactNode;
}): JSX.Element {
  const { plays, titleName, isOwn } = props;
  const upward = usePlayUpward(plays, titleName);
  return (
    <>
      {isOwn ? <PlayTrail upward={upward} plays={plays} /> : null}
      {titleName === undefined || !isOwn ? null : <h1 className="play__title">{titleName}</h1>}
      <PlayUpwardContext value={isOwn ? upward : undefined}>{props.children}</PlayUpwardContext>
    </>
  );
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
 * A television's player overlay's way out (D367 §6, §8): Up, "‹ Copper Kettle", and an on-screen Back,
 * since TV Bro's remote Back never reaches the page and the app cannot tell it from Silk; nothing on
 * a cast's receiver.
 */
export function OverlayUpAndBack(): JSX.Element | null {
  const { t } = useTranslation();
  const upward = use(PlayUpwardContext);
  const parent = upward === undefined ? undefined : parentOf(upward);
  if (parent === undefined) {
    return null;
  }
  return (
    <div className="tv-controls__leave">
      <UpLink crumb={parent} className="tv-controls__up">
        {t("trail.up", { name: parent.label })}
      </UpLink>
      <BackButton up={parent.to} />
    </div>
  );
}
