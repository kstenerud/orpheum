// SPDX-License-Identifier: MPL-2.0

/**
 * The navigation model over the history (D367 §7, §9): the scenarios of
 * navigation-back-up-2026-10-27.md §3, each a walk of go, replace, Up and Back over a stand-in
 * history, Up's parent found as each page finds it (upward.ts, play-up.tsx, title.tsx).
 */

import { describe, expect, it } from "vitest";

import { i18n } from "./i18n.ts";
import { folderUpward } from "./library-browse.tsx";
import { createNavigation, type Navigation } from "./navigation.ts";
import { playUpward } from "./play-up.tsx";
import { routeHref, type Route } from "./router.ts";
import { fakePlace, type FakePlace } from "./test-services.tsx";
import { titleUpward, type TitleDetail } from "./title.tsx";
import { libraryCrumb, parentOf, type LibraryName, type Upward } from "./upward.ts";
import type { PlaybackDecision } from "./watching.ts";

const shows: LibraryName = { id: 7, name: "Shows" };
const films: LibraryName = { id: 3, name: "Films" };
const kids: LibraryName = { id: 4, name: "Kids" };

/** Copper Kettle's S2E3, played: its series is 5, its libraries Shows. */
const playsEpisode: PlaybackDecision["plays"] = {
  titleId: 23,
  kind: "episode",
  libraryId: 7,
  libraryName: "Shows",
  episode: { seriesId: 5, seriesName: "Copper Kettle", seasonNumber: 2, episodeNumber: 3 },
};

/** Metropolis, played: the film 10, in Films and Kids. */
const playsFilm: PlaybackDecision["plays"] = {
  titleId: 10,
  kind: "movie",
  libraryId: 3,
  libraryName: "Films",
};

/** Each page's route in the scenarios. */
const home: Route = { screen: "home" };
const showsList: Route = { screen: "browse", library: 7 };
const series: Route = { screen: "series-page", collectionId: 5 };
const seasonTwo: Route = { screen: "series-page", collectionId: 5, season: 2 };
const episodePage: Route = { screen: "title", itemId: 23 };
const playing: Route = { screen: "play", fileId: 30 };
const filmPage: Route = { screen: "title", itemId: 10 };

/** The series' own page's place: under the library it is listed in. */
function seriesUpward(navigation: Navigation): Upward {
  return {
    crumbs: [libraryCrumb("series", [shows], { t: i18n.t, behind: navigation.behind() })],
    here: "Copper Kettle",
  };
}

/** S2E3's own page's place: under its series at its season. */
function episodePageUpward(navigation: Navigation): Upward {
  const episode: TitleDetail = {
    id: 23,
    kind: "episode",
    name: "Doing the Right-Back Thing",
    identificationState: "identified",
    art: {},
    versions: [],
    extras: [],
    libraries: [shows],
    episode: { seriesId: 5, seriesName: "Copper Kettle", seasonNumber: 2, episodeNumber: 3 },
  };
  return titleUpward(episode, { t: i18n.t, behind: navigation.behind() });
}

/** Presses Up on a page whose place is upward, as its trail's parent does. */
function up(navigation: Navigation, upward: (navigation: Navigation) => Upward): void {
  const parent = parentOf(upward(navigation));
  if (parent === undefined) {
    throw new Error("the page has no Up");
  }
  navigation.up(parent.to, parent.isIt);
}

/** The Play page's place for plays. */
function playingUpward(plays: PlaybackDecision["plays"]) {
  return (navigation: Navigation): Upward =>
    playUpward(plays, "Doing the Right-Back Thing", { t: i18n.t, behind: navigation.behind() });
}

/** The history's addresses and where it stands, "[current]" bracketed, as the research writes it. */
function historyOf(place: FakePlace, navigation: Navigation): string {
  const behind = navigation.behind().map(routeHref);
  return [...behind, `[${place.location.pathname}${place.location.search}]`].join(" ");
}

/** A navigation at home that has gone to each route in turn. */
function walked(...routes: Route[]): { place: FakePlace; navigation: Navigation } {
  const place = fakePlace("/");
  const navigation = createNavigation(place, false);
  for (const route of routes) {
    navigation.go(route);
  }
  return { place, navigation };
}

