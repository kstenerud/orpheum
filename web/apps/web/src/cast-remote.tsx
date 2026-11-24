// SPDX-License-Identifier: MPL-2.0

import "./cast.css";
import "./play-words.ts";
import "./cast-words.ts";

import {
  leaderOf,
  type components,
  type CastSending,
  type PlaybackReport,
  type TogetherSnapshot,
} from "@orpheum/sdk";
import {
  useEffect,
  useMemo,
  useRef,
  useState,
  type JSX,
  type ReactNode,
  type RefObject,
} from "react";
import { useTranslation } from "react-i18next";

import { tabStorage } from "./browser-storage.ts";
import { RemoteMenus, useReceiverFilmSound, useRemotePrevious } from "./cast-remote-choices.tsx";
import { RemotePad, remoteSoundMovesOf } from "./cast-remote-pad.tsx";
import { listMyCasts, moveCastOn, myCastsKey, takeEndAnswerOf, type Cast } from "./cast.ts";
import { useLeavesFullScreen } from "./film-fullscreen.ts";
import { useShowsARemote } from "./hidden-need.ts";
import type { PlayerSource, SequenceMoves } from "./player-controls.tsx";
import { useVolumeShown } from "./over-the-picture.tsx";
import { ClockTimes, shownAtMsOf, Timeline } from "./timeline.tsx";
import { EndsAt, leftMsOf } from "./convenience-area.tsx";
import { readTitle, titleKey } from "./item-detail.ts";
import { RemoteOffer } from "./cast-remote-offer.tsx";
import { OverlayUp, PlayUpwardContext, usePlayUpward } from "./play-up.tsx";
import { useLatest } from "./latest.ts";
import { useLive } from "./live.tsx";
import { homeTold } from "./notice.ts";
import type { ReopenAt } from "./reopen.ts";
import { useServices, useToken } from "./services.tsx";
import { titleCrumb } from "./upward.ts";
import { usePositionNow, useRemoteSource } from "./session-remote.tsx";
import { useMembership } from "./together-membership.ts";
import { TogetherBehindTheRemote } from "./together-on-the-remote.tsx";
import { useAroundFile, type AroundFile } from "./around-file.ts";
import { useLeavesForThePageAbove } from "./up-next.tsx";
import { useResource, type Resource } from "./use-resource.ts";
import { useTogether } from "./use-together.ts";

/** A title's pictures, as its detail names them. */
type ArtBody = components["schemas"]["ArtBody"];
type TitleBody = components["schemas"]["ItemBody"];
type PlaysBody = components["schemas"]["PlaysBody"];

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
 * film's Play page, in this one's place. A cast started from the browser's full screen leaves it,
 * and the remote fills the window under the top bar instead: the player's own view, a still where
 * the film would be, its overlay's rows over it (RemoteOf, D444).
 */
