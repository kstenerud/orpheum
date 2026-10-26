// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import type { TogetherConnection, TogetherSnapshot } from "./together-connection.ts";
import { cooperativeRunwayS, GroupLoading } from "./together-loading.ts";
import { loadingRunwayEvent, loadingRunwayOf } from "./together-player-events.ts";
import type { RosterMember } from "./together.ts";

/** A connection whose snapshot lists roster, this device being 9. */
function connectionWith(roster: () => readonly RosterMember[]): TogetherConnection {
  return {
    snapshot: () => ({ deviceId: 9, roster: roster() }) as unknown as TogetherSnapshot,
    note: () => undefined,
  } as unknown as TogetherConnection;
}

/** A member with one player device, loading or not. */
function memberWith(deviceId: number, isBuffering: boolean): RosterMember {
  return {
    id: deviceId,
    name: "someone",
    isHost: false,
    devices: [
      { id: deviceId, role: "video", isBuffering, cannotKeepUp: undefined, status: undefined },
    ],
  };
}

describe("a short runway while another player is still loading (D285 §2)", () => {
  it("keeps the runway while the group plays and another is loading, and lets it go after", () => {
    const media = new EventTarget();
    const runways: (number | undefined)[] = [];
    media.addEventListener(loadingRunwayEvent, (event) => {
      runways.push(loadingRunwayOf(event)?.aheadS);
    });
    let roster = [memberWith(9, false), memberWith(14, true)];
    const loading = new GroupLoading(
      media,
      connectionWith(() => roster),
    );
    loading.follow("Playing", true);
    expect(runways).toEqual([cooperativeRunwayS]);
    loading.follow("Playing", true); // told once
    roster = [memberWith(9, false), memberWith(14, false)];
    loading.follow("Playing", true);
    expect(runways).toEqual([cooperativeRunwayS, undefined]);
    roster = [memberWith(9, true), memberWith(14, false)]; // only itself loading: no runway
    loading.follow("Playing", false);
    expect(runways).toEqual([cooperativeRunwayS, undefined]);
  });
});
