// SPDX-License-Identifier: MPL-2.0

import "./cast.css";
import "./play-words.ts";
import "./cast-words.ts";

import type { CastSending } from "@orpheum/sdk";
import { useEffect, useRef, type JSX, type ReactNode, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import { RemotePad, remoteSoundMovesOf } from "./cast-remote-pad.tsx";
import { takeEndAnswerOf, type Cast } from "./cast.ts";
import { useLeavesFullScreen } from "./film-fullscreen.ts";
import { useShowsARemote } from "./hidden-need.ts";
import type { PlayerSource } from "./playback-target.ts";
import { useVolumeShown } from "./over-the-picture.tsx";
import { TargetMenus } from "./target-menus.tsx";
import { TargetTimeline } from "./target-timeline.tsx";
import { EndsAt, leftMsOf } from "./convenience-area.tsx";
import { RemoteOffer, type RemoteEnded } from "./cast-remote-offer.tsx";
import { useCastAgain } from "./cast-remote-again.ts";
import {
  playsFromWhereItStood,
  useEndWay,
  useRemoteEnded,
  useTellsTheHost,
} from "./cast-remote-ends.ts";
import { useReceiverHeard } from "./cast-remote-heard.ts";
import { OverlayUp, PlayUpwardContext } from "./play-up.tsx";
import { useLatest } from "./latest.ts";
import { homeTold } from "./notice.ts";
import type { ReopenAt } from "./reopen.ts";
import { useServices } from "./services.tsx";
import { useCasteeTarget, type WorkedSession } from "./castee-target.ts";
import { TogetherBehindTheRemote } from "./together-on-the-remote.tsx";
import { useAroundFile, type AroundFile } from "./around-file.ts";
import type { Resource } from "./use-resource.ts";
import {
  isServerLostIn,
  RemoteButtons,
  useCastItemId,
  useCastSession,
  useEpisodesMovedTo,
  useHeldCast,
  useHostedPanel,
  useLeavesAtTheCastsEnd,
  usePlaysHere,
  useRemoteMoves,
  useRemoteTitle,
  useWhereItStood,
  type CastEnd,
  type PlaysHereFrom,
} from "./castee-session.tsx";

export type { PlaysHereFrom } from "./castee-session.tsx";

/**
 * The Play page's video area while its device casts (D384 §2, D364 §8): the page keeps its place, and
 * where its video was it shows the remote of the cast castId, "Playing on ‹device›" with its
 * controls, which work the receiver. This tab's own Stop casting brings the film back to the page
 * (onPlayHere) at once, from where the receiver last stood, playing or paused as it was (D446 §2);
 * any other end, the receiver's take-back, its loss, the server's, keeps the remote, its note saying
 * why, and only Play here brings the film back (D552 §4). With nothing on, any end sends the page
 * home (D446 §2), saying so where the server no longer knows the cast (D386 §2). Where the cast
 * plays another film than the page's by then, the film comes back on that
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
 * The remote of one cast (D364 §8): it holds no player and loads no film; what it shows is what the
 * live connection says plays on the receiver, and where, as the cast's session says, moved on by the
 * clock between. With a video playing: the full-screen overlay's rows, its title as a sign, the
 * controls and Up next at the episode's end; with none, "Ready on ‹Device›" (D444). Stop casting, the
 * one way to bring the film back here (D388), is in the Cast sign's dialog, as everywhere (D444).
 * Stopped by this tab, the film plays here again at once, as the receiver left it (D446 §2); ended
 * any other way, the cast over, the remote stays with its note saying why, Play here bringing the
 * film back from where the receiver likely got to, and Cast again, once the receiver is back, casting
 * it there anew (D552 §2, §4, D553 §2); while this tab hosts Watch Together, a receiver gone has the
 * film play here at once, a notice saying why (D553 §3); with nothing on, the page goes home (D446
 * §2, D386 §2).
 * The cast's film ended with nothing next, the page goes to the page above it, or above the episode
 * the cast had moved on to, the cast going on (useLeavesAtTheCastsEnd).
 */
function Remote(props: {
  readonly sent: CastSending;
  readonly pageFileId: number;
  readonly onPlayHere: PlaysHereFrom;
}): JSX.Element | null {
  const { sent, pageFileId } = props;
  const { connection, snapshot, over, casts } = useCastSession(sent.castId);
  const hosted = useHostedPanel();
  const isHosting = hosted !== undefined;
  const stoodRef = useWhereItStood(hosted?.snapshot ?? snapshot);
  const playHere = usePlaysHere(sent, pageFileId, props.onPlayHere);
  const reason = over?.reason;
  const way = useEndWay(sent.castId, { reason, fileId: sent.fileId, isHosting });
  const takeOver = useTellsTheHost(playHere, {
    castId: sent.castId,
    reason,
    isHosting,
    name: sent.receiverName,
  });
  useTakesTheFilmOver(sent.castId, way === "takenOver" ? over : undefined, {
    stoodRef,
    playHere: takeOver,
  });
  useGoesHomeAsTheCastIsLost(way === "lost" ? over : undefined);
  const episodes = useEpisodesMovedTo();
  useLeavesAtTheCastsEnd(sent.fileId, pageFileId, episodes.of);
  const stood = playsFromWhereItStood(over, { stoodRef, playHere });
  const itemId = useCastItemId(casts, sent.castId, sent.fileId);
  const castAgain = useCastAgain(sent, { pageFileId, itemId }, stood.from);
  const ended = useRemoteEnded({ way, over }, sent.receiverName, {
    playHere: stood.play,
    castAgain,
  });
  if (over !== undefined && way !== "ended") {
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
        ended={ended}
      />
      <TogetherBehindTheRemote hostedSnapshot={hosted?.snapshot} />
    </>
  );
}

/**
 * Sends the page home, once, as its cast with nothing on ends (endWayOf) and the server knows it no
 * more ("gone": not among the device's casts, or its session's credential refused), as after a restart,
 * saying so in a pop-up there (the shared notice, notice-note.tsx). The owner: "For extreme
 * edge cases like this, it's usually best to just go home" (D386 §2). A cast ended with the Watch
 * Together session it took a part in (sessionEnded, D389 §1) is a normal end, and goes home with no
 * word: the session's own end tells this page of it, as it takes it home too.
 */
function useGoesHomeAsTheCastIsLost(over: CastEnd | undefined): void {
  const { t } = useTranslation();
  const { navigation } = useServices();
  const isLost = over !== undefined;
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
 * Plays the film here again, once, as its cast ends taken over (endWayOf), stopped here: from where
 * the receiver last stood, playing or paused as it was,
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
    if (over === undefined || hasTakenRef.current) {
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
  /** The cast's end this tab did not ask, said and played here from (D552 §4); else undefined. */
  readonly ended: RemoteEnded | undefined;
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
 * (useReceiverFilmSound, §6); the menus over the receiver (TargetMenus). Two columns where the window
 * is landscape and short or wide (§7, §8: cast.css). While the receiver is not reached the pad, the
 * timeline and the menus take no press (§9). The castee is the playback target (D555 §8); its sound
 * and choices are the cast session's, whatever else it plays in (the owner, 2026-11-06: "It
 * shouldn't be watch together's business at all which TV has what offset or playback
 * characteristics").
 */
function RemotePlaying(props: {
  readonly sent: CastSending;
  readonly cast: WorkedSession;
  readonly casts: Resource<readonly Cast[]>;
  readonly hosted: WorkedSession | undefined;
  readonly onAround: (around: AroundFile) => void;
  readonly ended: RemoteEnded | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  const { sent, hosted, onAround } = props;
  const worked = hosted ?? props.cast;
  const isHeard = useReceiverHeard(sent, props.cast.snapshot);
  const source = useCasteeTarget({ worked, cast: props.cast, sent, isHeard });
  // What plays after the cast's film and before it, read once for its ⏮ and its Up next (D477 §2).
  const around = useAroundFile(sent.fileId, true);
  useEffect(() => {
    if (around !== undefined) {
      onAround(around);
    }
  }, [around, onAround]);
  const moves = useRemoteMoves(sent, around);
  // How the receiver plays and sounds is said in the cast's session, and its sound chosen there,
  // whatever else it plays in (the owner, 2026-11-06), the target's sound and choices (D555 §8).
  const film = source.sound;
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
        ended={props.ended}
        isServerLost={isServerLostIn([props.cast, hosted])}
      />
      <RemoteTimeline source={source} isDisabled={isDisabled} />
      <RemotePad
        source={source}
        moves={moves}
        sound={remoteSoundMovesOf(film, volume.show)}
        isDisabled={isDisabled}
      />
      <RemoteButtons isDisabled={isDisabled}>
        <TargetMenus target={source} />
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
  return (
    <div className="cast-remote__timeline" data-focus-container="">
      <TargetTimeline
        target={props.source}
        isDisabled={props.isDisabled}
        hasArrowsAsCommands={false}
        timesClassName="cast-remote__time"
      />
    </div>
  );
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