export function CastRemote(props: {
  readonly castId: number;
  /** The film of the Play page the remote shows on. */
  readonly fileId: number;
  readonly onPlayHere: PlaysHereFrom;
}): JSX.Element | null {
  useLeavesFullScreen();
  // Its page hidden, it keeps connecting as when shown, so the receiver's controls stay at hand (D482 §3).
  useShowsARemote();
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
 * Plays the film of the cast's last word here again, from atMs, paused there where isPaused, or from
 * its own start where atMs is undefined, no place being known (D549 §3): on the
 * page shown where it is the page's film (onPlayHere); else on that film's Play page, in this one's
 * place. Nothing played on the receiver, the page goes home (D446 §2).
 */
function usePlaysHere(
  sent: CastSending,
  pageFileId: number,
  onPlayHere: PlaysHereFrom,
): (atMs: number | undefined, isPaused?: boolean) => void {
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
        ...(atMs === undefined ? {} : { atMs }), // none known: the film's own start, never 0 (D549 §3)
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
 * The cast's film ended with nothing next, the page goes to the page above it, or above the episode
 * the cast had moved on to, the cast going on (useLeavesAtTheCastsEnd).
 */
function Remote(props: {
  readonly sent: CastSending;
  readonly pageFileId: number;
  readonly onPlayHere: PlaysHereFrom;
}): JSX.Element | null {
  const { sent } = props;
  const { connection, snapshot, over, casts } = useCastSession(sent.castId);
  const hosted = useHostedPanel();
  const stoodRef = useWhereItStood(hosted?.snapshot ?? snapshot);
  const playHere = usePlaysHere(sent, props.pageFileId, props.onPlayHere);
  useTakesTheFilmOver(sent.castId, over, { stoodRef, playHere });
  useGoesHomeAsTheCastIsLost(over);
  const episodes = useEpisodesMovedTo();
  useLeavesAtTheCastsEnd(sent.fileId, props.pageFileId, episodes.of);
  if (over !== undefined) {
    return null; // the film plays here again at once, or the page goes home
  }
  return (
    <>
      <RemoteOf
        sent={sent}
        cast={{ snapshot, connection }}
        casts={casts}
        hosted={hosted}
        onAround={episodes.note}
      />
      <TogetherBehindTheRemote hostedSnapshot={hosted?.snapshot} />
    </>
  );
}

/**
 * Leaves the remote's Play page for the page above it, as a local play leaves at its film's end with
 * nothing next (LeavesAtTheEnd; the owner, 2026-11-17), once the cast that played a film, fileId,
 * plays nothing: its receiver, a browser's or a Chromecast's, has it play nothing at its film's end
 * with nothing after it, waiting ready (D364 §5). The cast goes on, connected, for the next video
 * played from that page; one that played nothing from the start leaves nothing. Where the cast had
 * moved on from the page's film, pageFileId, to another episode, the page leaves for that episode's
 * page, as a local play moved on by Up next does, where episodeOf knows it.
 */
function useLeavesAtTheCastsEnd(
  fileId: number,
  pageFileId: number,
  episodeOf: (fileId: number) => number | undefined,
): void {
  const { navigation } = useServices();
  const leaveRef = useLatest(useLeavesForThePageAbove());
  const episodeOfRef = useLatest(episodeOf);
  const playedRef = useRef(fileId);
  useEffect(() => {
    const played = playedRef.current;
    playedRef.current = fileId;
    if (played === 0 || fileId !== 0) {
      return;
    }
    const episodeId = played === pageFileId ? undefined : episodeOfRef.current(played);
    if (episodeId === undefined) {
      leaveRef.current?.();
      return;
    }
    const page = titleCrumb(episodeId, "");
    navigation.up(page.to, page.isIt);
  }, [fileId, pageFileId, leaveRef, episodeOfRef, navigation]);
}

/**
 * The episodes the remote has read the cast's moves to and back to, by their files: what plays after
 * and before each film the cast plays names its episode (useAroundFile), noted as it is read.
 */
function useEpisodesMovedTo(): {
  readonly note: (around: AroundFile) => void;
  readonly of: (fileId: number) => number | undefined;
} {
  const [episodes] = useState(() => new Map<number, number>());
  return useMemo(
    () => ({
      note: (around) => {
        for (const episode of [around.upNext?.episode, around.previous]) {
          if (episode !== undefined) {
            episodes.set(episode.fileId, episode.itemId);
          }
        }
      },
      of: (fileId) => episodes.get(fileId),
    }),
    [episodes],
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
 * this device then the one playing and controlling it (D446 §2, §5); where this tab's own Stop was
 * answered, from where the server said it stood, before that; where no device was heard leading,
 * from where the cast stood as it ended, and where that is not known either, from the film's own
 * start (D549).
 */
function useTakesTheFilmOver(
  castId: number,
  over: CastEnd | undefined,
  where: {
    readonly stoodRef: RefObject<ReopenAt | undefined>;
    readonly playHere: (atMs: number | undefined, isPaused?: boolean) => void;
  },
): void {
  const { stoodRef, playHere } = where;
  const playHereRef = useLatest(playHere);
  const hasTakenRef = useRef(false);
  useEffect(() => {
    void takeEndAnswerOf(castId); // an answer to a Stop asked before this remote was shown is not its own
  }, [castId]);
  useEffect(() => {
    if (over === undefined || !isTakenOver(over.reason) || hasTakenRef.current) {
      return;
    }
    hasTakenRef.current = true;
    // The first known (D549 §2): the server's answer to this tab's own Stop, where the receiver last
    // stood, the ended session's own place; none known, the film's own start, never 0 (§3).
    const answered = takeEndAnswerOf(castId) ?? Promise.resolve(undefined);
    void answered.then((ended) => {
      const from = ended ?? stoodRef.current ?? over.stood;
      playHereRef.current(from?.positionMs, from?.isPaused);
    });
  }, [castId, over, playHereRef, stoodRef]);
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

/**
 * How a cast ended, as its remote says it: why, and where its video stood, and whether paused, where
 * its session said (stood).
 */
interface CastEnd {
  readonly reason: string;
  readonly stood: ReopenAt | undefined;
}

/**
 * The cast castId's session, joined as this device's remote by a credential read from the server for
 * it: what it says, and how it ended once it has (ended, as by Stop casting in Cast's dialog, or no
 * longer the server's); and this device's casts as read for it, which name its title (useCastTitle).
 */
function useCastSession(castId: number): {
  readonly connection: ReturnType<typeof useTogether>["connection"];
  readonly snapshot: ReturnType<typeof useTogether>["snapshot"];
  readonly over: CastEnd | undefined;
  readonly casts: Resource<readonly Cast[]>;
} {
  const { api } = useServices();
  const token = useToken();
  // The cast started here handed its answer; only a page loaded anew asks (D478).
  const [casts] = useResource(myCastsKey(castId), async () => await listMyCasts(api));
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
    casts,
    over: endOf({
      endReason,
      isGone,
      stood:
        positionMs === undefined
          ? undefined
          : { positionMs, isPaused: snapshot?.group?.state === "paused" },
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
  readonly stood: ReopenAt | undefined;
}): CastEnd | undefined {
  const { endReason, isGone, stood } = cast;
  if (endReason === undefined && !isGone) {
    return undefined;
  }
  return { reason: endReason ?? "gone", stood };
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
 * Whether the receiver is heard (D550 §2): its device among the person's connected devices (the live
 * connection's, told once it has said any), and, once a device was heard leading the cast's session,
 * one still leading it. The remote's own connection stands with a television switched off, so it
 * alone does not say. A receiver still loading, never heard leading, is not gone.
 */
function useReceiverHeard(sent: CastSending, snapshot: TogetherSnapshot | undefined): boolean {
  const { connectedDeviceIds } = useLive();
  const isLeading = snapshot !== undefined && leaderOf(snapshot) !== undefined;
  const [hasLed, setHasLed] = useState(false);
  if (isLeading && !hasLed) {
    setHasLed(true); // kept as it renders, so the render that loses the leader already knows
  }
  const isDeviceGone =
    connectedDeviceIds.length > 0 && !connectedDeviceIds.includes(sent.receiverDeviceId);
  const isLeaderGone = hasLed && !isLeading;
  return !isDeviceGone && !isLeaderGone;
}

/**
 * What the remote shows of what plays on the receiver: its own page, laid out for a hand holding a
 * phone (D548 §1, amending D444's full-screen overlay), its stage filling the window under the top
 * bar, kept so a person can go elsewhere (the owner, 2026-11-23), its Cast the dialog, Stop casting
 * in it. The receiver ready with nothing playing, the bar alone, "Ready on ‹TV›"; a film playing
 * there, the remote's column (RemotePlaying). The controls work, while the person hosts Watch
 * Together, that session (hosted), the receiver its host's player (D364 §11), the cast's own playing
 * nothing.
 */
function RemoteOf(props: {
  readonly sent: CastSending;
  readonly cast: WorkedSession;
  /** This device's casts as read for the cast's session (useCastSession). */
  readonly casts: Resource<readonly Cast[]>;
  readonly hosted: WorkedSession | undefined;
  /** Told what plays around each film the cast plays, as it is read. */
  readonly onAround: (around: AroundFile) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { sent } = props;
  return (
    <div className="cast-remote__stage">
      <div className="cast-remote__page">
        {sent.fileId === 0 ? (
          <div className="cast-remote__bar">
            <RemoteUp sign={t("cast.readyOn", { name: sent.receiverName })} />
          </div>
        ) : (
          <RemotePlaying {...props} />
        )}
      </div>
    </div>
  );
}

/**
 * The remote's column while a film plays on the receiver (D548 §1): the bar, the way up and under it
 * where it plays and when it ends (RemoteUp); the offer area, the film's picture with the offer, the
 * Pills and the receiver's silence over it (RemoteOffer); the timeline, the time under it; the pad,
 * in a thumb's reach (RemotePad, D548 §5), its volume the film's level on the receiver
 * (useReceiverFilmSound, §6); the menus over the receiver (RemoteMenus). Two columns where the window
 * is landscape and short or wide (§7, §8: cast.css). While the receiver is not reached the pad, the
 * timeline and the menus take no press (§9).
 */
function RemotePlaying(props: {
  readonly sent: CastSending;
  readonly cast: WorkedSession;
  readonly casts: Resource<readonly Cast[]>;
  readonly hosted: WorkedSession | undefined;
  readonly onAround: (around: AroundFile) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { sent, hosted, onAround } = props;
  const worked = hosted ?? props.cast;
  const isHeard = useReceiverHeard(sent, props.cast.snapshot);
  const source = useRemoteSource({ ...worked, durationMs: sent.durationMs, isHeard });
  // What plays after the cast's film and before it, read once for its ⏮ and its Up next (D477 §2).
  const around = useAroundFile(sent.fileId, true);
  useEffect(() => {
    if (around !== undefined) {
      onAround(around);
    }
  }, [around, onAround]);
  const moves = useRemoteMoves(sent, around);
  // How the receiver plays and sounds is said in the cast's session, and its sound chosen there,
  // whatever else it plays in (the owner, 2026-11-06: "It shouldn't be watch together's business at
  // all which TV has what offset or playback characteristics").
  const playback = receiverPlaybackIn(props.cast.snapshot);
  const film = useReceiverFilmSound(playback?.sound?.channels?.[0], props.cast.connection);
  const volume = useVolumeShown();
  const title = useRemoteTitle(props.casts, sent);
  const isDisabled = !source.isReady;
  return (
    <PlayUpwardContext value={title.upward}>
      <div className="cast-remote__bar" data-focus-container="">
        <RemoteUp sign={t("cast.on", { name: sent.receiverName })}>
          <EndsAt leftMs={leftMsOf(source)} isShown />
        </RemoteUp>
      </div>
      <RemoteOffer
        sent={sent}
        source={source}
        upNext={around?.upNext}
        art={title.art}
        film={volume.isShown ? film : undefined}
      />
      <RemoteTimeline source={source} isDisabled={isDisabled} />
      <RemotePad
        source={source}
        moves={moves}
        sound={remoteSoundMovesOf(film, volume.show)}
        isDisabled={isDisabled}
      />
      <RemoteButtons isDisabled={isDisabled}>
        <RemoteMenus
          playback={playback}
          connection={props.cast.connection}
          report={{ fileId: sent.fileId, snapshot: worked.snapshot }}
        />
      </RemoteButtons>
    </PlayUpwardContext>
  );
}

/**
 * The remote's timeline (D548 §1), its slider's arrows its own, the time under it at the start;
 * disabled while the receiver is not reached (§9).
 */
function RemoteTimeline(props: {
  readonly source: PlayerSource;
  readonly isDisabled: boolean;
}): JSX.Element {
  const { t } = useTranslation();
  const { source } = props;
  return (
    <div className="cast-remote__timeline" data-focus-container="">
      <Timeline
        label={t("playerControls.where")}
        atMs={source.atMs}
        durationMs={source.durationMs}
        isDisabled={props.isDisabled}
        hasArrowsAsCommands={false}
        onSeek={source.seekTo}
        hasTimes={false}
      />
      <span className="cast-remote__time">
        <ClockTimes
          atMs={shownAtMsOf(source.atMs, source.durationMs)}
          durationMs={source.durationMs}
        />
      </span>
    </div>
  );
}

/**
 * The remote's row of menus (D548 §1), aria-disabled while the receiver is not reached (§9), a
 * press on any opener in it then doing nothing.
 */
function RemoteButtons(props: {
  readonly isDisabled: boolean;
  readonly children: ReactNode;
}): JSX.Element {
  const { isDisabled } = props;
  return (
    <div
      className="cast-remote__buttons"
      data-focus-container=""
      aria-disabled={isDisabled ? true : undefined}
      onClickCapture={
        isDisabled
          ? (event) => {
              event.preventDefault();
              event.stopPropagation();
            }
          : undefined
      }
    >
      {props.children}
    </div>
  );
}

/**
 * The remote's ⏮ and ⏭: ⏮ plays the previous episode in the cast (useRemotePrevious), ⏭ moves the
 * cast on to the next, as its Up next does, where one can play next.
 */
function useRemoteMoves(sent: CastSending, around: AroundFile | undefined): SequenceMoves {
  const { api } = useServices();
  const previous = useRemotePrevious(sent, around?.previous);
  const upNext = around?.upNext;
  if (upNext === undefined || upNext.episode.unavailable !== undefined) {
    return previous;
  }
  return {
    ...previous,
    next: () => {
      moveCastOn(api, sent.castId).catch(() => undefined); // the cast goes on as it was
    },
  };
}

/**
 * The player's way up (OverlayUp), under it on one line the remote's sign, its heading, where it
 * plays, and what the bar adds (children): Ends, at the line's end (D548 §1).
 */
function RemoteUp(props: { readonly sign: string; readonly children?: ReactNode }): JSX.Element {
  return (
    <div className="cast-remote__heading">
      <OverlayUp />
      <div className="cast-remote__where">
        <h2 id="cast-remote-heading" className="cast-remote__sign">
          {props.sign}
        </h2>
        {props.children}
      </div>
    </div>
  );
}

/**
 * The cast's title as its remote shows it: the player's way up to it (usePlayUpward), and its art;
 * neither while it is read.
 */
function useRemoteTitle(
  casts: Resource<readonly Cast[]>,
  sent: CastSending,
): {
  readonly upward: ReturnType<typeof usePlayUpward> | undefined;
  readonly art: ArtBody | undefined;
} {
  const title = useCastTitle(casts, sent.castId, sent.fileId);
  const upward = usePlayUpward(title === undefined ? undefined : playsOf(title), title?.name);
  return { upward: title === undefined ? undefined : upward, art: title?.art };
}

/**
 * The title the cast castId plays as fileId: the cast's item, as the server's list of this device's
 * casts names it (casts, the cast's session's read), read again under its file where the cast has
 * moved on since that read, then
 * the title's detail, as its Title page reads it (readTitle). Undefined while read, or where the
 * cast names no title.
 */
function useCastTitle(
  casts: Resource<readonly Cast[]>,
  castId: number,
  fileId: number,
): TitleBody | undefined {
  const { api } = useServices();
  const itemId = useCastItemId(casts, castId, fileId);
  const [title] = useResource(
    itemId === undefined ? "cast-art/none" : titleKey(itemId),
    async (signal) => (itemId === undefined ? undefined : await readTitle(api, itemId, signal)),
  );
  return title.state === "loaded" ? title.value : undefined;
}

/**
 * What a cast plays, title, as a Play page's playback says what it plays (PlaysBody), so the remote's
 * way up is the player's over its film (usePlayUpward): "The caster device shows the back links just
 * like the local video player does" (the owner, 2026-11-23). Its library is the title's first.
 */
function playsOf(title: TitleBody): PlaysBody {
  const library = title.libraries?.[0];
  return {
    itemId: title.id,
    kind: title.kind,
    ...(title.episode === undefined ? {} : { episode: title.episode }),
    ...(title.filmOfExtra === undefined ? {} : { filmOfExtra: title.filmOfExtra }),
    ...(library === undefined ? {} : { libraryId: library.id, libraryName: library.name }),
  };
}

/** The item the cast castId plays as fileId (useCastTitle); undefined while read, or where it names none. */
function useCastItemId(
  casts: Resource<readonly Cast[]>,
  castId: number,
  fileId: number,
): number | undefined {
  const { api } = useServices();
  // The cast's session's read, which the cast started here handed its answer (D478): a reader of its
  // own would ask the server again, the handed answer taken by the first. Read again only for
  // another film.
  const firstCast = sentCastIn(casts, castId);
  const isFirstFilm = firstCast === undefined || firstCast.fileId === fileId;
  const [again] = useResource(
    isFirstFilm ? "cast-item/first" : `${myCastsKey(castId)}/file/${String(fileId)}`,
    async () => (isFirstFilm ? [] : await listMyCasts(api)),
  );
  const cast = sentCastIn(isFirstFilm ? casts : again, castId);
  return cast?.fileId === fileId && cast.itemId !== 0 ? cast.itemId : undefined;
}

/** The cast castId this device sends, as the list read says it; undefined while read, or without it. */
function sentCastIn(casts: Resource<readonly Cast[]>, castId: number): Cast | undefined {
  return casts.state === "loaded"
    ? casts.value.find((each) => each.castId === castId && each.isSender)
    : undefined;
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
