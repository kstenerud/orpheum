// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it, vi } from "vitest";

import type { PlayerNote } from "./together-connection.ts";
import {
  playedMutedEvent,
  playOrPlayMuted,
  readyForSoundAtTap,
  soundOnAtTap,
  soundToldAgainAfterMs,
} from "./together-sound.ts";

/**
 * A stand-in video, paused at 61 s, whose browser refuses a play for refusal's reason:
 * "NotAllowedError" one with sound that no recent tap started, "AbortError" any; or plays.
 */
class FakeVideo {
  currentTime = 61;
  /* eslint-disable @typescript-eslint/naming-convention -- HTMLMediaElement's own names, which this stands in for */
  muted = false;
  paused = true;
  ended = false;
  /* eslint-enable @typescript-eslint/naming-convention -- the fake's own names follow */
  readonly dispatched: string[] = [];

  constructor(private readonly refusal?: "NotAllowedError" | "AbortError") {}

  play(): Promise<void> {
    if (this.refusal === "AbortError" || (this.refusal === "NotAllowedError" && !this.muted)) {
      return Promise.reject(new DOMException("refused", this.refusal));
    }
    this.paused = false;
    return Promise.resolve();
  }

  dispatchEvent(event: Event): boolean {
    this.dispatched.push(event.type);
    return true;
  }
}

/** A connection that keeps the notes it is given. */
function noting(): { readonly notes: PlayerNote[]; readonly note: (note: PlayerNote) => void } {
  const notes: PlayerNote[] = [];
  return {
    notes,
    note: (note) => {
      notes.push(note);
    },
  };
}

describe("a film the browser will not start with sound", () => {
  it("plays muted, and says so to its page and to the server's log", async () => {
    const video = new FakeVideo("NotAllowedError");
    const connection = noting();
    playOrPlayMuted(video, connection);
    await vi.waitFor(() => {
      expect(video.paused).toBe(false);
    });
    expect(video.muted).toBe(true);
    expect(video.dispatched).toEqual([playedMutedEvent]);
    expect(connection.notes).toEqual([{ kind: "playedMuted", positionMs: 61_000 }]);
  });

  it("stays paused, its sound on, when the browser refuses for another reason", async () => {
    const video = new FakeVideo("AbortError");
    const connection = noting();
    playOrPlayMuted(video, connection);
    await Promise.resolve();
    await Promise.resolve();
    expect(video.paused).toBe(true);
    expect(video.muted).toBe(false);
    expect(connection.notes).toEqual([]);
  });

  it("turns its sound on at the person's tap, playing it if it was paused, and says so, then again", () => {
    vi.useFakeTimers();
    const video = Object.assign(new FakeVideo(), { volume: 1 });
    video.muted = true;
    const connection = noting();
    soundOnAtTap(video, connection);
    expect(video.muted).toBe(false);
    expect(video.paused).toBe(false);
    const state = "muted=false volume=1.00 paused=false session=none";
    expect(connection.notes).toEqual([{ kind: "unmuted", positionMs: 61_000, soundState: state }]);
    video.muted = true; // something muted it again after the tap
    vi.advanceTimersByTime(soundToldAgainAfterMs);
    expect(connection.notes.at(-1)).toEqual({
      kind: "unmuted",
      why: "after",
      positionMs: 61_000,
      soundState: "muted=true volume=1.00 paused=false session=none",
    });
    vi.useRealTimers();
  });

  it("changes a paused video's muting at a tap, which lets it play with sound after, and leaves it unmuted", () => {
    const changes: boolean[] = [];
    const video = {
      paused: true,
      /* eslint-disable @typescript-eslint/naming-convention -- HTMLMediaElement's own name, which this stands in for */
      get muted(): boolean {
        return changes.at(-1) ?? false;
      },
      set muted(isMuted: boolean) {
        changes.push(isMuted);
      },
      /* eslint-enable @typescript-eslint/naming-convention -- only muted */
    };
    readyForSoundAtTap(video);
    expect(changes).toEqual([true, false]);
  });

  it("leaves a playing video's sound alone at a tap", () => {
    const changes: boolean[] = [];
    const video = {
      paused: false,
      /* eslint-disable @typescript-eslint/naming-convention -- HTMLMediaElement's own name, which this stands in for */
      get muted(): boolean {
        return false;
      },
      set muted(isMuted: boolean) {
        changes.push(isMuted);
      },
      /* eslint-enable @typescript-eslint/naming-convention -- only muted */
    };
    readyForSoundAtTap(video);
    expect(changes).toEqual([]);
  });
});
