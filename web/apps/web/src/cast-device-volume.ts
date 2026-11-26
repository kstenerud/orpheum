// SPDX-License-Identifier: MPL-2.0

/**
 * The volume of the Google Cast device a cast from this page plays on: the device's own, which its
 * Cast session's sender sets (cast-sdk.ts's CastDeviceVolume), as Google's sender design checklist
 * has every sender do ("Sender apps must allow users to control volume using hardware buttons and/or
 * software sliders"). The Cast link (google-cast.ts's linkCastSession) tells it the session's, and
 * the castee's target (castee-target.ts) shows it on the pad's volume where the castee says no film
 * channel of its own, as a Chromecast does (the owner, 2026-11-26: "Anything it can do, I want").
 * A page loaded again has no Cast session linked, so no device volume, until it casts again.
 */

import { useSyncExternalStore } from "react";

import type { CastDeviceVolume } from "./cast-sdk.ts";
import type { PlayerSound } from "./playback-target.ts";

/** The linked cast's device volume, as the pad's volume shows and sets it. */
class Linked {
  /** The device's volume as last heard, undefined where its session said none. */
  sound: PlayerSound | undefined;
  /** Stops hearing its changes, once nothing reads it. */
  private unhearing: (() => void) | undefined;

  constructor(
    readonly castId: number,
    private readonly volume: CastDeviceVolume,
  ) {}

  /** Starts hearing the device's volume while something reads it. */
  hear(): void {
    if (this.unhearing !== undefined) {
      return;
    }
    try {
      this.unhearing = this.volume.onChange(() => {
        this.sound = soundOf(this.volume);
        tellChanged();
      });
    } catch {
      this.unhearing = () => undefined; // a session that tells no change: its volume read once
    }
    this.sound = soundOf(this.volume);
  }

  /** Stops hearing it, nothing reading it or its session over. */
  unhear(): void {
    this.unhearing?.();
    this.unhearing = undefined;
  }
}

let linked: Linked | undefined;
const listeners = new Set<() => void>();

function tellChanged(): void {
  for (const listener of listeners) {
    listener();
  }
}

/** The device's volume as volume says it now, set through it. */
function soundOf(volume: CastDeviceVolume): PlayerSound | undefined {
  let heard: ReturnType<CastDeviceVolume["read"]>;
  try {
    heard = volume.read();
  } catch {
    return undefined; // a session the SDK has let go of
  }
  return heard === undefined
    ? undefined
    : {
        volume: heard.level,
        isMuted: heard.isMuted,
        canSetVolume: true,
        setVolume: (level) => {
          volume.setLevel(level);
          if (level > 0 && heard.isMuted) {
            volume.setMuted(false); // a slider raised unmutes, as the film's own does
          }
        },
        setMuted: volume.setMuted,
      };
}

/** Says the cast castId plays on a Google Cast device whose volume is volume. */
export function tellCastDeviceVolume(castId: number, volume: CastDeviceVolume): void {
  forgetCastDeviceVolume(linked?.castId);
  linked = new Linked(castId, volume);
  if (listeners.size > 0) {
    linked.hear();
  }
  tellChanged();
}

/** Says the cast castId's Cast session is over: its device's volume is no longer the page's. */
export function forgetCastDeviceVolume(castId: number | undefined): void {
  if (linked === undefined || linked.castId !== castId) {
    return;
  }
  linked.unhear();
  linked = undefined;
  tellChanged();
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  linked?.hear();
  return () => {
    listeners.delete(listener);
    if (listeners.size === 0) {
      linked?.unhear();
    }
  };
}

/** The volume of the Google Cast device the cast castId plays on, undefined where none is linked. */
export function useCastDeviceVolume(castId: number | undefined): PlayerSound | undefined {
  const read = (): PlayerSound | undefined =>
    linked !== undefined && linked.castId === castId ? linked.sound : undefined;
  return useSyncExternalStore(subscribe, read, read);
}
