// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { afterEach, describe, expect, it } from "vitest";

import { decisionTimesOf } from "./decision-times.ts";
import {
  bodyField,
  failOnUnexpectedRequests,
  playbackCauses,
  servedBy,
  type Answer,
  type Heard,
} from "./test-dom.tsx";
import { soundLatencyKey } from "./sound-latency.ts";
import { testServices } from "./test-services.tsx";
import {
  isStartThePages,
  startOnFilmsPictureKey,
  Watching,
  warmUpKeptMs,
  type Preload,
  type StartPreload,
} from "./watching.ts";

// This file never mounts, so it owns the check unmount() would otherwise do for it (finding 2,
// test-architecture-2026-10-26.md; test-dom.tsx's servedBy doc).
afterEach(failOnUnexpectedRequests);

const playableFormats = testServices().playableFormats();

/** A decision naming its media token, on its film's ladder unless isCut is false, as POST /playback answers. */
function decided(mediaToken: string, isCut = true): Answer {
  const onLadder = {
    url: `/api/v1/bytes/media/chunks/${mediaToken}/master.m3u8`,
    startAtBps: 4_000_000,
    qualityKbps: 2400,
    height: 1080,
    isBelowTheLowest: false,
    soundCarriedGainDb: 0,
    isReady: false,
    filmPictureKbps: 2400,
  };
  return [
    200,
    {
      delivery: "direct",
      url: "/u",
      notDirectReasons: [],
      mediaToken,
      resumePositionMs: 40_000,
      ...(isCut ? { onLadder } : {}),
    },
  ];
}

/** The requests heard, by method and path, once every one sent so far has been answered. */
async function settled(heard: readonly Heard[]): Promise<string[]> {
  await new Promise((resolve) => setTimeout(resolve, 0));
  await new Promise((resolve) => setTimeout(resolve, 0));
  return heard.map((request) => request.methodAndPath);
}

/** The preloads a stand-in started: each one's playlist and position, whether it was stopped. */
interface Preloads {
  readonly started: { url: string; positionMs: number; isStopped: boolean }[];
  /** What every preload says it settled on, and its player's last steps. */
  settledAtBps: number | undefined;
  steps: string | undefined;
  readonly start: StartPreload;
}

function standInPreloads(): Preloads {
  const preloads: Preloads = {
    started: [],
    settledAtBps: undefined,
    steps: undefined,
    start: (offer, positionMs): Preload => {
      const preload = { url: offer.url, positionMs, isStopped: false };
      preloads.started.push(preload);
      return {
        settledAtBps: () => preloads.settledAtBps,
        steps: () => preloads.steps,
        stop: () => {
          preload.isStopped = true;
        },
      };
    },
  };
  return preloads;
}

/** A Watching over a stand-in server answering decisions a, b, c in turn, and a clock it moves. */
function watchingServed(
  heard: Heard[],
  preloads: Preloads = standInPreloads(),
): { watching: Watching; clock: { ms: number } } {
  const { api } = servedBy(
    {
      "POST /playback": [decided("a"), decided("b"), decided("c")],
      "DELETE /playback/a": [204, undefined],
      "DELETE /playback/b": [204, undefined],
    },
    heard,
  );
  const clock = { ms: 0 };
  return { watching: new Watching(api, () => clock.ms, preloads.start), clock };
}

describe("a playback asked for again because its player gave the film's own picture up (D323 §2)", () => {
  it("tells the core so, which no other start does", async () => {
    const heard: Heard[] = [];
    const { watching } = watchingServed(heard);
    await watching.decide(7, playableFormats);
    await watching.decide(7, playableFormats, { cause: "ownPictureGivenUp" });
    const asked = heard.filter((request) => request.methodAndPath === "POST /playback");
    expect(asked.map((request) => bodyField(request, "hasGivenUpOwnPicture"))).toEqual([
      undefined,
      true,
    ]);
  });
});

