// SPDX-License-Identifier: MPL-2.0

/**
 * A player following its group, wired to its video's events, its page and its session's connection:
 * the one entry to the follower (together-follow.ts), kept apart from it.
 */

import type { TogetherConnection } from "./together-connection.ts";
import { GroupFollower } from "./together-follow.ts";
import {
  checkDriftEveryMs,
  followedEvents,
  type FollowOptions,
} from "./together-follow-options.ts";
import { markFollowingAGroup } from "./following-a-group.ts";
import type { FollowedMedia } from "./together-media.ts";
import {
  chunkLoadedOf,
  noteQualitySwitched,
  noteSoundMoved,
  playerLoadedChunkEvent,
  playerMovedItselfEvent,
  playerMovedSoundEvent,
  playerSwitchedQualityEvent,
} from "./together-player-events.ts";
import { isFollowerOf } from "./together-roster.ts";
import { whenVisibilityChanges } from "./shown-page.ts";

/**
 * Makes media follow the group while it plays options.fileId (watch-together.md, Sync): paused at
 * the group's position while it is Paused, Waiting or Idle; playing while it is Playing, its drift
 * nudged away by the rate or sought across. It tells the group its state on every change, and
 * counts as buffering until it has once been where the group is, and after a stall or a seek until
 * it has refillAheadMs loaded. While its roster says this device leads the session, it moves media
 * by none of that: the leader plays as a device plays a film alone, carrying out once each change
 * another device made, and the group follows it (D371 §1, GroupLeader). A player's person's own
 * play, pause and seek become commands, as their rights allow; the follower's own do not. Returns
 * the stopping.
 */
export function followGroup(
  media: FollowedMedia,
  connection: TogetherConnection,
  options: FollowOptions,
): () => void {
  const follower = new GroupFollower(media, connection, options);
  const listeners = [
    ...followedEvents.map(
      (event) =>
        [
          event,
          () => {
            follower.heard(event);
          },
        ] as const,
    ),
    ["progress", follower.loaded] as const,
    [playerMovedItselfEvent, follower.playerMovedItself] as const,
    // An iPhone's own full-screen player, which pauses the video as it closes.
    ["webkitendfullscreen", follower.browserPaused] as const,
    [
      playerLoadedChunkEvent,
      (event: Event) => {
        const loaded = chunkLoadedOf(event);
        if (loaded !== undefined) {
          follower.chunkLoaded(loaded);
          connection.note({ kind: "chunkLoaded", ...loaded });
        }
      },
    ] as const,
    [
      playerSwitchedQualityEvent,
      (event: Event) => {
        noteQualitySwitched(connection)(event);
        follower.qualitySwitched(event);
      },
    ] as const,
    [playerMovedSoundEvent, noteSoundMoved(connection)] as const,
  ];
  for (const [event, listener] of listeners) {
    media.addEventListener(event, listener);
  }
  connection.setPlayer(follower.playerState);
  // A leader chooses its quality as a device alone does (D371 §1): only a follower keeps the film's
  // own picture for its group (D286 §2).
  const unmarkFollowing = markFollowingAGroup(() => isFollowerOf(connection.snapshot()));
  const unsubscribe = connection.subscribe(follower.connectionChanged);
  const checker = setInterval(follower.tick, checkDriftEveryMs);
  const stopHearingPage = whenVisibilityChanges(options.page, follower.visibilityChanged);
  follower.apply();
  connection.reportState();
  return () => {
    stopHearingPage();
    clearInterval(checker);
    follower.stop();
    unmarkFollowing();
    unsubscribe();
    connection.setPlayer(undefined);
    for (const [event, listener] of listeners) {
      media.removeEventListener(event, listener);
    }
  };
}
