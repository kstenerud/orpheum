// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { sendFailuresTo } from "@orpheum/sdk";
import { act } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import {
  mediaBackAfterFailureMs,
  mediaBackAskAgainMs,
  mediaBackWaitSeconds,
  usePlaybackFailure,
} from "./playback-failure.ts";
import { Play, PlayView } from "./play.tsx";
import type { ReopenAt } from "./reopen.ts";
import {
  bodyField,
  fire,
  mount,
  playbackCauses,
  politeNotices,
  servedBy,
  settleUntil,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { directDecision } from "./test-fixtures.ts";
import { withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

describe("a connection too slow for the lowest quality", () => {
  // The owner, 2026-11-17: the dialog at the lowest quality (D506 §3) replaces the note (D506 §5).
  it("is no note under the film nor an alert: the dialog at the lowest quality says it (D506 §5)", async () => {
    const services = servedBy({
      "GET /files/2/waits": [200, {}],
      "GET /files/2/up-next": [200, {}],
      "GET /files/2/availability": [200, {}],
      "POST /playback/t/report": [200, {}],
      "DELETE /playback/t": [204, null],
    });
    const belowTheLowest = {
      url: "/api/v1/bytes/media/chunks/t/master.m3u8",
      startAtBps: 1_896_000,
      qualityKbps: 1200,
      height: 480,
      isBelowTheLowest: true,
      soundCarriedGainDb: 0,
      isReady: true,
      filmPictureKbps: 4000,
      linkSharePercent: 70,
      steadiness: 0,
    };
    mounted = await mount(
      withServices(
        <PlayView
          fileId={2}
          decision={directDecision({ onLadder: belowTheLowest })}
          onDecideAgain={() => undefined}
        />,
        services,
      ),
    );
    expect(politeNotices(mounted.container).join(" ")).not.toContain("connection");
    const alerts = [...mounted.container.querySelectorAll('[role="alert"]')];
    expect(alerts.map((each) => each.textContent).join(" ")).not.toContain("connection");
  });
});

describe("a playback that fails (D241 §7)", () => {
  it("asks the server why, and says its file's folder is missing", async () => {
    const services = servedBy({
      "GET /files/2/waits": [200, {}],
      "GET /files/2/up-next": [200, {}],
      "GET /files/2/availability": [
        200,
        { unavailable: { reason: "rootMissing", nodeName: "devnas", path: "/media/films" } },
      ],
      "POST /playback/t/report": [200, {}],
      "DELETE /playback/t": [204, null],
    });
    mounted = await mount(
      withServices(
        <PlayView fileId={2} decision={directDecision()} onDecideAgain={() => undefined} />,
        services,
      ),
    );
    await fire(mounted.container.querySelector("video"), new Event("error"));
    expect(mounted.container.querySelector('[role="alert"]')?.textContent).toContain(
      "This cannot be played now: the folder /media/films on devnas is missing.",
    );
  });

  it("says the film could not play, not a prepared copy, for a film on its ladder (the owner's test, 2026-10-18)", async () => {
    const services = servedBy({
      "GET /files/2/waits": [200, {}],
      "GET /files/2/up-next": [200, {}],
      "GET /files/2/availability": [200, {}],
      "POST /playback/t/report": [200, {}],
      "DELETE /playback/t": [204, null],
    });
    const onLadder = {
      url: "/api/v1/bytes/media/chunks/t/master.m3u8",
      startAtBps: 1_896_000,
      qualityKbps: 1200,
      height: 480,
      isBelowTheLowest: false,
      soundCarriedGainDb: 0,
      isReady: true,
      filmPictureKbps: 4000,
      linkSharePercent: 70,
      steadiness: 0,
    };
    mounted = await mount(
      withServices(
        <PlayView
          fileId={2}
          decision={directDecision({ onLadder })}
          onDecideAgain={() => undefined}
        />,
        services,
      ),
    );
    await fire(mounted.container.querySelector("video"), new Event("error"));
    const alert = mounted.container.querySelector('[role="alert"]')?.textContent ?? "";
    expect(alert).toMatch(/^The film could not play: /u);
    expect(alert).not.toContain("prepared copy");
  });

  it("tells the server the player failed, in its own words (the owner's test, 2026-10-18)", async () => {
    const sent: string[] = [];
    sendFailuresTo((text) => sent.push(text));
    const services = servedBy({
      "GET /files/2/waits": [200, {}],
      "GET /files/2/up-next": [200, {}],
      "GET /files/2/availability": [200, {}],
      "POST /playback/t/report": [200, {}],
      "DELETE /playback/t": [204, null],
    });
    mounted = await mount(
      withServices(
        <PlayView fileId={2} decision={directDecision()} onDecideAgain={() => undefined} />,
        services,
      ),
    );
    await fire(mounted.container.querySelector("video"), new Event("error"));
    expect(sent.some((text) => text.startsWith("Error: the player failed: "))).toBe(true);
    sendFailuresTo(() => undefined);
  });

  it("tells the server what failed where the player says, which its words alone never did (staging, 2026-10-29)", async () => {
    const sent: string[] = [];
    sendFailuresTo((text) => sent.push(text));
    const services = servedBy({ "GET /files/2/availability": [200, {}] });
    let fail: ((seen: string, whatFailed?: string) => void) | undefined;
    function Failing(): null {
      [, fail] = usePlaybackFailure(2);
      return null;
    }
    mounted = await mount(withServices(<Failing />, services));
    await act(async () => {
      fail?.("the browser gave no reason", "otherError internalException (fatal): x; status none");
      await Promise.resolve();
    });
    expect(sent).toContainEqual(
      expect.stringMatching(
        /^Error: the player failed: the browser gave no reason \(otherError internalException \(fatal\): x; status none\)/u,
      ),
    );
    sendFailuresTo(() => undefined);
  });

  it("keeps the player's own words when the file can be read", async () => {
    const services = servedBy({
      "GET /files/2/waits": [200, {}],
      "GET /files/2/up-next": [200, {}],
      "GET /files/2/availability": [200, {}],
      "POST /playback/t/report": [200, {}],
      "DELETE /playback/t": [204, null],
    });
    mounted = await mount(
      withServices(
        <PlayView fileId={2} decision={directDecision()} onDecideAgain={() => undefined} />,
        services,
      ),
    );
    await fire(mounted.container.querySelector("video"), new Event("error"));
    const alert = mounted.container.querySelector('[role="alert"]')?.textContent ?? "";
    expect(alert).not.toContain("cannot be played now");
    expect(alert).not.toBe("");
  });
});

describe("a playback its film's being unreachable stopped (the owner, 2026-10-21)", () => {
  it("asks again until the film can be read, then is opened again from where it stood", async () => {
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"], shouldAdvanceTime: true });
    try {
      const unavailable = {
        unavailable: {
          reason: "rootMissing",
          nodeName: "orpheum",
          path: "/tank/media/video/movies",
        },
      };
      const heard: Heard[] = [];
      const services = servedBy(
        {
          "GET /files/2/waits": [200, {}],
          "GET /files/2/up-next": [200, {}],
          "GET /files/2/availability": [
            [200, unavailable],
            [200, unavailable],
            [200, {}],
          ],
          "POST /playback/t/report": [200, {}],
          "DELETE /playback/t": [204, null],
        },
        heard,
      );
      const back: ReopenAt[] = [];
      mounted = await mount(
        withServices(
          <PlayView
            fileId={2}
            decision={directDecision()}
            onDecideAgain={() => undefined}
            onMediaBack={(at) => back.push(at)}
          />,
          services,
        ),
      );
      await fire(mounted.container.querySelector("video"), new Event("error"));
      expect(mounted.container.querySelector('[role="alert"]')?.textContent).toContain(
        "/tank/media/video/movies",
      );
      const asks = (): Heard[] =>
        heard.filter((each) => each.methodAndPath.endsWith("/availability"));
      await settleUntil(() => asks().length === 2); // asked again at once, held, still unreachable
      expect(asks()[1]?.query).toBe(`?waitSeconds=${String(mediaBackWaitSeconds)}`);
      expect(back).toEqual([]);
      act(() => {
        vi.advanceTimersByTime(mediaBackAskAgainMs);
      });
      await settleUntil(() => back.length > 0);
      expect(back).toEqual([{ positionMs: 0, isPaused: true }]);
      act(() => {
        vi.advanceTimersByTime(3 * mediaBackAfterFailureMs);
      });
      expect(back).toHaveLength(1); // told once
      expect(asks()).toHaveLength(3);
    } finally {
      vi.useRealTimers();
    }
  });

  it("is opened again once, though the page draws itself again as it opens it (staging, 2026-10-21)", async () => {
    // The page holds the film it played on screen while it asks for the new playback, drawn again
    // with a new onMediaBack; a page that asked again whether the film could be read then was told
    // at once that it could, and opened it a second time, 60 ms after the first.
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"], shouldAdvanceTime: true });
    try {
      const unavailable = {
        unavailable: { reason: "rootMissing", nodeName: "orpheum", path: "/tank/media/video" },
      };
      const heard: Heard[] = [];
      const services = servedBy(
        {
          "GET /files/2/waits": [200, {}],
          "GET /files/2/up-next": [200, {}],
          "POST /playback": [
            [200, directDecision()],
            [200, directDecision({ mediaToken: "again" })],
          ],
          "GET /files/2/availability": [
            [200, unavailable],
            [200, unavailable],
            [200, {}],
          ],
          "POST /playback/t/report": [200, {}],
          "DELETE /playback/t": [204, null],
          "POST /playback/again/report": [200, {}],
          "DELETE /playback/again": [204, null],
        },
        heard,
      );
      services.session.signIn("token");
      mounted = await mount(withServices(<Play fileId={2} />, services));
      await fire(mounted.container.querySelector("video"), new Event("error"));
      const asks = (): Heard[] =>
        heard.filter((each) => each.methodAndPath.endsWith("/availability"));
      await settleUntil(() => asks().length === 2);
      act(() => {
        vi.advanceTimersByTime(mediaBackAskAgainMs);
      });
      await settleUntil(() => playbackCauses(heard).length === 2);
      act(() => {
        vi.advanceTimersByTime(3 * mediaBackAfterFailureMs);
      });
      let turns = 0;
      await settleUntil(() => (turns += 1) > 20); // what a second opening would ask, answered
      expect(playbackCauses(heard)).toEqual(["playPage", "mediaBack"]);
    } finally {
      vi.useRealTimers();
    }
  });
});

/** How long a load that has left a player unable to play lasts before it counts as a stall (D247 §2). */
const slowLoadMs = 8000;

describe("a page that failed reports no stalls (D533 §3)", () => {
  /**
   * Mounts a film's page whose player is asked to play and has loaded nothing, as on staging
   * (2026-11-22): its first load counts as a stall each slowLoadMs. Returns the requests heard and
   * the video.
   */
  async function mountWaitingToLoad(): Promise<{
    readonly heard: Heard[];
    readonly video: HTMLVideoElement;
  }> {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "GET /files/2/waits": [200, {}],
        "GET /files/2/up-next": [200, {}],
        "GET /files/2/availability": [200, {}],
        "POST /playback/t/report": [200, {}],
        "DELETE /playback/t": [204, null],
      },
      heard,
    );
    mounted = await mount(
      withServices(
        <PlayView fileId={2} decision={directDecision()} onDecideAgain={() => undefined} />,
        services,
      ),
    );
    const video = mounted.container.querySelector("video");
    if (video === null) {
      throw new Error("no video");
    }
    await fire(video, new Event("loadstart")); // asked to play, nothing loaded yet
    return { heard, video };
  }

  /** The stalls the reports heard carry. */
  function stallsHeard(heard: readonly Heard[]): unknown[] {
    return heard
      .filter((each) => each.methodAndPath === "POST /playback/t/report")
      .flatMap((each) => {
        const stalls: unknown = bodyField(each, "stalls");
        return Array.isArray(stalls) ? (stalls as unknown[]) : [];
      });
  }

  it("reports the stall of a player waiting to load that has not failed", async () => {
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"], shouldAdvanceTime: true });
    try {
      const { heard } = await mountWaitingToLoad();
      act(() => {
        vi.advanceTimersByTime(slowLoadMs);
      });
      await settleUntil(() => stallsHeard(heard).length > 0);
      expect(stallsHeard(heard)).toHaveLength(1);
    } finally {
      vi.useRealTimers();
    }
  });

  it("reports no stall of its player once the page shows the film could not play", async () => {
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"], shouldAdvanceTime: true });
    try {
      const { heard, video } = await mountWaitingToLoad();
      await fire(video, new Event("error"));
      expect(mounted?.container.querySelector('[role="alert"]')).not.toBeNull();
      for (let wait = 0; wait < 3; wait += 1) {
        act(() => {
          vi.advanceTimersByTime(slowLoadMs);
        });
        let turns = 0;
        await settleUntil(() => (turns += 1) > 5); // what a stall would send, answered
      }
      expect(stallsHeard(heard)).toEqual([]);
    } finally {
      vi.useRealTimers();
    }
  });
});
