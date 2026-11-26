// SPDX-License-Identifier: MPL-2.0

import "./cast.css";
import "./play-castee.css";
import "./page-controls.css";
import "./cast-words.ts";

import type { CastSending } from "@orpheum/sdk";
import { useEffect, useRef, type JSX, type ReactNode, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import { browserStorage, tabStorage } from "./browser-storage.ts";
import { useReceiverHeard } from "./cast-remote-heard.ts";
import { RemoteOffers, RemotePicture } from "./cast-remote-offer.tsx";
import { takeEndAnswerOf, type Cast } from "./cast.ts";
import {
  isServerLostIn,
  RemoteButtons,
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
import { useCasteeTarget, type WorkedSession } from "./castee-target.ts";
import { EndsAt, leftMsOf } from "./convenience-area.tsx";
import { keepFillingWhileFullScreen, useLeavesFullScreen } from "./film-fullscreen.ts";
import { useIsFullScreen } from "./full-screen-button.tsx";
import { useShowsARemote } from "./hidden-need.ts";
import { useLatest } from "./latest.ts";
import { Pill } from "./pill.tsx";
import { OverlayUp, PlayUpwardContext } from "./play-up.tsx";
import type { PlaybackTarget } from "./playback-target.ts";
import { PlayerControls, type SequenceMoves } from "./player-controls.tsx";
import {
  barControlsOf,
  OverlayOver,
  OverlayRowsOf,
  SignsOverTheFilm,
  useControlsShown,
  type OverlayGiven,
} from "./player-overlay.tsx";
import type { ReopenAt } from "./reopen.ts";
import { TargetMenus } from "./target-menus.tsx";
import { TargetTimeline } from "./target-timeline.tsx";
import { useMembership } from "./together-membership.ts";
import { TogetherBehindTheRemote } from "./together-on-the-remote.tsx";
import { useHasControlsOverThePicture } from "./upright.ts";
import { useAroundFile, type AroundFile, type UpNext } from "./around-file.ts";
import type { Resource } from "./use-resource.ts";

/**
 * The Play page while its device casts the cast castId (D555 §8, the rework plan's phase 4 item 3):
 * the same page with the other target, the castee (useCasteeTarget). Where the video would be, the
 * castee's key frame where it says it stands; under it the Play page's own bar over the castee, its
 * timeline, play or pause between the 10 s steps, previous and next, the castee's volume, the menus;
 * Up next and the skips over the picture, as over the video. Every end of the cast swaps the target
 * back to the video here at the castee's last place, paused alone, playing on in a Watch Together
 * session, with no note and no press asked (D555 §5; useSwapsBackAtTheEnd). The cast moved into a
 * Watch Together part is no end (useCastSession). The castee not heard, the reconnecting sign shows on
 * its key frame and nothing takes a press (D555 §6); the server alone ends the cast. Its controls are
 * the video's Play page's: the bar under the picture held upright out of full screen, else the
 * overlay over the key frame (D555 §8), which full screen fills the screen with (D555 §9).
 */
export function CasteePlay(props: {
  readonly castId: number;
  /** The film of the Play page. */
  readonly fileId: number;
  readonly onPlayHere: PlaysHereFrom;
}): JSX.Element | null {
  // Its page hidden, it keeps connecting as when shown, so the castee's controls stay at hand (D482 §3).
  useShowsARemote();
  const sent = useHeldCast(props.castId);
  if (sent === undefined) {
    return null;
  }
  return (
    <section className="castee-play" aria-labelledby="castee-play-sign">
      <CasteePlayOf sent={sent} pageFileId={props.fileId} onPlayHere={props.onPlayHere} />
    </section>
  );
}

/**
 * The page over the cast sent while it lasts; as it ends, nothing, the film played here again or the
 * page gone home where nothing was on (usePlaysHere). The film ended on the castee with nothing next,
 * the page goes above it, the cast going on (useLeavesAtTheCastsEnd).
 */
function CasteePlayOf(props: {
  readonly sent: CastSending;
  readonly pageFileId: number;
  readonly onPlayHere: PlaysHereFrom;
}): JSX.Element | null {
  const { t } = useTranslation();
  const { sent, pageFileId } = props;
  // Ready on, nothing plays on the castee: a page and no picture, which full screen leaves (D440 §5);
  // a film on it, its key frame fills the screen as the video did (D555 §8).
  useLeavesFullScreen(sent.fileId === 0);
  const { connection, snapshot, over, casts } = useCastSession(sent.castId);
  const hosted = useHostedPanel();
  const stoodRef = useWhereItStood(hosted?.snapshot ?? snapshot);
  const playHere = usePlaysHere(sent, pageFileId, props.onPlayHere);
  useSwapsBackAtTheEnd(sent.castId, over, { stoodRef, isInASession: useIsInASession(), playHere });
  const episodes = useEpisodesMovedTo();
  useLeavesAtTheCastsEnd(sent.fileId, pageFileId, episodes.of);
  if (over !== undefined) {
    return null;
  }
  return (
    <>
      {sent.fileId === 0 ? (
        <div className="castee-play__top">
          <OverlayUp />
          <h2 id="castee-play-sign" className="castee-play__sign">
            {t("cast.readyOn", { name: sent.receiverName })}
          </h2>
        </div>
      ) : (
        <CasteePlaying
          sent={sent}
          cast={{ snapshot, connection }}
          casts={casts}
          hosted={hosted}
          onAround={episodes.note}
        />
      )}
      <TogetherBehindTheRemote hostedSnapshot={hosted?.snapshot} />
    </>
  );
}

/** Whether this tab takes part in a Watch Together session, as its host or a signed-in guest. */
function useIsInASession(): boolean {
  const isHosting = useMembership(tabStorage, "host") !== undefined;
  const isViewing = useMembership(browserStorage, "viewer") !== undefined;
  return isHosting || isViewing;
}

/**
 * Swaps the target back to the video here, once, as the cast castId ends, however it ends (D555 §5):
 * at the castee's last place, the server's answer to this tab's own Stop first, then where the castee
 * last stood (stoodRef), then where the cast's session stood as it ended, and none known, the film's
 * own start (D549 §3); paused when the person watches alone, playing on in a Watch Together session.
 */
function useSwapsBackAtTheEnd(
  castId: number,
  over: CastEnd | undefined,
  ways: {
    readonly stoodRef: RefObject<ReopenAt | undefined>;
    readonly isInASession: boolean;
    readonly playHere: (atMs: number | undefined, isPaused?: boolean) => void;
  },
): void {
  const { stoodRef } = ways;
  const waysRef = useLatest(ways);
  const hasSwappedRef = useRef(false);
  useEffect(() => {
    void takeEndAnswerOf(castId); // an answer to a Stop asked before this page was shown is not its own
  }, [castId]);
  useEffect(() => {
    if (over === undefined || hasSwappedRef.current) {
      return;
    }
    hasSwappedRef.current = true;
    const answered = takeEndAnswerOf(castId) ?? Promise.resolve(undefined);
    void answered.then((ended) => {
      const from = ended ?? stoodRef.current ?? over.stood;
      waysRef.current.playHere(from?.positionMs, !waysRef.current.isInASession);
    });
  }, [castId, over, stoodRef, waysRef]);
}

/**
 * The page while a film plays on the castee: the way up and the cast's sign, "On: ‹TV›", with when
 * the film ends; the stage, the castee's key frame (RemotePicture) with Up next or a skip over it
 * (RemoteOffers), and the reconnecting sign while the castee is not heard; then the Play page's
 * controls over the castee, the bar under the stage or the overlay over it (useHasControlsOverThePicture). The castee is the target (D555 §8); its session the one it is commanded in, the
 * hosted session's while this tab hosts Watch Together (D364 §11), its sound and choices the cast's.
 */
function CasteePlaying(props: {
  readonly sent: CastSending;
  readonly cast: WorkedSession;
  readonly casts: Resource<readonly Cast[]>;
  readonly hosted: WorkedSession | undefined;
  readonly onAround: (around: AroundFile) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { sent, hosted, onAround } = props;
  const isHeard = useReceiverHeard(sent, props.cast.snapshot);
  const target = useCasteeTarget({ worked: hosted ?? props.cast, cast: props.cast, sent, isHeard });
  // What plays after the cast's film and before it, read once for ⏮, ⏭ and Up next (D477 §2).
  const around = useAroundFile(sent.fileId, true);
  useEffect(() => {
    if (around !== undefined) {
      onAround(around);
    }
  }, [around, onAround]);
  const moves = useRemoteMoves(sent, around);
  const title = useRemoteTitle(props.casts, sent);
  const isDisabled = !target.isReady;
  const isOver = useHasControlsOverThePicture();
  const reconnecting = isServerLostIn([props.cast, hosted])
    ? t("cast.serverLost")
    : t("cast.reaching", { name: sent.receiverName });
  return (
    <PlayUpwardContext value={title.upward}>
      <div className="castee-play__top">
        <OverlayUp />
        <div className="castee-play__where">
          <h2 id="castee-play-sign" className="castee-play__sign">
            {t("cast.on", { name: sent.receiverName })}
          </h2>
          <EndsAt leftMs={leftMsOf(target)} isShown />
        </div>
      </div>
      <CasteeStage
        sent={sent}
        target={target}
        upNext={around?.upNext}
        art={title.art}
        sign={isDisabled ? reconnecting : undefined}
        where={<CasteeWhere name={sent.receiverName} target={target} />}
        overlay={(pictureRef) =>
          isOver ? (
            <CasteeOverlay
              target={target}
              moves={moves}
              isDisabled={isDisabled}
              pictureRef={pictureRef}
            />
          ) : null
        }
      />
      {isOver ? null : <CasteeBar target={target} moves={moves} isDisabled={isDisabled} />}
    </PlayUpwardContext>
  );
}

/**
 * Where the video would be, the castee's key frame where it says it stands (RemotePicture), with Up
 * next or a skip over it (RemoteOffers), and, while the castee is not heard, the reconnecting sign
 * (sign) in its centre (D555 §6); the overlay over it where drawn. Filling the screen in full screen
 * as the video's stage does (keepFillingWhileFullScreen, the class its full screen finds), where it
 * stays, the page above it hidden, "On: ‹TV›" with when the film ends (where) over its top.
 */
function CasteeStage(props: {
  readonly sent: CastSending;
  readonly target: PlaybackTarget;
  readonly upNext: UpNext | undefined;
  readonly art: Parameters<typeof RemotePicture>[0]["art"];
  readonly sign: string | undefined;
  readonly where: ReactNode;
  /** The overlay over the stage, given what focus goes to as it hides; null where none is drawn. */
  readonly overlay: (pictureRef: RefObject<HTMLElement | null>) => ReactNode;
}): JSX.Element {
  const { sent, target } = props;
  const stageRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const stage = stageRef.current;
    return stage === null ? undefined : keepFillingWhileFullScreen(stage, document);
  }, []);
  const isFullScreen = useIsFullScreen();
  return (
    <div ref={stageRef} className="film-player__stage castee-play__stage">
      <div className="castee-play__picture" aria-hidden="true">
        <RemotePicture fileId={sent.fileId} atMs={target.reportedAtMs} art={props.art} />
      </div>
      <div className="castee-play__offers">
        <RemoteOffers sent={sent} source={target} upNext={props.upNext} />
      </div>
      {props.sign === undefined ? null : (
        <span className="castee-play__centre">
          <Pill icon={<span className="cast-remote__spinner" />}>{props.sign}</Pill>
        </span>
      )}
      {isFullScreen ? props.where : null}
      {props.overlay(stageRef)}
    </div>
  );
}

/** Over the stage filling the screen, where the film plays and when it ends, shown throughout. */
function CasteeWhere(props: {
  readonly name: string;
  readonly target: PlaybackTarget;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <p className="castee-play__where castee-play__where--over">
      <span className="castee-play__sign">{t("cast.on", { name: props.name })}</span>
      <EndsAt leftMs={leftMsOf(props.target)} isShown />
    </p>
  );
}

/** The castee's overlay holds no video: its presses go to the castee (OverlayOver's target). */
const noVideo: RefObject<HTMLVideoElement | null> = { current: null };

/**
 * The video's overlay (OverlayOver) over the castee's key frame (D555 §8): its showing, hiding and
 * presses as over the video, the castee its target, nothing taking a press while it is not heard.
 */
function CasteeOverlay(props: {
  readonly target: PlaybackTarget;
  readonly moves: SequenceMoves;
  readonly isDisabled: boolean;
  readonly pictureRef: RefObject<HTMLElement | null>;
}): JSX.Element {
  const { moves, isDisabled } = props;
  return (
    <OverlayOver
      videoRef={noVideo}
      pictureRef={props.pictureRef}
      target={props.target}
      rows={(over) => <CasteeOverlayRows over={over} moves={moves} isDisabled={isDisabled} />}
    />
  );
}

/**
 * The overlay's rows over the castee, as over the video: the way up in full screen (above the stage
 * otherwise), the timeline, then the bar's buttons with the castee's volume, its menus and the signs
 * over the film; when the film ends is the stage's own, shown throughout (CasteeWhere).
 */
function CasteeOverlayRows(props: {
  readonly over: OverlayGiven;
  readonly moves: SequenceMoves;
  readonly isDisabled: boolean;
}): JSX.Element {
  const { over, moves, isDisabled } = props;
  const shown = useControlsShown(over.isTv, moves);
  const isFullScreen = useIsFullScreen();
  return (
    <OverlayRowsOf
      top={<div className="player-overlay__heading">{isFullScreen ? <OverlayUp /> : null}</div>}
      source={over.source}
      refs={over.refs}
      controls={[...barControlsOf(shown), "sound"]}
      moves={moves}
      menus={
        <>
          <RemoteButtons isDisabled={isDisabled}>
            <TargetMenus target={over.source} onOpenChange={over.onMenuOpenChange} />
          </RemoteButtons>
          <SignsOverTheFilm onOpenChange={over.onMenuOpenChange} />
        </>
      }
      isOverTheFilm
    />
  );
}

/**
 * The Play page's bar under the film (page-controls.css) over the castee, held upright out of full
 * screen: the timeline, then ⏮ ⟲ ▶ ⟳ ⏭, the menus and the castee's volume, none taking a press
 * while it is not heard.
 */
function CasteeBar(props: {
  readonly target: PlaybackTarget;
  readonly moves: SequenceMoves;
  readonly isDisabled: boolean;
}): JSX.Element {
  const { target, isDisabled } = props;
  return (
    <div className="page-controls castee-play__bar" data-focus-container="">
      <TargetTimeline target={target} isDisabled={isDisabled} hasArrowsAsCommands={false} />
      <PlayerControls
        source={target}
        controls={["previous", "steps", "play", "next", "sound"]}
        isOverVideo
        isOverlayRow
        moves={props.moves}
        menus={
          <RemoteButtons isDisabled={isDisabled}>
            <TargetMenus target={target} />
          </RemoteButtons>
        }
      />
    </div>
  );
}
