// SPDX-License-Identifier: MPL-2.0

import "./cast.css";
import "./play-castee.css";
import "./page-controls.css";
import "./cast-words.ts";

import type { CastSending } from "@orpheum/sdk";
import { useEffect, useRef, type JSX, type RefObject } from "react";
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
import { useLatest } from "./latest.ts";
import { Pill } from "./pill.tsx";
import { OverlayUp, PlayUpwardContext } from "./play-up.tsx";
import type { PlaybackTarget } from "./playback-target.ts";
import { PlayerControls, type SequenceMoves } from "./player-controls.tsx";
import type { ReopenAt } from "./reopen.ts";
import { TargetMenus } from "./target-menus.tsx";
import { TargetTimeline } from "./target-timeline.tsx";
import { useMembership } from "./together-membership.ts";
import { TogetherBehindTheRemote } from "./together-on-the-remote.tsx";
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
 * its key frame and nothing takes a press (D555 §6); the server alone ends the cast.
 */
export function CasteePlay(props: {
  readonly castId: number;
  /** The film of the Play page. */
  readonly fileId: number;
  readonly onPlayHere: PlaysHereFrom;
}): JSX.Element | null {
  useLeavesFullScreen();
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
 * (RemoteOffers), and the reconnecting sign while the castee is not heard; then the Play page's bar
 * over the castee. The castee is the target (D555 §8); its session the one it is commanded in, the
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
      />
      <CasteeBar target={target} moves={moves} isDisabled={isDisabled} />
    </PlayUpwardContext>
  );
}

/**
 * Where the video would be, the castee's key frame where it says it stands (RemotePicture), with Up
 * next or a skip over it (RemoteOffers), and, while the castee is not heard, the reconnecting sign
 * (sign) in its centre (D555 §6).
 */
function CasteeStage(props: {
  readonly sent: CastSending;
  readonly target: PlaybackTarget;
  readonly upNext: UpNext | undefined;
  readonly art: Parameters<typeof RemotePicture>[0]["art"];
  readonly sign: string | undefined;
}): JSX.Element {
  const { sent, target } = props;
  return (
    <div className="castee-play__stage">
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
    </div>
  );
}

/**
 * The Play page's bar under the film (page-controls.css) over the castee: the timeline, then
 * ⏮ ⟲ ▶ ⟳ ⏭, the menus and the castee's volume, none taking a press while it is not heard.
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
