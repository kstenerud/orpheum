// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it, vi } from "vitest";

import { Play } from "./play.tsx";
import { soundLatencyKey } from "./sound-latency.ts";
import { mount, playbackCauses, servedBy, type Heard, type Mounted } from "./test-dom.tsx";
import { directDecision } from "./test-fixtures.ts";
import { withServices } from "./test-services.tsx";
import type { PlaybackDecision } from "./watching.ts";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
  window.localStorage.removeItem(soundLatencyKey);
  vi.unstubAllGlobals();
});

/**
 * A browser's Web Audio, which happy-dom has not: a context the page may never start, so a sound
 * levelled through it plays as it is.
 */
function stubWebAudio(): void {
  vi.stubGlobal(
    "AudioContext",
    class {
      readonly state = "suspended";
      resume(): Promise<void> {
        return Promise.reject(new Error("not allowed to start"));
      }
    },
  );
}

/** The film's own file, levelling wanting its sound changed by gainDb. */
function ownFileLevelledBy(gainDb: number): PlaybackDecision {
  return directDecision({ levelling: { isMeasured: true, gainDb } });
}

/** The Play page of file fileId, the server answering each start with decision; the causes asked. */
async function playedCauses(fileId: number, decision: PlaybackDecision): Promise<unknown[]> {
  const heard: Heard[] = [];
  const services = servedBy(
    {
      "POST /playback": [200, decision],
      [`GET /files/${String(fileId)}/waits`]: [200, {}],
      [`GET /files/${String(fileId)}/up-next`]: [200, {}],
      [`GET /files/${String(fileId)}/availability`]: [200, {}],
      "POST /playback/t/report": [200, {}],
      "DELETE /playback/t": [204, null],
    },
    heard,
  );
  services.session.signIn("token", { deviceId: 5 });
  mounted = await mount(withServices(<Play fileId={fileId} />, services));
  return playbackCauses(heard);
}

describe("a raise on a device whose sound through Web Audio is late (D436)", () => {
  it("asks for its film again, onto its ladder, once", async () => {
    // The owner's TCL TV's latency, kept (staging, 2026-11-06).
    window.localStorage.setItem(soundLatencyKey, "426");
    expect(await playedCauses(31, ownFileLevelledBy(4.5))).toEqual(["playPage", "raiseWanted"]);
  });

  it("is not asked of a lowering, which the element's own volume makes", async () => {
    window.localStorage.setItem(soundLatencyKey, "426");
    expect(await playedCauses(32, ownFileLevelledBy(-11.6))).toEqual(["playPage"]);
  });

  it("is not asked where Web Audio is on time, or not yet measured", async () => {
    stubWebAudio();
    window.localStorage.setItem(soundLatencyKey, "12");
    expect(await playedCauses(33, ownFileLevelledBy(4.5))).toEqual(["playPage"]);
    mounted?.unmount();
    window.localStorage.removeItem(soundLatencyKey);
    expect(await playedCauses(34, ownFileLevelledBy(4.5))).toEqual(["playPage"]);
  });
});