describe("a playback asked for on a device whose sound through Web Audio is late (D436)", () => {
  afterEach(() => {
    window.localStorage.removeItem(soundLatencyKey);
  });

  it("tells the core so, which a device never measured or on time does not", async () => {
    const heard: Heard[] = [];
    const { watching } = watchingServed(heard);
    await watching.decide(7, playableFormats);
    window.localStorage.setItem(soundLatencyKey, "12");
    await watching.decide(7, playableFormats, { cause: "playPage" });
    // The owner's TCL TV's latency, kept (staging, 2026-11-06).
    window.localStorage.setItem(soundLatencyKey, "426");
    await watching.decide(7, playableFormats, { cause: "playPage" });
    const asked = heard.filter((request) => request.methodAndPath === "POST /playback");
    expect(asked.map((request) => bodyField(request, "isWebAudioLate"))).toEqual([
      undefined,
      undefined,
      true,
    ]);
  });
});

describe("the film being watched (D258)", () => {
  it("plays on the decision its film's page warmed up, asking once", async () => {
    const heard: Heard[] = [];
    const { watching } = watchingServed(heard);
    watching.warmUp(7, playableFormats);
    await settled(heard);
    const pressedMs = performance.now();
    const decision = await watching.decide(7, playableFormats);
    expect(decision.mediaToken).toBe("a");
    expect(await settled(heard)).toEqual(["POST /playback"]);
    // Its start is timed from Play, not from opening the page.
    expect(decisionTimesOf("a")?.askedMs).toBeGreaterThanOrEqual(pressedMs);
  });

  it("keeps its playbacks when the person goes back to its page, and lets go of them only for another film", async () => {
    const heard: Heard[] = [];
    const { watching } = watchingServed(heard);
    await watching.decide(7, playableFormats);
    // Back to film 7's page, from the player: nothing is let go of, nor asked again.
    watching.warmUp(7, playableFormats);
    expect(await settled(heard)).toEqual(["POST /playback", "POST /playback"]);
    // Film 9's page: film 7's playbacks are let go of.
    watching.warmUp(9, playableFormats);
    expect(await settled(heard)).toEqual([
      "POST /playback",
      "POST /playback",
      "DELETE /playback/a",
      "DELETE /playback/b",
      "POST /playback",
    ]);
  });

  it("asks again when its page's decision has waited too long, or was already played on", async () => {
    const heard: Heard[] = [];
    const { watching, clock } = watchingServed(heard);
    watching.warmUp(7, playableFormats);
    clock.ms = warmUpKeptMs;
    expect((await watching.decide(7, playableFormats)).mediaToken).toBe("b");
    // Deciding again, as a changed sound does, asks the server again.
    expect((await watching.decide(7, playableFormats)).mediaToken).toBe("c");
  });

  it("asks again at Play when its page's decision came before the film was cut (D260 §1)", async () => {
    const heard: Heard[] = [];
    const { api } = servedBy(
      {
        "POST /playback": [decided("a", false), decided("b")],
        "DELETE /playback/a": [204, undefined],
      },
      heard,
    );
    const watching = new Watching(api, () => 0, standInPreloads().start);
    watching.warmUp(7, playableFormats);
    const decision = await watching.decide(7, playableFormats);
    expect(decision.mediaToken).toBe("b");
    expect(decision.onLadder).toBeDefined();
  });

  it("lets go of the warm-up, preload and playback, for one asked from where the page starts", async () => {
    // A Play here mid-film took the film page's warm-up, made for 0:00, which the server never
    // heard of, and played from 0:00 (staging, 2026-10-27).
    const heard: Heard[] = [];
    const preloads = standInPreloads();
    const { api } = servedBy(
      {
        "POST /playback": [decided("a"), decided("b")],
        "DELETE /playback/a": [204, undefined],
      },
      heard,
    );
    const watching = new Watching(api, () => 0, preloads.start);
    watching.warmUp(7, playableFormats);
    await settled(heard);
    const decision = await watching.decide(7, playableFormats, {
      cause: "castOver",
      startsAtMs: 982_680,
    });
    expect(decision.mediaToken).toBe("b");
    expect(preloads.started[0]?.isStopped).toBe(true);
    expect(await settled(heard)).toContain("DELETE /playback/a");
    const asked = heard.filter((request) => request.methodAndPath === "POST /playback");
    expect(playbackCauses(asked)).toEqual(["filmPage", "castOver"]);
    expect(asked.map((request) => bodyField(request, "startsAtMs"))).toEqual([undefined, 982_680]);
  });

  it("preloads nothing where its media go over WebRTC, and plays on the warm-up's decision (the efficiency audit, finding 1)", async () => {
    const heard: Heard[] = [];
    const preloads = standInPreloads();
    const { watching } = watchingServed(heard, preloads);
    watching.warmUp(7, playableFormats, () => true);
    await settled(heard);
    expect(preloads.started).toEqual([]);
    const decision = await watching.decide(7, playableFormats);
    expect(decision.mediaToken).toBe("a");
    expect(decision.onLadder?.startAtBps).toBe(4_000_000);
    expect(await settled(heard)).toEqual(["POST /playback"]);
    // On HTTP the browser keeps what the preload loaded for the Play page, so it preloads (D260 §3).
    watching.warmUp(9, playableFormats, () => false);
    await settled(heard);
    expect(preloads.started).toHaveLength(1);
  });

  it("lets go of the warm-up, preload and playback, of a film sent on to a cast's receiver (D364 §3)", async () => {
    const heard: Heard[] = [];
    const preloads = standInPreloads();
    const { watching } = watchingServed(heard, preloads);
    watching.warmUp(7, playableFormats);
    await settled(heard);
    watching.sentAway(8); // another film: its own warm-up is not this one
    expect(preloads.started[0]?.isStopped).toBe(false);
    watching.sentAway(7);
    expect(preloads.started[0]?.isStopped).toBe(true);
    expect(await settled(heard)).toEqual(["POST /playback", "DELETE /playback/a"]);
  });

  it("lets go of the warm-up's playback when it asks again at Play, and asks again for one that failed", async () => {
    const heard: Heard[] = [];
    const { api } = servedBy(
      {
        "POST /playback": [decided("a", false), decided("b"), [503, {}], decided("c")],
        "DELETE /playback/a": [204, undefined],
      },
      heard,
    );
    const watching = new Watching(api, () => 0, standInPreloads().start);
    watching.warmUp(7, playableFormats);
    expect((await watching.decide(7, playableFormats)).mediaToken).toBe("b");
    expect(await settled(heard)).toContain("DELETE /playback/a");
    watching.warmUp(7, playableFormats); // the server fails this warm-up
    expect((await watching.decide(7, playableFormats)).mediaToken).toBe("c");
  });

  it("tells the server why it asks for each playback, and how many the page has asked for (staging, 2026-10-14)", async () => {
    const heard: Heard[] = [];
    const { watching, clock } = watchingServed(heard);
    watching.warmUp(7, playableFormats);
    clock.ms = warmUpKeptMs;
    await watching.decide(7, playableFormats);
    await watching.decide(7, playableFormats, { cause: "subtitleFetched" });
    const offLadder: Heard[] = [];
    const { api } = servedBy(
      {
        "POST /playback": [decided("d", false), decided("e"), [503, {}], decided("f")],
        "DELETE /playback/d": [204, undefined],
      },
      offLadder,
    );
    const other = new Watching(api, () => 0, standInPreloads().start);
    other.warmUp(8, playableFormats);
    await other.decide(8, playableFormats);
    other.warmUp(8, playableFormats); // the server fails this warm-up
    await other.decide(8, playableFormats);
    await settled(offLadder);
    const asked = [...heard, ...offLadder].filter(
      (request) => request.methodAndPath === "POST /playback",
    );
    expect(playbackCauses(asked)).toEqual([
      "filmPage",
      "warmUpStale",
      "subtitleFetched",
      "filmPage",
      "warmUpOffLadder",
      "filmPage",
      "warmUpFailed",
    ]);
    const counts = asked.map((request) => Number(bodyField(request, "pageStarts")));
    expect(counts.map((count) => count - (counts[0] ?? 0))).toEqual([0, 1, 2, 3, 4, 5, 6]);
  });

  it("preloads a film on its ladder from where it resumes, and Play starts where the preload settled (D260 §3)", async () => {
    const preloads = standInPreloads();
    const { watching } = watchingServed([], preloads);
    watching.warmUp(7, playableFormats);
    await settled([]);
    expect(preloads.started).toEqual([
      { url: "/api/v1/bytes/media/chunks/a/master.m3u8", positionMs: 40_000, isStopped: false },
    ]);
    // It stepped down to the 1.2 Mb/s quality while the page was open.
    preloads.settledAtBps = 1_896_000;
    preloads.steps = "level 4; frag 4/0 loaded; level 1";
    const decision = await watching.decide(7, playableFormats);
    expect(decision.onLadder?.startAtBps).toBe(1_896_000);
    expect(preloads.started[0]?.isStopped).toBe(true);
    // Its steps go with the playback's startup report (the owner's test, 2026-10-18).
    expect(decisionTimesOf(decision.mediaToken)?.preloadSteps).toBe(
      "level 4; frag 4/0 loaded; level 1",
    );
  });

  // The player starts a playback's own offer by the link its device remembers, never a start the
  // page chose itself, a warm-up's settled quality or the ladder's top (D381).
  it("keeps a start the page chose as the page's, and the server's as none of the page's", async () => {
    const preloads = standInPreloads();
    const { watching } = watchingServed([], preloads);
    watching.warmUp(7, playableFormats);
    await settled([]);
    preloads.settledAtBps = 1_896_000;
    const settledOn = (await watching.decide(7, playableFormats)).onLadder;
    expect(settledOn !== undefined && isStartThePages(settledOn)).toBe(true);
    const unsettled = standInPreloads();
    const served = watchingServed([], unsettled);
    served.watching.warmUp(7, playableFormats);
    await settled([]);
    const asServed = (await served.watching.decide(7, playableFormats)).onLadder;
    expect(asServed !== undefined && isStartThePages(asServed)).toBe(false);
  });

  it("starts on the film's own picture with no preload, where this browser is set to (a debugging aid)", async () => {
    window.localStorage.setItem(startOnFilmsPictureKey, "1");
    const preloads = standInPreloads();
    const { watching } = watchingServed([], preloads);
    watching.warmUp(7, playableFormats);
    await settled([]);
    const decision = await watching.decide(7, playableFormats);
    window.localStorage.removeItem(startOnFilmsPictureKey);
    expect(preloads.started).toEqual([]);
    expect(decision.onLadder?.startAtBps).toBe(Number.MAX_SAFE_INTEGER);
  });

  it("stops the preload when another film is chosen, and starts none for a film not on its ladder", async () => {
    const preloads = standInPreloads();
    const { api } = servedBy({
      "POST /playback": [decided("a"), decided("b", false)],
      "DELETE /playback/a": [204, undefined],
    });
    const watching = new Watching(api, () => 0, preloads.start);
    watching.warmUp(7, playableFormats);
    await settled([]);
    watching.warmUp(9, playableFormats);
    await settled([]);
    expect(preloads.started.map((preload) => preload.isStopped)).toEqual([true]);
  });
});

