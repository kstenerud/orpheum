// SPDX-License-Identifier: MPL-2.0

import { playbackFailures } from "@orpheum/player";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, describe, expect, it } from "vitest";

import { i18n } from "./i18n.ts";
import { playbackFailureText } from "./film-player.tsx";
import {
  ladderAudioRenditionIndex,
  ladderSoundLevel,
  ladderStartingAudio,
  soundChoiceCause,
} from "./ladder-sound.ts";
import { PlayView, positionToMoveFrom } from "./play.tsx";
import { playUp, playUpward } from "./play-up.tsx";
import { failOnUnexpectedRequests, servedBy, type Heard } from "./test-dom.tsx";
import { directDecision, unmeasuredSound } from "./test-fixtures.ts";
import { requestPlayback } from "./watching.ts";
import { testServices, withServices } from "./test-services.tsx";

// This file never mounts, so it owns the check unmount() would otherwise do for it (finding 2,
// test-architecture-2026-10-26.md; test-dom.tsx's servedBy doc).
afterEach(failOnUnexpectedRequests);

describe("playing a file", () => {
  it("asks the server with this browser's playable formats, and nothing of where it is", async () => {
    const playableFormats = testServices().playableFormats();
    const heard: Heard[] = [];
    const { api } = servedBy(
      { "POST /playback": [200, { delivery: "direct", url: "/u", notDirectReasons: [] }] },
      heard,
    );
    const decision = await requestPlayback(api, 7, { playableFormats, cause: "playPage" });
    expect(heard).toEqual([
      {
        methodAndPath: "POST /playback",
        // Why it asked and how many times this page has, for the server's log alone.
        body: { fileId: 7, playableFormats, cause: "playPage", pageStarts: 1 },
      },
    ]);
    expect(decision.delivery).toBe("direct");
  });

  it("levels a ladder's sound by the gain it carries, or by the film's own levelling when it carries none (2026-10-24)", () => {
    const measured = directDecision({ levelling: { isMeasured: true, gainDb: 4.5 } });
    expect(ladderSoundLevel(measured, 3)).toEqual({
      levelling: { isMeasured: true, gainDb: 3 },
      carriedGainDb: 3,
    });
    // Offered unlevelled to a person who plays without levelling: measured, carrying nothing.
    expect(ladderSoundLevel(measured, 0)).toEqual({
      levelling: { isMeasured: true, gainDb: 4.5 },
      carriedGainDb: 0,
    });
  });

  it("asks for the film's own file when the person chose the original over its ladder", async () => {
    const playableFormats = testServices().playableFormats();
    const heard: Heard[] = [];
    const { api } = servedBy(
      { "POST /playback": [200, { delivery: "direct", url: "/u", notDirectReasons: [] }] },
      heard,
    );
    await requestPlayback(api, 7, { playableFormats, cause: "originalChosen" });
    expect(heard.map((each) => each.body)).toEqual([
      expect.objectContaining({ fileId: 7, cause: "originalChosen", preferredDelivery: "direct" }),
    ]);
  });

  it("plays the original in a video element and says so", () => {
    const markup = renderToStaticMarkup(
      withServices(
        <PlayView fileId={2} decision={directDecision()} onDecideAgain={() => undefined} />,
      ),
    );
    expect(markup).toContain('src="/api/v1/bytes/media/direct/abc"');
    expect(markup).toContain("playsInline");
    // Its tracks say it plays as it is, each the original (the owner, 2026-10-20).
    expect(markup).toContain("Video: H.264 1920×1080, 2.4 Mb/s (original)");
    expect(markup).toContain("Sound: AAC stereo (original)");
    // Levelling sits under the video, on as the person left it, and says what it does (D226).
    expect(markup).toContain("Level loudness");
    expect(markup).toContain("Its loudness is not measured yet, so it plays as it is.");
  });

  it("names the film above the picture, and nothing where the file is filed as no title", () => {
    const named = renderToStaticMarkup(
      withServices(
        <PlayView
          fileId={2}
          decision={directDecision({ titleName: "Metropolis" })}
          onDecideAgain={() => undefined}
        />,
      ),
    );
    expect(named).toMatch(
      /<\/nav><h1 class="play__title">Metropolis<\/h1><div class="play" data-focus-container="" data-focus-start="">/u,
    );
    const unnamed = renderToStaticMarkup(
      withServices(
        <PlayView fileId={2} decision={directDecision()} onDecideAgain={() => undefined} />,
      ),
    );
    expect(unnamed).not.toContain("play__title");
  });

  it("offers its trail up to the film's page, first on the page, and no Back of its own (D367 §6, §8)", () => {
    const markup = renderToStaticMarkup(
      withServices(
        <PlayView
          fileId={2}
          decision={directDecision({
            titleName: "Metropolis",
            plays: { itemId: 10, kind: "movie", libraryId: 1, libraryName: "Films" },
          })}
          onDecideAgain={() => undefined}
        />,
      ),
    );
    expect(markup).toMatch(
      /^<nav class="up-trail" aria-label="Where you are"><ol class="up-trail__crumbs"><li><a href="\/browse\?library=1">Films<\/a><\/li><li><a href="\/titles\/10">Metropolis<\/a><\/li><li aria-current="page">Playing<\/li><\/ol><\/nav>/u,
    );
    expect(markup).not.toContain("back-button");
  });

  it("puts what plays' own page last before the playing, a press up (the owner, 2026-10-28)", () => {
    const trailOf = (plays: Parameters<typeof directDecision>[0]): string => {
      const markup = renderToStaticMarkup(
        withServices(
          <PlayView fileId={2} decision={directDecision(plays)} onDecideAgain={() => undefined} />,
        ),
      );
      const crumbs = /<ol class="up-trail__crumbs">(.*?)<\/ol>/u.exec(markup)?.[1] ?? "";
      return [...crumbs.matchAll(/<li[^>]*>(.*?)<\/li>/gu)]
        .map((part) => part[1])
        .slice(-2)
        .join(" › ");
    };
    const episode = { seriesId: 20, seriesName: "Glass Tide", seasonNumber: 2, episodeNumber: 3 };
    expect(
      trailOf({
        titleName: "Low Water",
        plays: { itemId: 23, kind: "episode", libraryId: 1, episode },
      }),
    ).toBe('<a href="/titles/23">S2E3 Low Water</a> › Playing');
    expect(
      trailOf({
        titleName: "Making Metropolis",
        plays: { itemId: 12, kind: "extra", libraryId: 1, filmOfExtra: { id: 10, name: "Metropolis" } },
      }),
    ).toBe('<a href="/titles/12">Making Metropolis</a> › Playing');
    expect(
      trailOf({ titleName: "Holiday 2019", plays: { itemId: 30, kind: "other", libraryId: 4 } }),
    ).toBe('<a href="/titles/30">Holiday 2019</a> › Playing');
    expect(trailOf({ titleName: "Metropolis", plays: { itemId: 10, kind: "movie", libraryId: 1 } })).toBe(
      '<a href="/titles/10">Metropolis</a> › Playing',
    );
  });

  it("says a film to convert that came with no ladder cannot be converted, rather than play it (D344)", () => {
    const markup = renderToStaticMarkup(
      withServices(
        <PlayView
          fileId={2}
          decision={directDecision({
            delivery: "hls",
            profile: "remux",
            notDirectReasons: ["container"],
          })}
          onDecideAgain={() => undefined}
        />,
      ),
    );
    expect(markup).toContain("This file can&#x27;t be converted for this device.");
    expect(markup).not.toContain("<video");
  });

  it("plays a ladder's sound by its place among the ladder's sounds, each track's standard mix then its boosted one", () => {
    const converted = {
      codec: "aac",
      channelCount: 2,
      kbps: 128,
      isOriginal: false,
      ...unmeasuredSound,
    };
    const { tracks } = directDecision({
      tracks: {
        ...directDecision().tracks,
        sounds: [
          { audioTrackIndex: 0, isBoosted: false, ...converted },
          { audioTrackIndex: 0, isBoosted: true, ...converted },
          { audioTrackIndex: 1, isBoosted: false, ...converted },
        ],
      },
    });
    expect(ladderAudioRenditionIndex(tracks, { audioTrackIndex: 0, isBoosted: true })).toBe(1);
    expect(ladderAudioRenditionIndex(tracks, { audioTrackIndex: 0, isBoosted: false })).toBe(0);
    expect(ladderAudioRenditionIndex(tracks, { audioTrackIndex: 1, isBoosted: false })).toBe(2);
    // German has no centre to raise, and the file's own sounds are no ladder's.
    expect(ladderAudioRenditionIndex(tracks, { audioTrackIndex: 1, isBoosted: true })).toBe(
      undefined,
    );
    const original = directDecision().tracks;
    expect(ladderAudioRenditionIndex(original, { audioTrackIndex: 0, isBoosted: false })).toBe(
      undefined,
    );
    // It starts boosted as the person left the boost, else on its playlist's default.
    expect(ladderStartingAudio(tracks, { audioTrackIndex: 0, isBoosted: true })).toBe(1);
    expect(ladderStartingAudio(tracks, { audioTrackIndex: 0, isBoosted: false })).toBe(undefined);
    expect(ladderStartingAudio(tracks, { audioTrackIndex: 1, isBoosted: true })).toBe(undefined);
  });

  it("tells its player a sound chosen in place as the boost turned on or off, which its note says", () => {
    expect(soundChoiceCause({ audioTrackIndex: 0, isBoosted: true })).toBe("boostOn");
    expect(soundChoiceCause({ audioTrackIndex: 0, isBoosted: false })).toBe("boostOff");
  });

  it("offers the file's audio tracks, starting with the one the playback starts with", () => {
    const markup = renderToStaticMarkup(
      withServices(
        <PlayView
          fileId={2}
          decision={directDecision({
            audioTracks: [
              { streamId: 80, audioTrackIndex: 0, language: "en", kind: "plain" },
              { streamId: 81, audioTrackIndex: 1, language: "fr", kind: "commentary" },
            ],
            startingAudioStreamId: 81,
          })}
          onDecideAgain={() => undefined}
        />,
      ),
    );
    expect(markup).toContain("Sound");
    // Its chooser, showing the track it starts with, not a native select (plans/toolkit.md step 6).
    expect(markup).not.toContain("<select");
    expect(markup).toMatch(/class="route-chooser__button"[^>]*>French · commentary</u);
  });

  it("words every playback failure by the player's code, never the browser's text", () => {
    const t = i18n.t.bind(i18n);
    for (const failure of playbackFailures) {
      const text = playbackFailureText(t, failure);
      expect(text).not.toContain("play.failure");
      expect(text).not.toBe("");
      expect(text).not.toBe(failure);
    }
    const texts = playbackFailures.map((failure) => playbackFailureText(t, failure));
    expect(new Set(texts).size).toBe(texts.length);
    expect(playbackFailureText(t, "unsupported")).toBe("this browser does not play its format");
  });
});

