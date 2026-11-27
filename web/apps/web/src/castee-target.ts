// SPDX-License-Identifier: MPL-2.0

import { leaderOf, type PlaybackReport, type TogetherSnapshot } from "@orpheum/sdk";

import { useReceiverFilmSound } from "./cast-remote-choices.tsx";
import type { CasteeChoices, PlaybackTarget } from "./playback-target.ts";
import { useRemoteSource } from "./session-remote.tsx";
import type { useTogether } from "./use-together.ts";

/** The castee as a playback target, its choices the castee's own. */
export type CasteeTarget = PlaybackTarget & { readonly choices: CasteeChoices };

/** A session a caster's page works: what it says, and the connection that commands it. */
export type WorkedSession = Pick<ReturnType<typeof useTogether>, "snapshot" | "connection">;

/** What the castee's target is made of (useCasteeTarget). */
export interface CasteeOf {
  /**
   * The session the castee's film is commanded in and placed by: the cast's own, or, while its
   * person hosts Watch Together, that session, the castee its host's player (D364 §11).
   */
  readonly worked: WorkedSession;
  /** The cast's session, in which the castee says how it plays and takes its choices (D434). */
  readonly cast: WorkedSession;
  /** The film sent to the castee, which Report a problem names, and how long it lasts (0 unknown). */
  readonly sent: {
    readonly fileId: number;
    readonly durationMs: number;
  };
  /** Whether the castee is heard (useReceiverHeard, D550): unheard, nothing takes a press. */
  readonly isHeard: boolean;
}

/**
 * The castee as the playback target (PlaybackTarget, D555 §8): its film's place as the session says
 * it, moved on by the clock while it plays, a seek chosen here shown at once and sent once its moves
 * rest (useRemoteSource); play or pause sent through the session; its film's own level and mute on
 * it (useReceiverFilmSound, D548 §6), a Chromecast's its device's own volume (D557 §5), carried
 * through the cast's session alone (D563), undefined where it says none; and its choices, what it
 * says of its sound and its picture, sent to it as its own presses (CasteeChoices, D434).
 */
export function useCasteeTarget(castee: CasteeOf): CasteeTarget {
  const { worked, cast } = castee;
  const source = useRemoteSource({
    ...worked,
    durationMs: castee.sent.durationMs,
    isHeard: castee.isHeard,
  });
  const playback = casteePlaybackIn(cast.snapshot);
  const sound = useReceiverFilmSound(playback?.sound?.channels?.[0], cast.connection);
  return {
    ...source,
    kind: "castee",
    sound,
    choices: {
      kind: "castee",
      playback,
      connection: cast.connection,
      report: { fileId: castee.sent.fileId, snapshot: worked.snapshot },
    },
  };
}

/**
 * How the castee plays, as it says in the cast's session: there it leads, or, while its person
 * hosts Watch Together and it leads that session instead, it is the one device besides this remote,
 * a panel saying its playback alone (D364 §11).
 */
export function casteePlaybackIn(
  snapshot: TogetherSnapshot | undefined,
): PlaybackReport | undefined {
  if (snapshot === undefined) {
    return undefined;
  }
  const devices = snapshot.roster.flatMap((member) => member.devices);
  const castee =
    leaderOf(snapshot) ??
    devices.find((each) => each.id !== snapshot.deviceId && each.playback !== undefined);
  return castee?.playback;
}

/**
 * Whether the castee waits for a press on its own remote before it starts (D468 §2, D564 §2), as it
 * says in the cast's session (D555 §4): a device of this remote's member, the castee's, besides this
 * one, saying its cast is waitingForOK. It says running, or leaves, as the press starts it.
 */
export function isCasteeWaitingForOKIn(snapshot: TogetherSnapshot | undefined): boolean {
  if (snapshot === undefined) {
    return false;
  }
  const own = snapshot.roster.find((member) => member.id === snapshot.memberId);
  return (
    own?.devices.some(
      (each) => each.id !== snapshot.deviceId && each.castState === "waitingForOK",
    ) === true
  );
}

/**
 * Whether the session, its group said, holds no film yet: a cast's own session from its opening
 * until its castee puts the cast's film on (the core's together/cast.go openAlone, group.go
 * changeItem). Where its group plays nothing, where it stands, 0:00 paused, is no place of the film.
 */
export function holdsNoFilmIn(snapshot: TogetherSnapshot | undefined): boolean {
  return snapshot?.group?.fileId === 0;
}

/**
 * How the castee's device stands in the cast's session (D569 §1, §6), its roster's word for it: the
 * leader, or another of this remote's member's devices that plays. "absent" where the roster shows
 * none; "loading" where it stands so, which the server says of a device that has never yet said it
 * is ready to play; else "present". Read from the session, not this page's memory, a page opened
 * during the start says the same.
 */
export function casteeStandingIn(
  snapshot: TogetherSnapshot | undefined,
): "absent" | "loading" | "present" {
  if (snapshot === undefined) {
    return "absent";
  }
  const own = snapshot.roster.find((member) => member.id === snapshot.memberId);
  const castee =
    leaderOf(snapshot) ??
    own?.devices.find(
      (each) => each.id !== snapshot.deviceId && (each.role === "video" || each.role === "both"),
    );
  if (castee === undefined) {
    return "absent";
  }
  return castee.status === "loading" ? "loading" : "present";
}
