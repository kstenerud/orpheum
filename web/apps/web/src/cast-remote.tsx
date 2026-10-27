// SPDX-License-Identifier: MPL-2.0

import "./cast.css";

import {
  expectedGroupPositionMs,
  scrubPauseWindowMs,
  type GroupCommand,
  type GroupState,
  type TogetherConnection,
  type TogetherSnapshot,
} from "@orpheum/sdk";
import type { TFunction } from "i18next";
import { useEffect, useRef, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { tabStorage } from "./browser-storage.ts";
import { endCast, listMyCasts, moveCastOn, noteSending, type Cast } from "./cast.ts";
import { placeCode } from "./episode-place.ts";
import { seekStepMs } from "./player-commands.ts";
import { useServices, useToken } from "./services.tsx";
import { Loaded } from "./title.tsx";
import { clockText } from "./tv-controls.tsx";
import { useUpNext } from "./up-next.tsx";
import { useResource } from "./use-resource.ts";
import { monotonicMs, useTogether } from "./use-together.ts";

/** How often the remote's position moves on while the film plays, in ms: a clock's second. */
const tickEveryMs = 1000;

/**
 * The Casting page: this device's remote for the cast it sends (D337 §5; plans/cast-to-device.md
 * §5), found again from the server when the page is loaded again (§6.3); none says so.
 */
export function Casting(): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [casts] = useResource("my-casts", async () => await listMyCasts(api));
  return (
    <div className="cast-remote">
      <Loaded
        resource={casts}
        render={(list) => {
          const sent = list.find((each) => each.isSender);
          return sent === undefined ? <p>{t("cast.notCasting")}</p> : <Remote cast={sent} />;
        }}
      />
    </div>
  );
}

/**
 * The remote of one cast: it holds no player and loads no film (§5); what it shows is what the
 * receiver last said, moved on by the clock between, and its play, pause and seeks are the cast's
 * commands. When the cast ends, taken back, replaced, stopped or at the film's end, it says so and
 * offers to play here from where the cast stood (§9 Q5).
 */
export function Remote(props: { readonly cast: Cast }): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const token = useToken();
  const { connection, snapshot } = useTogether({
    credential: props.cast.membershipCredential,
    sessionToken: token,
    role: "panel",
  });
  const [cast, setCast] = useState(props.cast);
  useFollowsTheFilm(cast, snapshot, setCast);
  const positionMs = usePositionNow(snapshot);
  const [stoppedAtMs, setStoppedAtMs] = useState<number | undefined>(undefined);
  const endReason = snapshot?.status === "ended" ? snapshot.endReason : undefined;
  const isOver = endReason !== undefined || stoppedAtMs !== undefined;
  useEffect(() => {
    if (isOver) {
      noteSending(tabStorage(), false); // a film played here plays here again
    }
  }, [isOver]);
  if (isOver) {
    return (
      <CastOver
        cast={cast}
        reason={stoppedAtMs === undefined ? endReason : "stopped"}
        positionMs={stoppedAtMs ?? positionMs ?? 0}
      />
    );
  }
  const stop = (): void => {
    const atMs = positionMs ?? 0;
    endCast(api, cast.castId, "stopped").then(
      (ended) => {
        setStoppedAtMs(ended.positionMs);
      },
      () => {
        setStoppedAtMs(atMs);
      },
    );
  };
  return (
    <section className="cast-remote" aria-label={t("cast.remote")}>
      <h1>{cast.titleName === "" ? t("cast.aFilm") : cast.titleName}</h1>
      <p>{t("cast.playingOn", { name: cast.receiverName })}</p>
      <RemoteControls
        snapshot={snapshot}
        connection={connection}
        positionMs={positionMs}
        durationMs={cast.durationMs}
      />
      <RemoteUpNext cast={cast} onMovedOn={setCast} />
      <button type="button" onClick={stop}>
        {t("cast.stop")}
      </button>
    </section>
  );
}