describe("the next episode warmed up as Up next shows (the owner's request, 2026-11-11)", () => {
  it("is not a film page's warm-up, which lets go of the film playing, its runs stopped at once", async () => {
    const heard: Heard[] = [];
    const { watching } = watchingServed(heard);
    await watching.decide(7, playableFormats);
    watching.warmUp(8, playableFormats);
    expect(await settled(heard)).toEqual([
      "POST /playback",
      "DELETE /playback/a",
      "POST /playback",
    ]);
  });

  it("keeps the film playing's playbacks to its end, and its Play takes the warmed decision, asking nothing", async () => {
    const heard: Heard[] = [];
    const preloads = standInPreloads();
    const { api } = servedBy(
      {
        "POST /playback": [decided("a"), decided("b"), decided("c")],
        "DELETE /playback/a": [204, undefined],
        "DELETE /playback/c": [204, undefined],
      },
      heard,
    );
    const watching = new Watching(api, () => 0, preloads.start);
    await watching.decide(7, playableFormats);
    watching.warmUpNext(8, playableFormats);
    // Up next shown again, as the person moves back and forward over the end: asked once.
    watching.warmUpNext(8, playableFormats);
    // The film playing deciding again, as a changed sound does, keeps the next episode's warm-up.
    await watching.decide(7, playableFormats, { cause: "audioChosen" });
    expect(await settled(heard)).toEqual(["POST /playback", "POST /playback", "POST /playback"]);
    expect(playbackCauses(heard)).toEqual(["playPage", "upNext", "audioChosen"]);
    const decision = await watching.decide(8, playableFormats);
    expect(decision.mediaToken).toBe("b");
    // Its decision alone, nothing preloaded: the start is the server's (D381).
    expect(decision.onLadder?.startAtBps).toBe(4_000_000);
    // Only now are the film played's playbacks let go of.
    expect((await settled(heard)).slice(3)).toEqual(["DELETE /playback/a", "DELETE /playback/c"]);
  });

  it("is taken by its Play while it is still asked for, its playback kept as the one watched", async () => {
    const heard: Heard[] = [];
    const { watching } = watchingServed(heard);
    await watching.decide(7, playableFormats);
    watching.warmUpNext(8, playableFormats);
    expect((await watching.decide(8, playableFormats)).mediaToken).toBe("b");
    // In the order the stand-in server reads them, a POST's body read last.
    expect((await settled(heard)).toSorted()).toEqual([
      "DELETE /playback/a",
      "POST /playback",
      "POST /playback",
    ]);
    // Another film: the next episode's playback, now the one watched, is let go of.
    await watching.decide(9, playableFormats);
    expect((await settled(heard)).slice(3)).toEqual(["DELETE /playback/b", "POST /playback"]);
  });

  it("is let go of, its playback, when another film is chosen instead", async () => {
    const heard: Heard[] = [];
    const { watching } = watchingServed(heard);
    await watching.decide(7, playableFormats);
    watching.warmUpNext(8, playableFormats);
    await settled(heard);
    watching.warmUp(9, playableFormats);
    expect(await settled(heard)).toEqual([
      "POST /playback",
      "POST /playback",
      "DELETE /playback/a",
      "DELETE /playback/b",
      "POST /playback",
    ]);
  });

  it("is let go of once answered, preloading nothing, where another film was chosen while it was asked for", async () => {
    const heard: Heard[] = [];
    const preloads = standInPreloads();
    const { watching } = watchingServed(heard, preloads);
    await watching.decide(7, playableFormats);
    watching.warmUpNext(8, playableFormats);
    watching.warmUp(9, playableFormats);
    expect((await settled(heard)).toSorted()).toEqual([
      "DELETE /playback/a",
      "DELETE /playback/b",
      "POST /playback",
      "POST /playback",
      "POST /playback",
    ]);
    expect(preloads.started.map((preload) => preload.url)).toEqual([
      "/api/v1/bytes/media/chunks/c/master.m3u8",
    ]);
  });

  // A hidden player beside the film playing could take a television's hardware decoder from it
  // (the owner, 2026-11-12, "Decision only while playing").
  it("preloads nothing on any transport, where a film page's warm-up on HTTP preloads (D260 §3)", async () => {
    const preloads = standInPreloads();
    const { watching } = watchingServed([], preloads);
    await watching.decide(7, playableFormats);
    watching.warmUpNext(8, playableFormats);
    await settled([]);
    expect(preloads.started).toEqual([]);
    expect((await watching.decide(8, playableFormats)).mediaToken).toBe("b");
    // Continue watching's press, a film page's warm-up, keeps its preload.
    watching.warmUp(9, playableFormats, () => false);
    await settled([]);
    expect(preloads.started.map((preload) => preload.url)).toEqual([
      "/api/v1/bytes/media/chunks/c/master.m3u8",
    ]);
  });
});
