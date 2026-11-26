// SPDX-License-Identifier: MPL-2.0

import "./cast.css";
import "./play-castee.css";
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
import { useLeavesFullScreen } from "./film-fullscreen.ts";
import { useShowsARemote } from "./hidden-need.ts";
import { isOnTelevision } from "./input-mode.ts";
import { useLatest } from "./latest.ts";
import { useVolumeShown, VolumeShown } from "./over-the-picture.tsx";
import { Pill } from "./pill.tsx";
import { OverlayUp, PlayUpwardContext } from "./play-up.tsx";
import type { PlaybackTarget } from "./playback-target.ts";
import { useStageCommands, usePlayersControls } from "./player-commands.ts";
import { useStageNow } from "./overlay-stage.ts";
import { useControlsShown } from "./player-overlay.tsx";
import { PadMenus, PadTimeline, RemotePad, remoteSoundMovesOf } from "./remote-pad.tsx";
import type { ReopenAt } from "./reopen.ts";
import { TargetMenus } from "./target-menus.tsx";
import { useMembership } from "./together-membership.ts";
import { TogetherBehindTheRemote } from "./together-on-the-remote.tsx";
import { useAroundFile, type AroundFile, type UpNext } from "./around-file.ts";
import type { Resource } from "./use-resource.ts";

/**
 * The Play page while its device casts the cast castId (D555 §8, the rework plan's phase 4 item 3):
 * the same page with the other target, the castee (useCasteeTarget), laid out as the remote was
 * (D548; the owner, 2026-11-26), upright, on its side and on a computer alike, full screen or not
 * (CasteePlaying): the castee's key frame where the video would be, with Up next and the skips over
 * it; under it, or beside it on its side, the timeline, the 3×3 pad and the menus' row. Every end of
 * the cast swaps the target back to the video here at the castee's last place, paused alone, playing
 * on in a Watch Together session, with no note and no press asked (D555 §5; useSwapsBackAtTheEnd).
 * The cast moved into a Watch Together part is no end (useCastSession). The castee not heard, the
 * reconnecting sign shows on its key frame and nothing takes a press (D555 §6); the server alone ends
 * the cast. The page fills the window under the top bar, scrolling where it does not fit
 * (play-castee.css).
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
  // Casting is never full screen (the owner, 2026-11-26: "Never offer the fullscreen mode button when
  // casting"): a cast started from the film full screen leaves it, as Ready on always did (D440 §5).
  useLeavesFullScreen();
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
        <div className="castee-play__page">
          <div className="castee-play__top">
            <div className="castee-play__heading">
              <OverlayUp />
            </div>
            <h2 id="castee-play-sign" className="castee-play__sign">
              {t("cast.readyOn", { name: sent.receiverName })}
            </h2>
          </div>
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

/** The castee's stage holds no video: its presses, keys and commands go to the castee, its target. */
const noVideo: RefObject<HTMLVideoElement | null> = { current: null };

/**
 * What plays after the cast's film and before it, read once for ⏮, ⏭ and Up next (D477 §2), told to
 * onAround as it is read.
 */
function useAroundTold(
  fileId: number,
  onAround: (around: AroundFile) => void,
): AroundFile | undefined {
  const around = useAroundFile(fileId, true);
  useEffect(() => {
    if (around !== undefined) {
      onAround(around);
    }
  }, [around, onAround]);
  return around;
}

/**
 * Works the Play page's keys and commands on the castee as on the video (D555 §8), the stage the
 * returned ref holds its picture: nothing taken while it is not heard (D555 §6).
 */
function useCasteeCommands(target: PlaybackTarget): RefObject<HTMLDivElement | null> {
  const stageRef = useRef<HTMLDivElement>(null);
  useStageCommands(useStageNow({ videoRef: noVideo, pictureRef: stageRef }, target));
  return stageRef;
}

/**
 * The page while a film plays on the castee, the remote's layout (D548 §1; the owner, 2026-11-26:
 * "the remote-control looking layout (3x3 pad). That should be the layout for portrait and
 * landscape mode when casting"): the bar, the way up and under it "On: ‹TV›" with when the film ends;
 * the offer area, the castee's key frame (RemotePicture) with Up next or a skip over it (RemoteOffers)
 * and in its centre the reconnecting sign while the castee is not heard, else the volume a moment
 * after the pad set it; the timeline, the time under it; the pad (RemotePad), its volume the castee's
 * film's level; the menus' row at the end, with no ⤢: casting is never full screen (the owner,
 * 2026-11-26: "Never offer the fullscreen mode button when casting"), and a browser's own full screen
 * shows this same page. One column upright, two on its side or a computer's wide window
 * (play-castee.css). The castee is the target (D555 §8); its session the one it is commanded in, the
 * hosted session's while this tab hosts Watch Together (D364 §11), its sound and choices the cast's.
 * While the castee is not heard, nothing takes a press (D555 §6). The page is the player's own
 * controls for the castee's stage (usePlayersControls), so Space and M are the player's on it.
 */