/**
 * Follows the receiver to another film, the next episode its Up next played (plans/up-next.md): as
 * the file the cast's session says plays changes to another, the cast is read again for its title
 * and length.
 */
function useFollowsTheFilm(
  cast: Cast,
  snapshot: TogetherSnapshot | undefined,
  onCast: (cast: Cast) => void,
): void {
  const { api } = useServices();
  const playingFileId = snapshot?.group?.fileId;
  // The file the session said last, so only a change of it is followed: the remote's own Up next
  // moves the cast on before the receiver puts the next file on.
  const saidRef = useRef(playingFileId);
  useEffect(() => {
    const said = saidRef.current;
    saidRef.current = playingFileId;
    if (
      said === undefined ||
      playingFileId === said ||
      playingFileId === undefined ||
      playingFileId === 0 ||
      playingFileId === cast.fileId
    ) {
      return;
    }
    listMyCasts(api).then(
      (casts) => {
        const now = casts.find((each) => each.castId === cast.castId);
        if (now !== undefined && now.fileId !== cast.fileId) {
          onCast({
            ...cast,
            fileId: now.fileId,
            titleName: now.titleName,
            durationMs: now.durationMs,
          });
        }
      },
      () => undefined, // read again at the next change
    );
  }, [api, cast, playingFileId, onCast]);
}

/**
 * The remote's Up next: the cast's film's next episode, "Up next: S1E3 · ‹name›", which moves the
 * cast on to it, the receiver handed it (plans/up-next.md); nothing where none can play next.
 */
function RemoteUpNext(props: {
  readonly cast: Cast;
  readonly onMovedOn: (cast: Cast) => void;
}): JSX.Element | null {
  const { t } = useTranslation();
  const { api } = useServices();
  const { cast, onMovedOn } = props;
  const upNext = useUpNext(cast.fileId);
  if (upNext === undefined || upNext.episode.unavailable !== undefined) {
    return null;
  }
  const { episode } = upNext;
  const place = placeCode(t, upNext.ordering, { ...episode, isOnlyGroup: upNext.isOnlyGroup });
  return (
    <button
      type="button"
      className="cast-remote__up-next"
      onClick={() => {
        moveCastOn(api, cast.castId).then(
          (moved) => {
            onMovedOn({
              ...cast,
              fileId: moved.fileId,
              titleName: moved.titleName,
              durationMs: moved.durationMs,
            });
          },
          () => undefined, // the cast goes on as it was
        );
      }}
    >
      {t("upNext.offer", { place, name: episode.name ?? "" })}
    </button>
  );
}

