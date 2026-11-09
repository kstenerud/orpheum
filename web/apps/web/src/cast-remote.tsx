// SPDX-License-Identifier: MPL-2.0

import "./cast.css";

import {
  leaderOf,
  type CastSending,
  type PlaybackReport,
  type TogetherSnapshot,
} from "@orpheum/sdk";
import { useEffect, useRef, useState, type JSX, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import { tabStorage } from "./browser-storage.ts";
import { noOverlay, RemoteMenus, useRemotePrevious } from "./cast-remote-choices.tsx";
import { listMyCasts, moveCastOn } from "./cast.ts";
import { placeCode } from "./episode-place.ts";
import { useLeavesFullScreen } from "./film-fullscreen.ts";
import type { PlayerControl, PlayerSource } from "./player-controls.tsx";
import { OverlayRowsOf, SignsOverTheFilm } from "./player-overlay.tsx";
import { useLatest } from "./latest.ts";
import { useLive } from "./live.tsx";
import { homeTold } from "./notice.ts";
import type { ReopenAt } from "./reopen.ts";
import { useServices, useToken } from "./services.tsx";
import { usePositionNow, useRemoteSource } from "./session-remote.tsx";
import { useMembership } from "./together-membership.ts";
import { TogetherBehindTheRemote } from "./together-on-the-remote.tsx";
import { offerState, useUpNext } from "./up-next.tsx";
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
 * controls, which work the receiver. Stop casting, in Cast's dialog or on the receiver, brings the
 * film back to the page (onPlayHere), as the receiver gone does, from where the receiver last stood,
 * playing or paused as it was, and with nothing on there sends the page home (D446 §2, §5); a cast the server no longer knows sends the page home, saying so in a pop-up
 * (D386 §2). Where the cast plays another film than the page's by then, the film comes back on that
 * film's Play page, in this one's place. A cast started from full screen leaves it: the remote is a
 * page, the control centre while casting, not a film (D440 §5), laid out as the full-screen overlay
 * is, with its set of controls (D444).
 */
export function CastRemote(props: {
  readonly castId: number;
  /** The film of the Play page the remote shows on. */
  readonly fileId: number;
  readonly onPlayHere: PlaysHereFrom;
}): JSX.Element | null {
  useLeavesFullScreen();
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
 * page shown where it is the page's film (onPlayHere); else on that film's Play page, in this one's
 * place. Nothing played on the receiver, the page goes home (D446 §2).
 */
function usePlaysHere(
  sent: CastSending,
  pageFileId: number,
  onPlayHere: PlaysHereFrom,
): (atMs: number, isPaused?: boolean) => void {
  const { navigation } = useServices();
  return (atMs, isPaused = false) => {
    if (sent.fileId === 0) {
      navigation.replace({ screen: "home" }); // nothing was on: nothing to take on (D446 §2)
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
 * clock between. With a video playing: the full-screen overlay's rows, its title as a sign, the
 * controls and Up next at the episode's end; with none, "Ready on ‹Device›" (D444). Stop casting, the
 * one way to bring the film back here (D388), is in the Cast sign's dialog, as everywhere (D444).
 * Stopped here or on the receiver, or the receiver gone, the film plays here again at once, as the
 * receiver left it, or the page goes home where nothing was on (D446 §2); lost, the server knowing it no more, the page goes home (D386 §2).
 */
function Remote(props: {
  readonly sent: CastSending;
  readonly pageFileId: number;
  readonly onPlayHere: PlaysHereFrom;
}): JSX.Element | null {
  const { sent } = props;
  const { connection, snapshot, over } = useCastSession(sent.castId);
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
      <TogetherBehindTheRemote hostedSnapshot={hosted?.snapshot} />
    </>
  );
}

/**
 * The ends of a cast after which the film plays here again at once, as the receiver left it (D446
 * §2): Stop casting, in Cast's dialog (D444); Stop casting here, on the receiver (takenBack, D446 §3);
 * the receiver gone, its connection lost for ReceiverGoneAfter (D446 §5, D364 §6).
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
 * Plays the film here again, once, as its cast ends by one of takenOverAt, stopped here, stopped on
 * the receiver or the receiver gone: from where the receiver last stood, playing or paused as it was,
 * this device then the one playing and controlling it (D446 §2, §5); where no device was heard
 * leading, from where the cast stood as it ended.
 */
function useTakesTheFilmOver(
  over: CastEnd | undefined,
  stoodRef: RefObject<ReopenAt | undefined>,
  playHere: (atMs: number, isPaused?: boolean) => void,
): void {
  const playHereRef = useLatest(playHere);
  const hasTakenRef = useRef(false);
  useEffect(() => {
    if (over === undefined || !isTakenOver(over.reason) || hasTakenRef.current) {
      return;
    }
    hasTakenRef.current = true;
    const from = stoodRef.current ?? over;
    playHereRef.current(from.positionMs, from.isPaused);
  }, [over, playHereRef, stoodRef]);
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
  const isPaused = snapshot?.group?.state === "paused";
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
 * it: what it says, and how it ended once it has (ended, as by Stop casting in Cast's dialog, or no
 * longer the server's).
 */
function useCastSession(castId: number): {
  readonly connection: ReturnType<typeof useTogether>["connection"];
  readonly snapshot: ReturnType<typeof useTogether>["snapshot"];
  readonly over: CastEnd | undefined;
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
  const endReason = snapshot?.status === "ended" ? snapshot.endReason : undefined;
  const isGone = casts.state === "loaded" && own === undefined;
  return {
    connection,
    snapshot,
    over: endOf({
      endReason,
      isGone,
      stood: { positionMs: positionMs ?? 0, isPaused: snapshot?.group?.state === "paused" },
    }),
  };
}

/**
 * How a cast ended, from its session's end or its absence, and where it stood (stood, as its session
 * says it now); undefined while it lasts.
 */
function endOf(cast: {
  readonly endReason: string | undefined;
  readonly isGone: boolean;
  readonly stood: ReopenAt;
}): CastEnd | undefined {
  const { endReason, isGone, stood } = cast;
  if (endReason === undefined && !isGone) {
    return undefined;
  }
  return { reason: endReason ?? "gone", ...stood };
}

/** A session as the remote works it: what it says, and its connection. */
type WorkedSession = Pick<ReturnType<typeof useTogether>, "snapshot" | "connection">;

/**
 * How the receiver plays, as it says in the cast's session: there it leads, or, while its person
 * hosts Watch Together and it leads that session instead, it is the one device besides this remote,
 * a panel saying its playback alone (D364 §11).
 */
function receiverPlaybackIn(snapshot: TogetherSnapshot | undefined): PlaybackReport | undefined {
  if (snapshot === undefined) {
    return undefined;
  }
  const devices = snapshot.roster.flatMap((member) => member.devices);
  const receiver =
    leaderOf(snapshot) ??
    devices.find((each) => each.id !== snapshot.deviceId && each.playback !== undefined);
  return receiver?.playback;
}

/**
 * What the remote shows of what plays on the receiver, laid out as the full-screen overlay is
 * (D444, amending D440 §5): the receiver ready with nothing playing, "Ready on ‹TV›" alone (§5); or
 * the overlay's rows, its top the sign "‹film› · Playing on ‹TV›", Up next only at the episode's end
 * as over the film (upNextLeadMs) and the signs over the film, the Cast sign's dialog holding Stop
 * casting; the timeline; the bar with ⏮ (useRemotePrevious) and the overlay's menus over the
 * receiver (RemoteMenus). The bar and timeline work, while the person hosts Watch Together, that
 * session (hosted), the receiver its host's player (D364 §11), the cast's own playing nothing.
 */
function RemoteOf(props: {
  readonly sent: CastSending;
  readonly cast: WorkedSession;
  readonly hosted: WorkedSession | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  const { sent } = props;
  if (sent.fileId === 0) {
    return (
      <div className="player-overlay cast-remote__overlay" data-shown="full">
        <div className="player-overlay__row player-overlay__top">
          <p id="cast-remote-heading" className="cast-remote__sign">
            {t("cast.readyOn", { name: sent.receiverName })}
          </p>
          <SignsOverTheFilm onOpenChange={noOverlay} />
        </div>
      </div>
    );
  }
  return <RemotePlaying {...props} />;
}

/** The remote's rows while a film plays on the receiver (RemoteOf). */
function RemotePlaying(props: {
  readonly sent: CastSending;
  readonly cast: WorkedSession;
  readonly hosted: WorkedSession | undefined;
}): JSX.Element {
  const { sent, hosted } = props;
  const worked = hosted ?? props.cast;
  const source = useRemoteSource({ ...worked, durationMs: sent.durationMs });
  const moves = useRemotePrevious(sent);
  // How the receiver plays and sounds is said in the cast's session, and its sound chosen there,
  // whatever else it plays in (the owner, 2026-11-06: "It shouldn't be watch together's business at
  // all which TV has what offset or playback characteristics").
  const playback = receiverPlaybackIn(props.cast.snapshot);
  return (
    <div className="player-overlay cast-remote__overlay" data-shown="full">
      <OverlayRowsOf
        top={<RemoteTop sent={sent} source={source} />}
        source={source}
        controls={remoteControls}
        moves={moves}
        menus={<RemoteMenus playback={playback} connection={props.cast.connection} />}
        isOverTheFilm={false}
      />
    </div>
  );
}

/** The remote's bar: the overlay's, ⏮ leading it where there is a previous (D440 §5). */
const remoteControls: readonly PlayerControl[] = ["previous", "steps", "play"];

/**
 * The remote's top row, as the overlay's over the film: the film's sign, Up next at its end, and
 * the signs over the film (the Cast sign, Watch Together's).
 */
function RemoteTop(props: {
  readonly sent: CastSending;
  readonly source: PlayerSource;
}): JSX.Element {
  const { t } = useTranslation();
  const { sent, source } = props;
  const isAtTheEnd = offerState(
    { atMs: source.atMs, durationMs: source.durationMs, hasEnded: false },
    { isDismissed: false, isCancelled: false },
    false,
  ).isNearTheEnd;
  return (
    <>
      <div className="cast-remote__sign">
        <h2 id="cast-remote-heading">{sent.itemName === "" ? t("cast.aFilm") : sent.itemName}</h2>
        <p>{t("cast.playingOn", { name: sent.receiverName })}</p>
      </div>
      {isAtTheEnd ? (
        <div className="convenience-area">
          <RemoteUpNext sent={sent} />
        </div>
      ) : null}
      <SignsOverTheFilm onOpenChange={noOverlay} />
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
