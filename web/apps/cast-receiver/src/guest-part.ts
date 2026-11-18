// SPDX-License-Identifier: MPL-2.0

import { playChunked, type ChunkedPlayback } from "@orpheum/player";
import {
  createTogetherConnection,
  dataOrThrow,
  expectedGroupPositionMs,
  failureText,
  followGroup,
  type Api,
  type TogetherConnection,
  type TogetherEndReason,
} from "@orpheum/sdk";

import { receiverFormats } from "./device.ts";
import type { Reached } from "./reach.ts";

/**
 * A Chromecast taking the part of a Watch Together guest who joined by their link, signed in to
 * nothing (D518 §1): by a credential of the guest's own membership, which the trade of their grant
 * gave it, it is another device of theirs in the session, a follower of the group (D371 §4) with
 * their rights alone. It plays the group's film as the guest's own player does (D246 §3): asked for
 * at POST /together/playback and its chunks read by that credential, followed by the SDK's follower,
 * which leads nothing. It has no person's session, so it takes no cast, shows no up next and has no
 * Stop of its own; told the guest was removed or the session ended (D518 §2), it lets go of the
 * session and its owner sends it home, as at D389's end.
 */

/** What a Chromecast taking a guest's part needs. */
export interface GuestPartOptions {
  readonly api: Api;
  /** The credential of the guest's membership the trade answered. */
  readonly credential: string;
  readonly reached: Reached;
  readonly video: HTMLVideoElement;
  readonly note: (line: string) => void;
  /** Called once, as the session first welcomes it: the device it is there. */
  readonly onWelcomed: (deviceId: number) => void;
  /** Called once, as its part ends: the guest removed, the session over, or the credential refused. */
  readonly onEnded: (reason: TogetherEndReason) => void;
}

/** None of a person's own presses: the Chromecast has none, and only follows the group. */
const leadsNothing = { canPlayPause: false, canSeek: false, canNext: false } as const;

/** What the core decided the group's film plays as, for this device: its address on the ladder or not. */
interface Decision {
  readonly url: string;
  readonly onLadder?: { readonly url: string; readonly startAtBps: number };
}

/** Where the group is in its film now, as its session last told it, in milliseconds. */
function groupPositionMs(together: TogetherConnection): number {
  const snapshot = together.snapshot();
  const expected = expectedGroupPositionMs(snapshot, performance.now());
  return Math.max(0, expected ?? snapshot.group?.positionMs ?? 0);
}

/** The receiver as a device of a Watch Together guest's membership. */
export class GuestPart {
  private together: TogetherConnection | undefined;
  private playback: ChunkedPlayback | undefined;
  private stopFollowing: (() => void) | undefined;
  /** The group's film it plays, 0 for none. */
  private playingFileId = 0;
  private isWelcomed = false;

  constructor(private readonly options: GuestPartOptions) {}

  /** Takes the guest's part in their session; returns the letting go of it. */
  start(): () => void {
    const { credential, reached } = this.options;
    const together = createTogetherConnection({
      credential,
      client: "cast-receiver",
      openSocket: reached.openSocket,
      monotonicMs: () => performance.now(),
    });
    this.together = together;
    const unsubscribe = together.subscribe(() => {
      this.heard(together);
    });
    together.open();
    return () => {
      unsubscribe();
      this.leave();
    };
  }

  /** Follows what the session says: its film put on or taken off, or its part's end. */
  private heard(together: TogetherConnection): void {
    if (together !== this.together) {
      return;
    }
    const { note } = this.options;
    const snapshot = together.snapshot();
    if (snapshot.endReason !== undefined) {
      note(`the guest's part ended: ${snapshot.endReason}`);
      this.leave();
      this.options.onEnded(snapshot.endReason);
      return;
    }
    if (snapshot.status !== "connected") {
      return;
    }
    if (!this.isWelcomed) {
      this.isWelcomed = true;
      note("welcomed in the guest's session: following the group");
      this.options.onWelcomed(snapshot.deviceId ?? 0);
    }
    const fileId = snapshot.group?.fileId ?? 0;
    if (fileId !== this.playingFileId) {
      void this.play(together, fileId);
    }
  }

  /** Plays the group's film fileId, or nothing for 0, followed from where the group is. */
  private async play(together: TogetherConnection, fileId: number): Promise<void> {
    const { api, credential, note } = this.options;
    this.stopFilm();
    this.playingFileId = fileId;
    if (fileId === 0) {
      return;
    }
    try {
      const decision = dataOrThrow(
        await api.POST("/together/playback", {
          headers: { authorization: `Bearer ${credential}` },
          body: { playableFormats: receiverFormats() },
        }),
      );
      if (this.playingFileId === fileId && this.together === together) {
        this.startFilm(together, fileId, decision);
      }
    } catch (cause) {
      note(`could not start the group's file ${String(fileId)}: ${failureText(cause)}`);
    }
  }

  /** Starts the film the core decided on, from where the group is now, and follows the group. */
  private startFilm(together: TogetherConnection, fileId: number, decision: Decision): void {
    const { credential, reached, video, note } = this.options;
    const positionMs = groupPositionMs(together);
    const played = decision.onLadder?.url ?? decision.url;
    note(`playing the group's file ${String(fileId)} from ${(positionMs / 1000).toFixed(1)} s`);
    this.playback = playChunked(video, new URL(played, globalThis.location.origin).href, {
      onFailed: (failure) => {
        note(`failed: ${failure}`);
      },
      startAtBps: decision.onLadder?.startAtBps ?? 0,
      startPositionS: positionMs / 1000,
      monotonicMs: () => performance.now(),
      authorization: `Bearer ${credential}`,
      fetch: reached.fetch,
    });
    this.stopFollowing = followGroup(video, together, {
      fileId,
      leads: leadsNothing,
      monotonicMs: () => performance.now(),
    });
  }

  private stopFilm(): void {
    this.stopFollowing?.();
    this.stopFollowing = undefined;
    this.playback?.destroy();
    this.playback = undefined;
    this.playingFileId = 0;
  }

  /** Lets go of the session, then closes it: its close tells heard nothing once let go. */
  private leave(): void {
    const together = this.together;
    this.together = undefined;
    together?.close();
    this.stopFilm();
  }
}