/** Where the cast's film is now, moved on each second while it plays; undefined while unknown. */
function usePositionNow(snapshot: TogetherSnapshot | undefined): number | undefined {
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

/** The remote's play and pause, its steps back and forward, and its timeline. */
function RemoteControls(props: {
  readonly snapshot: TogetherSnapshot | undefined;
  readonly connection: TogetherConnection | undefined;
  readonly positionMs: number | undefined;
  readonly durationMs: number;
}): JSX.Element {
  const { t } = useTranslation();
  const { connection, positionMs, durationMs } = props;
  const state = props.snapshot?.group?.state;
  const isReady = props.snapshot?.status === "connected" && positionMs !== undefined;
  const send = (command: GroupCommand): void => {
    connection?.command(command);
  };
  const seekTo = (atMs: number): void => {
    const most = durationMs > 0 ? durationMs : Number.POSITIVE_INFINITY;
    send({ command: "seek", positionMs: Math.min(Math.max(0, atMs), most) });
  };
  return (
    <>
      <output>{stateText(t, state)}</output>
      <div className="cast-remote__buttons">
        <button
          type="button"
          disabled={!isReady}
          onClick={() => {
            send({ command: state === "Playing" ? "pause" : "play" });
          }}
        >
          {t(state === "Playing" ? "cast.pause" : "cast.play")}
        </button>
        <button
          type="button"
          disabled={!isReady}
          onClick={() => {
            seekTo((positionMs ?? 0) - seekStepMs);
          }}
        >
          {t("cast.back")}
        </button>
        <button
          type="button"
          disabled={!isReady}
          onClick={() => {
            seekTo((positionMs ?? 0) + seekStepMs);
          }}
        >
          {t("cast.forward")}
        </button>
      </div>
      <Timeline
        positionMs={positionMs ?? 0}
        durationMs={durationMs}
        isReady={isReady}
        onSeek={seekTo}
      />
    </>
  );
}

/**
 * The cast's timeline: where the film stands, and a drag along it, sent once the drag has rested
 * for a moment, as a leading player's drag is one seek (scrubPauseWindowMs).
 */
function Timeline(props: {
  readonly positionMs: number;
  readonly durationMs: number;
  readonly isReady: boolean;
  readonly onSeek: (atMs: number) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const [draftMs, setDraftMs] = useState<number | undefined>(undefined);
  const timerRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  useEffect(
    () => () => {
      clearTimeout(timerRef.current);
    },
    [],
  );
  const { onSeek } = props;
  const shownMs = draftMs ?? props.positionMs;
  return (
    <>
      {props.durationMs > 0 ? (
        <input
          type="range"
          className="cast-remote__timeline"
          aria-label={t("cast.timeline")}
          min={0}
          max={props.durationMs}
          step={1000}
          value={Math.min(shownMs, props.durationMs)}
          disabled={!props.isReady}
          onChange={(event) => {
            const atMs = Number(event.currentTarget.value);
            setDraftMs(atMs);
            clearTimeout(timerRef.current);
            timerRef.current = setTimeout(() => {
              setDraftMs(undefined);
              onSeek(atMs);
            }, scrubPauseWindowMs);
          }}
        />
      ) : null}
      <span className="cast-remote__time">
        {props.durationMs > 0
          ? `${clockText(shownMs)} / ${clockText(props.durationMs)}`
          : clockText(shownMs)}
      </span>
    </>
  );
}

/**
 * The remote once its cast is over: why, in the person's words, and Play here from where it stood
 * (§9 Q5), which the Play page starts from; at the film's end, only why.
 */
function CastOver(props: {
  readonly cast: Cast;
  readonly reason: string | undefined;
  readonly positionMs: number;
}): JSX.Element {
  const { t } = useTranslation();
  const { navigation } = useServices();
  const { cast, reason } = props;
  return (
    <section className="cast-remote" aria-label={t("cast.remote")}>
      <h1>{cast.titleName === "" ? t("cast.aFilm") : cast.titleName}</h1>
      <output>{overText(t, reason ?? "gone", cast.receiverName)}</output>
      {reason === "filmEnded" ? null : (
        <button
          type="button"
          onClick={() => {
            navigation.go({ screen: "play", fileId: cast.fileId, atMs: props.positionMs });
          }}
        >
          {t("cast.playHere", { at: clockText(props.positionMs) })}
        </button>
      )}
    </section>
  );
}

/** Why a cast is over, in the person's words; name is its receiver's. */
function overText(t: TFunction, reason: string, name: string): string {
  switch (reason) {
    case "takenBack":
      return t("cast.over.takenBack", { name });
    case "replaced":
      return t("cast.over.replaced", { name });
    case "stopped":
      return t("cast.over.stopped", { name });
    case "filmEnded":
      return t("cast.over.filmEnded", { name });
    default:
      return t("cast.over.gone", { name });
  }
}

/** Where the cast's film stands, in the person's words. */
function stateText(t: TFunction, state: GroupState | undefined): string {
  switch (state) {
    case "Playing":
      return t("cast.playing");
    case "Paused":
      return t("cast.paused");
    case "Idle":
    case "Waiting":
    case undefined:
      return t("cast.waiting");
  }
}
