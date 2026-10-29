// SPDX-License-Identifier: MPL-2.0

/**
 * When a following player moved for each change of its group, against when the change reached it,
 * and what held it meanwhile: the groupFollowed note the server logs. A cast's receiver played on
 * seven seconds after its remote paused it, and nothing said whether the pause came late or the
 * player held it (the owner, 2026-10-28).
 */

import type { GroupView } from "./together.ts";
import type { TogetherConnection } from "./together-connection.ts";
import type { ShownPage } from "./shown-page.ts";
import { haveMetadata, type FollowedMedia } from "./together-media.ts";
import type { GroupHeldBy, PlayerNote } from "./together-notes.ts";

/** The groupFollowed note. */
export type GroupFollowedNote = Extract<PlayerNote, { readonly kind: "groupFollowed" }>;

/**
 * What holds a following player from following its group now: its own command awaiting the group's
 * answer, its page hidden where hiding pauses its film (a handheld's; a computer's is given no page,
 * its hidden page playing on), or its film not loaded far enough to move; undefined for nothing.
 */
export function heldByOf(
  isAwaitingAnswer: boolean,
  page: ShownPage | undefined,
  media: Pick<FollowedMedia, "readyState">,
): Exclude<GroupHeldBy, "atOnce"> | undefined {
  if (isAwaitingAnswer) {
    return "awaitingAnswer";
  }
  if (page?.visibilityState === "hidden") {
    return "hidden";
  }
  return media.readyState < haveMetadata ? "loading" : undefined;
}

/**
 * Notes to connection event, a play of media its follower did not make, as no command came of it,
 * while its group, where it is, does not play; any other event, and any of a leader's, it leaves. The follower plays its
 * player only with its group (followGroup), and pauses it again, so the note says what played it:
 * the element's own autoplay as it loaded, or the page, a press or a media key. A Fire TV's cast
 * played, paused and played as it started, and nothing said what played it first (the owner's Marrowgate
 * Family, staging, 2026-10-28).
 */
export function notePlayedWhileHeld(played: {
  readonly connection: Pick<TogetherConnection, "note">;
  readonly event: string;
  readonly media: Pick<FollowedMedia, "autoplay" | "currentTime">;
  /** Where its group is, undefined while it plays another film or none. */
  readonly where: { readonly group: Pick<GroupView, "state"> } | undefined;
  /** Whether the player leads its session, which its group holds nowhere (D371 §1). */
  readonly isLeading?: boolean;
}): void {
  const { connection, event, media, where } = played;
  if (
    event !== "play" ||
    where === undefined ||
    where.group.state === "Playing" ||
    played.isLeading === true
  ) {
    return;
  }
  connection.note({
    kind: "playedWhileHeld",
    positionMs: media.currentTime * 1000,
    why: media.autoplay ? "autoplay" : "page",
  });
}

/** One player's changes of its group, heard and not yet followed, on its monotonic clock. */
export class FollowedChanges {
  /** The number of the change last heard; undefined before one. */
  #heardSeq: number | undefined;
  /** The change last heard and not yet followed, when it came and what first held it. */
  #waiting: { readonly seq: number; readonly heardAtMs: number; heldBy: GroupHeldBy } | undefined =
    undefined;

  /** Starts with seq, the change the group is at already, as heard: it is no change of its own. */
  constructor(seq: number | undefined) {
    this.#heardSeq = seq;
  }

  /** The group is at its change seq, at nowMs: a change not heard before waits to be followed. */
  heard(seq: number | undefined, nowMs: number): void {
    if (seq === undefined || seq === this.#heardSeq) {
      return;
    }
    this.#heardSeq = seq;
    this.#waiting = { seq, heardAtMs: nowMs, heldBy: "atOnce" };
  }

  /** The player could not follow the change waiting, for why; the first reason is the one told. */
  held(why: Exclude<GroupHeldBy, "atOnce">): void {
    if (this.#waiting?.heldBy === "atOnce") {
      this.#waiting.heldBy = why;
    }
  }

  /**
   * Whether the player may follow its group at nowMs, held by heldBy or nothing: once it may, the
   * change waiting is told to connection, as the player moves, so the note says when.
   */
  mayFollow(
    heldBy: Exclude<GroupHeldBy, "atOnce"> | undefined,
    nowMs: number,
    connection: Pick<TogetherConnection, "note">,
  ): boolean {
    if (heldBy !== undefined) {
      this.held(heldBy);
      return false;
    }
    const note = this.followed(nowMs);
    if (note !== undefined) {
      connection.note(note);
    }
    return true;
  }

  /** The player followed the group at nowMs: the note of the change waiting, once; else undefined. */
  followed(nowMs: number): GroupFollowedNote | undefined {
    const waiting = this.#waiting;
    if (waiting === undefined) {
      return undefined;
    }
    this.#waiting = undefined;
    return {
      kind: "groupFollowed",
      seq: waiting.seq,
      heldMs: Math.max(0, Math.round(nowMs - waiting.heardAtMs)),
      why: waiting.heldBy,
    };
  }
}