describe("Up over the history (D367 §7; navigation-back-up-2026-10-27.md §3)", () => {
  it("§3.3: Up from the player climbs back to each parent behind, its info page first, and Back then leaves, never down", () => {
    const { place, navigation } = walked(showsList, series);
    navigation.replace(seasonTwo); // a season is a view of the series (§9)
    navigation.go(episodePage);
    navigation.go(playing);
    up(navigation, playingUpward(playsEpisode));
    expect(historyOf(place, navigation)).toBe(
      "/ /browse?library=7 /series/5?season=2 [/titles/23]",
    );
    up(navigation, episodePageUpward);
    expect(historyOf(place, navigation)).toBe("/ /browse?library=7 [/series/5?season=2]");
    up(navigation, seriesUpward);
    expect(historyOf(place, navigation)).toBe("/ [/browse?library=7]");
    navigation.back();
    expect(routeHref(navigation.route())).toBe("/");
  });

  it("§3.4: Continue Watching → play: Back is Home; Up is the episode's page in its place, and Back then Home", () => {
    const { place, navigation } = walked(playing);
    up(navigation, playingUpward(playsEpisode));
    expect(historyOf(place, navigation)).toBe("/ [/titles/23]");
    expect(place.addresses).toHaveLength(2);
    navigation.back();
    expect(navigation.route()).toEqual(home);
  });

  it("§3.5: a search's result played: Up goes to its page, then back to the series behind, Back then to the results", () => {
    const results: Route = { screen: "browse", library: 7, query: "lasso" };
    const { place, navigation } = walked(results, series, playing);
    up(navigation, playingUpward(playsEpisode));
    expect(historyOf(place, navigation)).toBe(
      "/ /browse?library=7&query=lasso /series/5 [/titles/23]",
    );
    up(navigation, episodePageUpward);
    expect(historyOf(place, navigation)).toBe("/ /browse?library=7&query=lasso [/series/5]");
    navigation.back();
    expect(navigation.route()).toEqual(results);
  });

  it("§3.7: a binge into season 2, Up next replacing the player: Up is its page, then the series behind, Back Home", () => {
    const { place, navigation } = walked({ ...series, season: 1 }, { screen: "play", fileId: 29 });
    navigation.replace(playing); // Up next plays S2E1 in the player's place
    up(navigation, playingUpward(playsEpisode));
    expect(historyOf(place, navigation)).toBe("/ /series/5?season=1 [/titles/23]");
    up(navigation, episodePageUpward);
    expect(historyOf(place, navigation)).toBe("/ [/series/5?season=1]");
    navigation.back();
    expect(navigation.route()).toEqual(home);
  });

  it("§3.9: opened from outside, Up puts the parent in place, and again, never adding an entry", () => {
    const place = fakePlace("/play/30");
    const navigation = createNavigation(place, false);
    up(navigation, playingUpward(playsEpisode));
    up(navigation, episodePageUpward);
    up(navigation, seriesUpward);
    expect(place.addresses).toEqual(["/browse?library=7"]);
    expect(navigation.back()).toBe(false);
  });

  it("§3.10: a television reloaded on the player keeps its base, whose Back goes to the parent of the page shown", () => {
    const place = fakePlace("/play/30");
    const navigation = createNavigation(place, true);
    navigation.onBase(() => {
      // As the input layer does: the page shown's parent (input-layer.tsx's backUpRef).
      const parent = parentOf(episodePageUpward(navigation));
      navigation.replace(parent?.to ?? home);
    });
    up(navigation, playingUpward(playsEpisode));
    expect(routeHref(navigation.route())).toBe("/titles/23");
    navigation.back();
    expect(routeHref(navigation.route())).toBe("/series/5?season=2");
  });

  it("§3.15: an other library's folders: Up climbs them as Back does, each folder behind", () => {
    const folders = (folder?: string): Route => ({
      screen: "browse",
      library: 7,
      view: "folders",
      ...(folder === undefined ? {} : { folder }),
    });
    const { place, navigation } = walked(folders(), folders("A"), folders("A/B"), playing);
    const playsOther: PlaybackDecision["plays"] = {
      titleId: 31,
      kind: "other",
      libraryId: 7,
      libraryName: "Shows",
    };
    up(navigation, playingUpward(playsOther));
    expect(navigation.route()).toEqual({ screen: "title", itemId: 31 });
    const video: TitleDetail = {
      id: 31,
      kind: "other",
      name: "Holiday 2019",
      identificationState: "identified",
      art: {},
      versions: [],
      extras: [],
      libraries: [shows],
    };
    up(navigation, () => titleUpward(video, { t: i18n.t, behind: navigation.behind() }));
    expect(navigation.route()).toEqual(folders("A/B"));
    up(navigation, () => folderUpward(shows, "A/B", folders));
    expect(historyOf(place, navigation)).toBe(
      "/ /browse?library=7&view=folders [/browse?library=7&view=folders&folder=A]",
    );
  });

  it("§3.17: an extra played goes up to its own page, then its film's, each behind; Back then Home", () => {
    const { place, navigation } = walked(filmPage, { screen: "title", itemId: 14 }, playing);
    const playsExtra: PlaybackDecision["plays"] = {
      titleId: 14,
      kind: "extra",
      libraryId: 3,
      filmOfExtra: { id: 10, name: "Metropolis" },
    };
    up(navigation, playingUpward(playsExtra));
    expect(historyOf(place, navigation)).toBe("/ /titles/10 [/titles/14]");
    const extra: TitleDetail = {
      id: 14,
      kind: "extra",
      name: "Making Metropolis",
      identificationState: "identified",
      art: {},
      versions: [],
      extras: [],
      libraries: [films],
      filmOfExtra: { id: 10, name: "Metropolis" },
    };
    up(navigation, () => titleUpward(extra, { t: i18n.t, behind: navigation.behind() }));
    expect(historyOf(place, navigation)).toBe("/ [/titles/10]");
    navigation.back();
    expect(navigation.route()).toEqual(home);
  });

  it("§3.18: a film two libraries list goes up to the one came through, else the first by name", () => {
    const heat: TitleDetail = {
      id: 10,
      kind: "movie",
      name: "Metropolis",
      identificationState: "identified",
      art: {},
      versions: [],
      extras: [],
      libraries: [films, kids],
    };
    const filmUpward = (navigation: Navigation): Upward =>
      titleUpward(heat, { t: i18n.t, behind: navigation.behind() });
    const throughKids = walked({ screen: "browse", library: 4 }, filmPage, playing);
    up(throughKids.navigation, playingUpward(playsFilm));
    up(throughKids.navigation, filmUpward);
    expect(historyOf(throughKids.place, throughKids.navigation)).toBe("/ [/browse?library=4]");
    const fromHome = walked(playing);
    up(fromHome.navigation, playingUpward(playsFilm));
    up(fromHome.navigation, filmUpward);
    expect(historyOf(fromHome.place, fromHome.navigation)).toBe("/ [/browse?library=3]");
  });
});
