// SPDX-License-Identifier: MPL-2.0

import {
  expectedGroupPositionMs,
  scrubPauseWindowMs,
  type GroupCommand,
  type GroupState,
  type TogetherConnection,
  type TogetherSnapshot,
} from "@orpheum/sdk";
import type { TFunction } from "i18next";
import { useEffect, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { PlayerControls, type PlayerSource, type SequenceMoves } from "./player-controls.tsx";
import { useSeekAhead, type SeekAhead } from "./seek-ahead.ts";
import { monotonicMs } from "./use-together.ts";

/** How often a remote's position moves on while the film plays, in ms: a clock's second. */
const tickEveryMs = 1000;

/**
 * The remote of a session's film, a device with no player commanding another's (D88: a cast is Watch
 * Together with a group of one): a cast's receiver from its sender (D337 §5), and a Watch Together
 * group from its host's session page (D364 §15). One remote for both (rule 11). Its bar is the
 * player's own (D369 §1): play and pause, its steps back and forward and its timeline, a drag along
 * the timeline and steps in a row sent as one seek once they have rested (scrubPauseWindowMs), as a
 * leading player's drag is one seek, the timeline showing its place meanwhile and until the film
 * is there (seek-ahead.ts). Where it is given a previous (moves), ⏮ leads the bar, as the player's
 * does (a cast's remote, D440 §5).
 */
export function SessionRemote(props: {
  readonly snapshot: TogetherSnapshot | undefined;
  readonly connection: TogetherConnection | undefined;
  /** How long the film lasts, in ms; 0 while not known. */
  readonly durationMs: number;
  /** What ⏮ plays, where the remote offers it. */
  readonly moves?: SequenceMoves | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  const positionMs = usePositionNow(props.snapshot);
  const { connection, durationMs } = props;
  const status = props.snapshot?.status;
  const ahead = useSeekAhead((atMs) => {
    const most = durationMs > 0 ? durationMs : Number.POSITIVE_INFINITY;
    connection?.command({ command: "seek", positionMs: Math.min(Math.max(0, atMs), most) });
  });
  return (
    <>
      <output>
        {status === "connecting" || status === "reconnecting"
          ? t(`together.${status}`) // its buttons wait: a press now would be dropped unsaid
          : stateText(t, props.snapshot?.group?.state)}
      </output>
      <PlayerControls
        source={remoteSource({ ...props, positionMs, ahead })}
        controls={["previous", "play", "steps", "timeline"]}
        moves={props.moves}
      />
    </>
  );
}

/** Where the session's film is now, moved on each second while it plays; undefined while unknown. */
export function usePositionNow(snapshot: TogetherSnapshot | undefined): number | undefined {
  const [nowMs, setNowMs] = useState(monotonicMs);
  const isPlaying = snapshot?.group?.state === "Playing";
  useEffect(() => {
    if (!isPlaying) {
      return undefined;
    }
    const timer = setInterval(() => {
      setNowMs(monotonicMs());
    }, tickEveryMs);
    return () => {
      clearInterval(timer);
    };
  }, [isPlaying]);
  if (snapshot?.group === undefined || snapshot.group.state === "Idle") {
    return undefined;
  }
  return expectedGroupPositionMs(snapshot, Math.max(nowMs, snapshot.groupReceivedAtMs));
}

/**
 * A session's film as the remote's bar plays it: where it stands, as its player last said and the
 * clock has moved it on, or where a seek being chosen sends it (ahead); its commands sent over the
 * session, a seek once its moves rest.
 */
function remoteSource(remote: {
  readonly snapshot: TogetherSnapshot | undefined;
  readonly connection: TogetherConnection | undefined;
  readonly positionMs: number | undefined;
  readonly durationMs: number;
  readonly ahead: SeekAhead;
}): PlayerSource {
  const { connection, positionMs, durationMs, ahead } = remote;
  const isPlaying = remote.snapshot?.group?.state === "Playing";
  const send = (command: GroupCommand): void => {
    connection?.command(command);
  };
  return {
    atMs: ahead.shownMs(positionMs ?? 0),
    durationMs,
    isPaused: !isPlaying,
    isReady: remote.snapshot?.status === "connected" && positionMs !== undefined,
    playOrPause: () => {
      ahead.land();
      send({ command: isPlaying ? "pause" : "play" });
    },
    seekTo: (atMs) => {
      ahead.aim(atMs, scrubPauseWindowMs);
    },
  };
}

/** Where the session's film stands, in the person's words. */
function stateText(t: TFunction, state: GroupState | undefined): string {
  switch (state) {
    case "Playing":
      return t("remote.playing");
    case "Paused":
      return t("remote.paused");
    case "Idle":
    case "Waiting":
    case undefined:
      return t("remote.waiting");
  }
}
