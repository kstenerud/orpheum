// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { ownAudioTracksOf, switchOwnTrack, type OwnAudioTracks } from "./own-tracks.ts";

/** An element's list of three audio tracks, the first playing, as Safari's lists a file's. */
function threeTracks(): OwnAudioTracks {
  return { length: 3, 0: { enabled: true }, 1: { enabled: false }, 2: { enabled: false } };
}

describe("switching a file played as it is to another of its tracks, in place (the owner's answers of 2026-10-31)", () => {
  it("enables the track chosen and no other, where the element lists its tracks, as Safari's does", () => {
    const tracks = threeTracks();
    expect(switchOwnTrack(tracks, 2)).toBe(true);
    expect([tracks[0]?.enabled, tracks[1]?.enabled, tracks[2]?.enabled]).toEqual([
      false,
      false,
      true,
    ]);
  });

  it("does nothing, and says so, where the element lists none, as Chrome's and Firefox's do not, or lacks the track", () => {
    expect(switchOwnTrack(undefined, 1)).toBe(false);
    const tracks = threeTracks();
    expect(switchOwnTrack(tracks, 3)).toBe(false);
    expect(tracks[0]?.enabled).toBe(true);
  });

  it("finds the element's list by the feature, not the browser's name", () => {
    const listing = { audioTracks: threeTracks() } as unknown as HTMLMediaElement;
    expect(ownAudioTracksOf(listing)?.length).toBe(3);
    expect(ownAudioTracksOf({} as HTMLMediaElement)).toBeUndefined();
  });
});