describe("where a player moving to a lower quality goes on from", () => {
  it("is where its video is once it has loaded, and where it was to start before then", () => {
    expect(positionToMoveFrom({ readyState: 4, currentTime: 3090 }, 3_082_298)).toBe(3_090_000);
    // Not loaded, the video reads 0: Bram resumed at 51:22 went on from its first minute.
    expect(positionToMoveFrom({ readyState: 0, currentTime: 0 }, 3_082_298)).toBe(3_082_298);
    expect(positionToMoveFrom(null, 3_082_298)).toBe(3_082_298);
  });
});

describe("the page above a Play page, where its Back goes with no page of this app behind it", () => {
  it("is what plays' own page, a film's, an extra's, an episode's or an other video's", () => {
    expect(playUp({ itemId: 10, kind: "movie", libraryId: 1 })).toEqual({
      screen: "title",
      itemId: 10,
    });
    expect(
      playUp({ itemId: 12, kind: "extra", libraryId: 1, filmOfExtra: { id: 10, name: "Metropolis" } }),
    ).toEqual({ screen: "title", itemId: 12 });
    const aired = { seriesId: 20, seriesName: "Glass Tide", seasonNumber: 2, episodeNumber: 3 };
    expect(playUp({ itemId: 23, kind: "episode", libraryId: 1, episode: aired })).toEqual({
      screen: "title",
      itemId: 23,
    });
    expect(playUp({ itemId: 30, kind: "other", libraryId: 4 })).toEqual({
      screen: "title",
      itemId: 30,
    });
  });

  it("has above it an extra's film, an episode's series at its season or group, an other video's library", () => {
    const grandparent = (plays: Parameters<typeof playUpward>[0]): unknown =>
      playUpward(plays, "", { t: i18n.t, behind: [] }).crumbs.at(-2)?.to;
    expect(
      grandparent({
        itemId: 12,
        kind: "extra",
        libraryId: 1,
        filmOfExtra: { id: 10, name: "Metropolis" },
      }),
    ).toEqual({ screen: "title", itemId: 10 });
    const aired = { seriesId: 20, seriesName: "Glass Tide", seasonNumber: 2, episodeNumber: 3 };
    expect(grandparent({ itemId: 23, kind: "episode", libraryId: 1, episode: aired })).toEqual({
      screen: "series-page",
      collectionId: 20,
      season: 2,
    });
    const inDVDOrder = {
      ...aired,
      ordering: "tmdb:dvd",
      inOrdering: { groupNumber: 1, numberInGroup: 4 },
    };
    expect(grandparent({ itemId: 23, kind: "episode", libraryId: 1, episode: inDVDOrder })).toEqual(
      {
        screen: "series-page",
        collectionId: 20,
        season: 1,
      },
    );
    // An ordering that leaves it out: the series' top.
    const leftOut = { ...aired, ordering: "tmdb:dvd" };
    expect(grandparent({ itemId: 23, kind: "episode", libraryId: 1, episode: leftOut })).toEqual({
      screen: "series-page",
      collectionId: 20,
    });
    expect(grandparent({ itemId: 30, kind: "other", libraryId: 4 })).toEqual({
      screen: "browse",
      library: 4,
    });
    expect(grandparent({ itemId: 30, kind: "other" })).toEqual({ screen: "home" });
  });

  it("is home where the server does not say what it plays: a file filed as no title, an older server", () => {
    expect(playUp(undefined)).toEqual({ screen: "home" });
  });
});
