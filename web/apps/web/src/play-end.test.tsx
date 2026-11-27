// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act } from "react";
import { afterEach, describe, expect, it } from "vitest";

import type { UpNext } from "./around-file.ts";
import { Play } from "./play.tsx";
import type { Route } from "./router.ts";
import {
  mount,
  servedBy,
  settleUntil,
  type Answer,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { directDecision } from "./test-fixtures.ts";
import { withServices } from "./test-services.tsx";
import type { PlaybackDecision } from "./watching.ts";

let mounted: Mounted | undefined;

/** What a playback plays, as its decision says it. */
type Plays = NonNullable<PlaybackDecision["plays"]>;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

/** A film, item 10, which file 2 plays. */
const film: Plays = {
  itemId: 10,
  kind: "film",
  libraryId: 1,
  libraryName: "Films",
};

/** Season 2's third episode, item 23, which file 2 plays. */
const episode: Plays = {
  itemId: 23,
  kind: "episode",
  libraryId: 1,
  episode: { seriesId: 20, seriesName: "Glass Tide", seasonNumber: 2, episodeNumber: 3 },
};

/** The episode after it, which file 4 plays. */
const nextEpisode: UpNext = {
  episode: {
    itemId: 24,
    name: "High Water",
    groupNumber: 2,
    numberInGroup: 4,
    fileId: 4,
    isPartway: false,
  },
  ordering: "aired",
};

/**
 * File 2's Play page, opened over the routes before (the first page /films), playing plays, its server saying upNext of what
 * plays after it; its video, played and then ended, by the end returned.
 */
async function playedToItsEnd(
  plays: Plays,
  around: Answer,
  before: readonly Route[],
): Promise<{ readonly services: ReturnType<typeof servedBy>; readonly end: () => void }> {
  const heard: Heard[] = [];
  const decision = directDecision({ resumePositionMs: 0, titleName: "Low Water", plays });
  const services = servedBy(
    {
      "POST /playback": [200, decision],
      "GET /files/2/waits": [200, {}],
      "GET /files/2/up-next": around,
      "GET /files/2/trickplay": [404, {}],
      "POST /playback/t/report": [204, null],
      "PUT /playback/t/wants": [204, null],
      "DELETE /playback/t": [204, null],
    },
    heard,
  );
  services.session.signIn("token", { deviceId: 5 });
  for (const route of before) {
    services.navigation.go(route);
  }
  services.navigation.go({ screen: "play", fileId: 2 });
  mounted = await mount(withServices(<Play fileId={2} />, services));
  const shown = mounted;
  await settleUntil(() => shown.container.querySelector("video") !== null);
  const video = shown.container.querySelector("video");
  if (video === null) {
    throw new Error("no video");
  }
  let hasEnded = false;
  Object.defineProperties(video, {
    paused: { get: () => hasEnded },
    ended: { get: () => hasEnded },
  });
  act(() => {
    video.dispatchEvent(new Event("playing"));
  });
  await settleUntil(() =>
    heard.some((request) => request.methodAndPath === "GET /files/2/up-next"),
  );
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 20));
  });
  return {
    services,
    end: () => {
      hasEnded = true;
      act(() => {
        video.dispatchEvent(new Event("ended"));
      });
    },
  };
}

describe("a video played to its end with nothing next (the owner, 2026-11-17)", () => {
  it("leaves a film's player for the film's page, back to its entry, as the player's Up does", async () => {
    const before: Route[] = [{ screen: "title", itemId: 10 }];
    const { services, end } = await playedToItsEnd(film, [200, {}], before);
    end();
    expect(services.navigation.route()).toEqual({ screen: "title", itemId: 10 });
    expect(services.navigation.behind()).toEqual([{ screen: "films" }]);
  });

  it("leaves the last episode's player for the episode's own page, in the player's place where nothing of it is behind", async () => {
    const { services, end } = await playedToItsEnd(episode, [200, {}], []);
    end();
    expect(services.navigation.route()).toEqual({ screen: "title", itemId: 23 });
    expect(services.navigation.behind()).toEqual([{ screen: "films" }]);
  });

  it("leaves where what plays next cannot be read, a next that cannot be read not offered", async () => {
    const { services, end } = await playedToItsEnd(episode, [500, {}], []);
    end();
    expect(services.navigation.route()).toEqual({ screen: "title", itemId: 23 });
  });

  it("stays on the player where an episode plays next, its Up next offered there", async () => {
    const { services, end } = await playedToItsEnd(episode, [200, { upNext: nextEpisode }], []);
    end();
    expect(services.navigation.route()).toEqual({ screen: "play", fileId: 2 });
    expect(mounted?.container.textContent).toContain("High Water");
  });
});
