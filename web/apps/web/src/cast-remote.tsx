// SPDX-License-Identifier: MPL-2.0

import "./cast.css";

import {
  leaderOf,
  type CarriesSoundChoices,
  type CastSending,
  type PlaybackReport,
  type PlaybackSound,
  type TogetherSnapshot,
} from "@orpheum/sdk";
import { longestSoundOffsetMs } from "@orpheum/player";
import { useEffect, useRef, useState, type JSX, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import { audioTrackLabel } from "./audio-tracks.tsx";
import { tabStorage } from "./browser-storage.ts";
import { endCastRetrying, listMyCasts, moveCastOn } from "./cast.ts";
import { placeCode } from "./episode-place.ts";
import { useLive } from "./live.tsx";
import { homeTold } from "./notice.ts";
import { rateText } from "./rate-text.ts";
import type { ReopenAt } from "./reopen.ts";
import { useServices, useToken } from "./services.tsx";
import { SessionRemote, usePositionNow } from "./session-remote.tsx";
import { soundOffsetStepMs } from "./sound-delay.tsx";
import { useMembership } from "./together-membership.ts";
import { useUpNext } from "./up-next.tsx";
import { useResource } from "./use-resource.ts";
import { useTogether } from "./use-together.ts";

/**
 * Plays the page's film here again, from atMs, or where the person left it where undefined, paused
 * there where isPaused.
 */
export type PlaysHereFrom = (atMs: number | undefined, isPaused?: boolean) => void;

/**
 * The Play page's video area while its device casts (D384 §2, D364 §8): the page keeps its place, and
 * where its video was it shows the remote of the cast castId, "Playing on ‹device›" with its
 * controls, which work the receiver. Stop casting, here or in Cast's dialog, brings the film back to
 * the page at the receiver's position (onPlayHere), as control taken back on the receiver and the
 * receiver gone do, the page then playing it from where the receiver last stood, playing or paused as
 * it was (D385 §2, §3); a cast the server no longer knows sends the page home, saying so in a pop-up
 * (D386 §2). Where the cast plays another film than the page's by then, the film comes back on that
 * film's Play page, in this one's place.
 */
export function CastRemote(props: {
  readonly castId: number;
  /** The film of the Play page the remote shows on. */
  readonly fileId: number;
  readonly onPlayHere: PlaysHereFrom;
}): JSX.Element | null {
  const sent = useHeldCast(props.castId);
  if (sent === undefined) {
    return null;
  }
  return (
    <section className="cast-remote" aria-labelledby="cast-remote-heading">
      <Remote sent={sent} pageFileId={props.fileId} onPlayHere={props.onPlayHere} />
    </section>
  );
}

/**
 * The cast castId as the live connection says it, and after it ends, as it was last said, so its
 * end is said; undefined until it is said.
 */
function useHeldCast(castId: number): CastSending | undefined {
  const { sending } = useLive();
  const now = sending?.castId === castId ? sending : undefined;
  const [held, setHeld] = useState<CastSending | undefined>(now);
  if (now !== undefined && now !== held) {
    setHeld(now); // kept as it renders, so it is there once the cast ends
  }
  return now ?? held;
}

/**
 * Plays the film of the cast's last word here again, from atMs, paused there where isPaused: on the
 * page shown where it is the page's film, or nothing played on the receiver (onPlayHere); else on
 * that film's Play page, in this one's place.
 */
function usePlaysHere(
  sent: CastSending,
  pageFileId: number,
  onPlayHere: PlaysHereFrom,
): (atMs: number, isPaused?: boolean) => void {
  const { navigation } = useServices();
  return (atMs, isPaused = false) => {
    if (sent.fileId === 0) {
      onPlayHere(undefined);
    } else if (sent.fileId === pageFileId) {
      onPlayHere(atMs, isPaused);
    } else {
      navigation.replace({
        screen: "play",
        fileId: sent.fileId,
        atMs,
        shouldPlayHere: true,
        ...(isPaused ? { isPaused: true } : {}),
      });
    }
  };
}

/**
 * The remote of one cast (D364 §8): it holds no player and loads no film; what it shows is what the
 * live connection says plays on the receiver, and where, as the cast's session says, moved on by the
 * clock between. With a video playing: its title, the controls, Up next and Stop casting, the one
 * way to bring the film back here (D388); with none, "Ready on ‹Device›" and Stop casting. Stopped, its
 * control taken back on the receiver, or the receiver gone, the film plays here again at once (D384
 * §2, D385 §2, §3); lost, the server knowing it no more, the page goes home (D386 §2).
 */
function Remote(props: {
  readonly sent: CastSending;
  readonly pageFileId: number;
  readonly onPlayHere: PlaysHereFrom;
}): JSX.Element | null {
  const { t } = useTranslation();
  const { sent } = props;
  const { connection, snapshot, over, stop } = useCastSession(sent.castId);
  const hosted = useHostedPanel();
  const stoodRef = useWhereItStood(hosted?.snapshot ?? snapshot);
  const playHere = usePlaysHere(sent, props.pageFileId, props.onPlayHere);
  useTakesTheFilmOver(over, stoodRef, playHere);
  useGoesHomeAsTheCastIsLost(over);
  if (over !== undefined) {
    return null; // the film plays here again at once, or the page goes home
  }
  return (
    <>
      <RemoteOf sent={sent} cast={{ snapshot, connection }} hosted={hosted} />
      <button type="button" onClick={stop}>
        {t("cast.stop")}
      </button>
    </>
  );
}

/**
 * The ends of a cast after which the film plays here again at once: Stop casting, here or elsewhere
 * (D384 §2); control taken back on the receiver (D385 §2); the receiver gone, its connection lost
 * for ReceiverGoneAfter (D385 §3, D364 §6).
 */
const takenOverAt: ReadonlySet<string> = new Set(["stopped", "takenBack", "receiverGone"]);

/** Whether a cast that ended for reason has its film played here again at once (takenOverAt). */
function isTakenOver(reason: string): boolean {
  return takenOverAt.has(reason);
}

/**
 * Sends the page home, once, as its cast ends any way but takenOverAt: the server knows the cast no
 * more ("gone": not among the device's casts, or its session's credential refused), as after a
 * restart, saying so in a pop-up there (the shared notice, notice-note.tsx). The owner: "For extreme
 * edge cases like this, it's usually best to just go home" (D386 §2). A cast ended with the Watch
 * Together session it took a part in (sessionEnded, D389 §1) is a normal end, and goes home with no
 * word: the session's own end tells this page of it, as it takes it home too.
 */
function useGoesHomeAsTheCastIsLost(over: CastEnd | undefined): void {
  const { t } = useTranslation();
  const { navigation } = useServices();
  const isLost = over !== undefined && !isTakenOver(over.reason);
  const isSessionsEnd = over?.reason === "sessionEnded";
  const hasGoneRef = useRef(false);
  useEffect(() => {
    if (!isLost || hasGoneRef.current) {
      return;
    }
    hasGoneRef.current = true;
    if (isSessionsEnd) {
      navigation.replace({ screen: "home" });
      return;
    }
    homeTold(navigation, t("cast.over.lost"));
  }, [isLost, isSessionsEnd, navigation, t]);
}

/**
 * Plays the film here again, once, as its cast ends by one of takenOverAt: stopped, from where the
 * server said it stood as it ended, playing (D384 §2); its control taken back on the receiver, or the
 * receiver gone, from where the receiver last stood, playing or paused as it was (D385 §2, §3), this
 * device then the one playing and controlling it; where no device was heard leading, from where the
 * cast stood as it ended.
 */
function useTakesTheFilmOver(
  over: CastEnd | undefined,
  stoodRef: RefObject<ReopenAt | undefined>,
  playHere: (atMs: number, isPaused?: boolean) => void,
): void {
  const playHereRef = useRef(playHere);
  const hasTakenRef = useRef(false);
  useEffect(() => {
    playHereRef.current = playHere;
  });
  useEffect(() => {
    if (over === undefined || !isTakenOver(over.reason) || hasTakenRef.current) {
      return;
    }
    hasTakenRef.current = true;
    if (over.reason === "stopped") {
      playHereRef.current(over.positionMs); // playing, as Stop casting always has (D384 §2)
      return;
    }
    const from = stoodRef.current ?? over;
    playHereRef.current(from.positionMs, from.isPaused);
  }, [over, stoodRef]);
}

/**
 * Where the receiver last stood while a device led the session the remote works, the cast's or,
 * while this tab hosts Watch Together, that session (D364 §11): its film's position, moved on by the
 * clock, and whether it was paused, kept at each render while a device leads. Control taken back,
 * the cast ends as it stands. The receiver gone, it led no more once the server found its connection
 * lost, and where it stood then is its last position, never where the clock has moved a cast's
 * session on to since, which is not held for want of a player (D385 §3: "The caster doesn't need to
 * get its position exactly right").
 */
function useWhereItStood(snapshot: TogetherSnapshot | undefined): RefObject<ReopenAt | undefined> {
  const positionMs = usePositionNow(snapshot);
  const stoodRef = useRef<ReopenAt | undefined>(undefined);
  const isLed =
    snapshot !== undefined && snapshot.status !== "ended" && leaderOf(snapshot) !== undefined;
  const isPaused = snapshot?.group?.state === "Paused";
  useEffect(() => {
    if (isLed && positionMs !== undefined) {
      stoodRef.current = { positionMs, isPaused };
    }
  }, [isLed, positionMs, isPaused]);
  return stoodRef;
}

/** How a cast ended, as its remote says it: why, and where its video stood, and whether paused. */
interface CastEnd extends ReopenAt {
  readonly reason: string;
}

/**
 * The cast castId's session, joined as this device's remote by a credential read from the server for
 * it: what it says, how it ended once it has (stopped here, ended elsewhere, or no longer the
 * server's), and Stop casting, which ends it.
 */
function useCastSession(castId: number): {
  readonly connection: ReturnType<typeof useTogether>["connection"];
  readonly snapshot: ReturnType<typeof useTogether>["snapshot"];
  readonly over: CastEnd | undefined;
  readonly stop: () => void;
} {
  const { api } = useServices();
  const token = useToken();
  const [casts] = useResource(`my-casts/${String(castId)}`, async () => await listMyCasts(api));
  const own =
    casts.state === "loaded"
      ? casts.value.find((each) => each.castId === castId && each.isSender)
      : undefined;
  const { connection, snapshot } = useTogether({
    credential: own?.membershipCredential,
    sessionToken: token,
    role: "panel",
  });
  const positionMs = usePositionNow(snapshot);
  const [stoppedAtMs, setStoppedAtMs] = useState<number | undefined>(undefined);
  const endReason = snapshot?.status === "ended" ? snapshot.endReason : undefined;
  const isGone = casts.state === "loaded" && own === undefined;
  // Tried once more where the first try fails: a connection that has just dropped fails it, and the
  // next goes on its replacement (the owner, 2026-11-03: the phone played the film itself while the
  // laptop went on with the cast).
  const stop = (): void => {
    const atMs = positionMs ?? 0;
    endCastRetrying(api, castId, "stopped").then(
      (ended) => {
        setStoppedAtMs(ended?.positionMs ?? atMs);
      },
      () => {
        setStoppedAtMs(atMs);
      },
    );
  };
  return {
    connection,
    snapshot,
    over: endOf({
      stoppedAtMs,
      endReason,
      isGone,
      stood: { positionMs: positionMs ?? 0, isPaused: snapshot?.group?.state === "Paused" },
    }),
    stop,
  };
}

/**
 * How a cast ended, from Stop here, its session's end, or its absence, and where it stood (stood, as
 * its session says it now); undefined while it lasts.
 */
function endOf(cast: {
  readonly stoppedAtMs: number | undefined;
  readonly endReason: string | undefined;
  readonly isGone: boolean;
  readonly stood: ReopenAt;
}): CastEnd | undefined {
  const { stoppedAtMs, endReason, isGone, stood } = cast;
  if (stoppedAtMs !== undefined) {
    return { reason: "stopped", positionMs: stoppedAtMs, isPaused: false };
  }
  if (endReason === undefined && !isGone) {
    return undefined;
  }
  return { reason: endReason ?? "gone", ...stood };
}

/** A session as the remote works it: what it says, and its connection. */
type WorkedSession = Pick<ReturnType<typeof useTogether>, "snapshot" | "connection">;

/**
 * What the remote shows of what plays on the receiver: the receiver ready with nothing playing (§5),
 * or the film and its controls, which, while the person hosts Watch Together, work that session
 * (hosted), the receiver its host's player (D364 §11), the cast's own playing nothing.
 */
function RemoteOf(props: {
  readonly sent: CastSending;
  readonly cast: WorkedSession;
  readonly hosted: WorkedSession | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  const { sent, hosted } = props;
  if (sent.fileId === 0) {
    return <p id="cast-remote-heading">{t("cast.readyOn", { name: sent.receiverName })}</p>;
  }
  const worked = hosted ?? props.cast;
  // The receiver plays in the session it leads, the hosted one while its person hosts (D364 §11):
  // how it plays and sounds is said there, and its sound is chosen there.
  const playback = worked.snapshot === undefined ? undefined : leaderOf(worked.snapshot)?.playback;
  return (
    <>
      <h2 id="cast-remote-heading">{sent.itemName === "" ? t("cast.aFilm") : sent.itemName}</h2>
      <p>{t("cast.playingOn", { name: sent.receiverName })}</p>
      <ReceiverPlayback playback={playback} />
      <ReceiverSound sound={playback?.sound} connection={worked.connection} />
      <SessionRemote
        snapshot={worked.snapshot}
        connection={worked.connection}
        durationMs={sent.durationMs}
      />
      <RemoteUpNext sent={sent} />
    </>
  );
}

/**
 * The Watch Together session this tab hosts, joined as a panel, whose controls the cast's remote
 * works while it is on (D364 §11); undefined while this tab hosts none.
 */
function useHostedPanel(): WorkedSession | undefined {
  const token = useToken();
  const hosting = useMembership(tabStorage, "host");
  const hosted = useTogether({
    credential: hosting?.credential,
    sessionToken: token,
    role: "panel",
  });
  return hosting === undefined ? undefined : hosted;
}

/**
 * How the receiver plays the film, as its state comes back to the remote (D371 §4): the file as it
 * is or converted, the quality it plays and the link it chose it on. The owner: "The controller side
 * should get information about playback, such as the transcoding and bandwidth choices". Nothing
 * while the receiver has said nothing.
 */
function ReceiverPlayback(props: {
  readonly playback: PlaybackReport | undefined;
}): JSX.Element | null {
  const { t } = useTranslation();
  const { playback } = props;
  if (playback === undefined) {
    return null;
  }
  const { qualityKbps, linkKbps } = playback;
  return (
    <p className="cast-remote__playback">
      {[
        t(`cast.playback.${playback.delivery}`),
        qualityKbps === undefined
          ? undefined
          : t("cast.playback.quality", { rate: rateText(qualityKbps) }),
        linkKbps === undefined ? undefined : t("cast.playback.link", { rate: rateText(linkKbps) }),
      ]
        .filter((each) => each !== undefined)
        .join(" · ")}
    </p>
  );
}

/**
 * The receiver's sound, as it says it, and the remote's way to change it: the dialogue boost where
 * its film has a boosted mix, levelling, and the audio track where the file has more than one, each
 * choice sent to the receiver, which carries it out as its own press and says how it sounds after.
 * A Fire TV's own controls are hidden while it is controlled, so its sound could only be changed
 * after taking control back (the owner, VQ116's run). Nothing while the receiver has said nothing.
 */
function ReceiverSound(props: {
  readonly sound: PlaybackSound | undefined;
  readonly connection: CarriesSoundChoices | undefined;
}): JSX.Element | null {
  const { t, i18n } = useTranslation();
  const { sound, connection } = props;
  if (sound === undefined || connection === undefined) {
    return null;
  }
  const words = {
    unknownLanguage: t("play.audio.unknownLanguage"),
    commentary: t("play.audio.commentary"),
    described: t("play.audio.described"),
  };
  return (
    <div className="cast-remote__sound">
      {sound.isBoostOn === undefined ? null : (
        <RemoteSwitch
          key={`boost-${String(sound.isBoostOn)}`}
          label={t("play.boost")}
          isOn={sound.isBoostOn}
          onSwitched={(isOn) => {
            connection.chooseSound({ kind: "boost", isOn });
          }}
        />
      )}
      <RemoteSwitch
        key={`levelling-${String(sound.isLevellingOn)}`}
        label={t("play.level.switch")}
        isOn={sound.isLevellingOn}
        onSwitched={(isOn) => {
          connection.chooseSound({ kind: "levelling", isOn });
        }}
      />
      {sound.audioTracks.length < 2 ? null : (
        <label className="audio-tracks">
          {t("play.audio.label")}{" "}
          <select
            key={`audio-${String(sound.audioStreamId)}`}
            defaultValue={sound.audioStreamId ?? ""}
            onChange={(event) => {
              connection.chooseSound({
                kind: "audioTrack",
                streamId: Number(event.currentTarget.value),
              });
            }}
          >
            {sound.audioTracks.map((track) => (
              <option key={track.streamId} value={track.streamId}>
                {audioTrackLabel(track, i18n.language, words)}
              </option>
            ))}
          </select>
        </label>
      )}
      <RemoteSoundOffset ms={sound.soundOffsetMs} connection={connection} />
    </div>
  );
}

/**
 * The receiver's sound timing on the remote, for a TV that plays its sound late (the owner,
 * 2026-11-06: "Put a control in watch together and cast so that I can adjust the sound timing"):
 * as the receiver said it stands, each press moving it a step from there, which the receiver keeps
 * with its device as its own press. Nothing where the receiver offers no timing (undefined).
 */
function RemoteSoundOffset(props: {
  readonly ms: number | undefined;
  readonly connection: CarriesSoundChoices;
}): JSX.Element | null {
  const { t } = useTranslation();
  const { ms, connection } = props;
  if (ms === undefined) {
    return null;
  }
  const move = (byMs: number): void => {
    const next = Math.min(Math.max(ms + byMs, -longestSoundOffsetMs), longestSoundOffsetMs);
    connection.chooseSound({ kind: "soundOffset", ms: next });
  };
  let shown = t("cast.soundOffset.inStep");
  if (ms !== 0) {
    shown =
      ms > 0 ? t("cast.soundOffset.laterBy", { ms }) : t("cast.soundOffset.earlierBy", { ms: -ms });
  }
  return (
    <p className="play__sound-delay">
      <span>{shown}</span>{" "}
      <button
        type="button"
        onClick={() => {
          move(-soundOffsetStepMs);
        }}
      >
        {t("play.soundOffset.earlier", { ms: soundOffsetStepMs })}
      </button>{" "}
      <button
        type="button"
        onClick={() => {
          move(soundOffsetStepMs);
        }}
      >
        {t("play.soundOffset.later", { ms: soundOffsetStepMs })}
      </button>
    </p>
  );
}

/**
 * One of the receiver's switches on the remote: as the receiver said it stands, and as pressed here
 * until the receiver says again (its key changes with what it says).
 */
function RemoteSwitch(props: {
  readonly label: string;
  readonly isOn: boolean;
  readonly onSwitched: (isOn: boolean) => void;
}): JSX.Element {
  const [isOn, setIsOn] = useState(props.isOn);
  return (
    <label className="sound__switch">
      <input
        type="checkbox"
        checked={isOn}
        onChange={(event) => {
          const isNowOn = event.currentTarget.checked;
          setIsOn(isNowOn);
          props.onSwitched(isNowOn);
        }}
      />
      {props.label}
    </label>
  );
}

/**
 * The remote's Up next: the cast's film's next episode, "Up next: S1E3 · ‹name›", which moves the
 * cast on to it, the receiver handed it (plans/up-next.md); nothing where none can play next.
 */
function RemoteUpNext(props: { readonly sent: CastSending }): JSX.Element | null {
  const { t } = useTranslation();
  const { api } = useServices();
  const { sent } = props;
  const upNext = useUpNext(sent.fileId);
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
        moveCastOn(api, sent.castId).catch(() => undefined); // the cast goes on as it was
      }}
    >
      {t("upNext.offer", { place, name: episode.name ?? "" })}
    </button>
  );
}