function CasteePlaying(props: {
  readonly sent: CastSending;
  readonly cast: WorkedSession;
  readonly casts: Resource<readonly Cast[]>;
  readonly hosted: WorkedSession | undefined;
  readonly onAround: (around: AroundFile) => void;
}): JSX.Element {
  const { sent, hosted, onAround } = props;
  const isHeard = useReceiverHeard(sent, props.cast.snapshot);
  const target = useCasteeTarget({ worked: hosted ?? props.cast, cast: props.cast, sent, isHeard });
  const around = useAroundTold(sent.fileId, onAround);
  const moves = useRemoteMoves(sent, around);
  const title = useRemoteTitle(props.casts, sent);
  const isDisabled = !target.isReady;
  const stageRef = useCasteeCommands(target);
  const pageRef = useRef<HTMLDivElement>(null);
  usePlayersControls(pageRef, stageRef);
  const volume = useVolumeShown();
  const shown = useControlsShown(isOnTelevision(), moves);
  return (
    <PlayUpwardContext value={title.upward}>
      <div ref={pageRef} className="castee-play__page" data-player-controls="">
        <CasteeTop receiverName={sent.receiverName} leftMs={leftMsOf(target)} />
        <CasteeStage
          stageRef={stageRef}
          sent={sent}
          target={target}
          upNext={around?.upNext}
          art={title.art}
          centre={
            isDisabled ? (
              <CasteeReaching
                receiverName={sent.receiverName}
                isServerLost={isServerLostIn([props.cast, hosted])}
              />
            ) : (
              <VolumeShown sound={target.sound} isShown={volume.isShown} />
            )
          }
        />
        <PadTimeline source={target} isDisabled={isDisabled} hasArrowsAsCommands={false} />
        <RemotePad
          source={target}
          moves={moves}
          shown={shown}
          sound={remoteSoundMovesOf(target.sound, volume.show)}
          isDisabled={isDisabled}
        />
        <PadMenus
          menus={
            <RemoteButtons isDisabled={isDisabled}>
              <TargetMenus target={target} />
            </RemoteButtons>
          }
        />
      </div>
    </PlayUpwardContext>
  );
}

/** The offer area's centre while the castee is not reached: "Reaching ‹TV›…", or the server lost. */
function CasteeReaching(props: {
  readonly receiverName: string;
  readonly isServerLost: boolean;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <span className="castee-play__centre">
      <Pill icon={<span className="cast-remote__spinner" />}>
        {props.isServerLost
          ? t("cast.serverLost")
          : t("cast.reaching", { name: props.receiverName })}
      </Pill>
    </span>
  );
}

/** The castee page's bar: the way up, and under it "On: ‹TV›" and when the film ends. */
function CasteeTop(props: {
  readonly receiverName: string;
  readonly leftMs: number | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <div className="castee-play__top" data-focus-container="">
      <div className="castee-play__heading">
        <OverlayUp />
      </div>
      <div className="castee-play__where">
        <h2 id="castee-play-sign" className="castee-play__sign">
          {t("cast.on", { name: props.receiverName })}
        </h2>
        <EndsAt leftMs={props.leftMs} isShown />
      </div>
    </div>
  );
}

/**
 * The offer area (D548 §2): where the video would be, the castee's key frame where it says it stands
 * (RemotePicture), 16:9 and as large as its cell lets it, with Up next or a skip over it
 * (RemoteOffers) and its centre's one sign (centre).
 */
function CasteeStage(props: {
  /** The stage itself, the castee's picture to the player's keys and commands (PlayerStage). */
  readonly stageRef: RefObject<HTMLDivElement | null>;
  readonly sent: CastSending;
  readonly target: PlaybackTarget;
  readonly upNext: UpNext | undefined;
  readonly art: Parameters<typeof RemotePicture>[0]["art"];
  readonly centre: ReactNode;
}): JSX.Element {
  const { sent, target, stageRef, art, upNext, centre } = props;
  return (
    <div className="castee-play__offer-cell">
      <div ref={stageRef} className="castee-play__stage">
        <div className="castee-play__picture" aria-hidden="true">
          <RemotePicture fileId={sent.fileId} atMs={target.reportedAtMs} art={art} />
        </div>
        <div className="castee-play__offers">
          <RemoteOffers sent={sent} source={target} upNext={upNext} />
        </div>
        {centre}
      </div>
    </div>
  );
}
