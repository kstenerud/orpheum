// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { toldTheCastHowItPlays, workedByTheRemote } from "./receiver-panels.ts";
import type { TellsPlaybackAlone, TogetherConnection } from "./together-connection.ts";
import { playerSwitchedQualityEvent } from "./together-player-events.ts";
import type { PlaybackReport } from "./together.ts";

/** A cast's session as a receiver tells it how it plays: each report and the playback read. */
interface CastSession extends TellsPlaybackAlone {
  readonly reports: (PlaybackReport | undefined)[];
  readonly reads: () => (() => PlaybackReport | undefined) | undefined;
}

function castSession(): CastSession {
  const reports: (PlaybackReport | undefined)[] = [];
  let read: (() => PlaybackReport | undefined) | undefined;
  return {
    reports,
    reads: () => read,
    setPlayback: (next) => {
      read = next;
    },
    reportPlayback: () => {
      reports.push(read?.());
    },
  };
}

describe("a receiver's panels worked by its cast's remote (D555 §3)", () => {
  it("carries each choice out by its panel and tells both sessions each change of it", () => {
    const chosen: string[] = [];
    let heardChoice: ((choice: string) => void) | undefined;
    let panelChanged: (() => void) | undefined;
    const cast = castSession();
    let ledReports = 0;
    const led = {
      reportState: () => {
        ledReports += 1;
      },
    } as unknown as TogetherConnection;
    const stop = workedByTheRemote<string>(
      (listener) => {
        heardChoice = listener;
        return () => {
          heardChoice = undefined;
        };
      },
      {
        subscribe: (listener) => {
          panelChanged = listener;
          return () => {
            panelChanged = undefined;
          };
        },
        choose: (choice) => {
          chosen.push(choice);
        },
      },
      { cast, led },
    );
    heardChoice?.("subtitles 2");
    expect(chosen).toEqual(["subtitles 2"]);
    panelChanged?.();
    expect(ledReports).toBe(1);
    expect(cast.reports).toHaveLength(1);
    stop();
    expect(heardChoice).toBeUndefined();
    expect(panelChanged).toBeUndefined();
  });

  it("tells the cast how its film plays, again at each quality its player began, until stopped", () => {
    const cast = castSession();
    const media = new EventTarget();
    const stop = toldTheCastHowItPlays(cast, media, { delivery: "converted" });
    expect(cast.reads()?.()).toEqual({ delivery: "converted" });
    media.dispatchEvent(
      new CustomEvent(playerSwitchedQualityEvent, {
        detail: { qualityKbps: 4200, linkEstimateKbps: 18_000 },
      }),
    );
    expect(cast.reports).toEqual([{ delivery: "converted", qualityKbps: 4200, linkKbps: 18_000 }]);
    stop();
    expect(cast.reads()).toBeUndefined();
    media.dispatchEvent(
      new CustomEvent(playerSwitchedQualityEvent, { detail: { qualityKbps: 1 } }),
    );
    expect(cast.reports).toHaveLength(1);
  });
});
