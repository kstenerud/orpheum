// SPDX-License-Identifier: MPL-2.0

import "./play.css";

import type { CannotKeepUp } from "@orpheum/sdk";
import {
  createContext,
  use,
  useCallback,
  useEffect,
  useMemo,
  useState,
  type JSX,
  type ReactNode,
  type RefObject,
} from "react";
import { useTranslation } from "react-i18next";

import { PlayTop } from "./play-up.tsx";
import { AudioChoiceMenu } from "./audio-tracks.tsx";
import {
  FilmPlayer,
  type FilmComposition,
  type FilmParts,
  deliveryOf,
  type MovedDown,
} from "./film-player.tsx";
import { problemText } from "./problem.ts";
import type { ReopenAt } from "./reopen.ts";
import type { LadderOffer } from "./moving-down.ts";
import { usePlayState } from "./play-state.ts";
import { usePlayerCommands } from "./player-commands.ts";
import { fillTheScreen, keepFillingTheWindow } from "./film-fullscreen.ts";
import { keepThePressForTheStart } from "./start-refused.ts";
import { useVideoPressedForPlay } from "./video-pressed-for-play.ts";
import { isBrowserFullscreen, isTelevision, thisBrowserWindow } from "./tv-device.ts";
import { useFullScreenLeftAsBack } from "./input-layer.tsx";
import { useServices } from "./services.tsx";
import { BarePlayer } from "./bare-player.tsx";
import { PlayOriginal } from "./play-original.tsx";
import {
  ForTesters,
  ForTestersContext,
  ForTestersHome,
  useForTestersPanel,
} from "./for-testers.tsx";
import { FrameTimingNote } from "./frame-timing-note.tsx";
import { BoostBox } from "./sound.tsx";
import { SoundOffsetControl } from "./sound-delay.tsx";
import { FindSubtitles } from "./subtitle-search.tsx";
import { SubtitleControls } from "./subtitles.tsx";
import { LevellingControl } from "./levelling.tsx";
import { LadderTracks, OwnFileTracks, usePlayerTracks } from "./player-tracks.tsx";
import { LinkDebugTools } from "./link-debug-tools.tsx";
import { useIsFullScreen } from "./full-screen-button.tsx";
import { PlayerMenusContext, StartOver, type PlayerMenusWays } from "./player-menus.tsx";
import { usePlaybackHere } from "./how-it-plays.tsx";
import {
  newPicturePanel,
  PicturePanelContext,
  useOriginalOfferedToARemote,
} from "./picture-panel.ts";
import { newSoundPanel, SoundPanelContext } from "./sound-panel.ts";
import { Loaded } from "./title.tsx";
import { PlaysAlong, useReceiving, type Receiving } from "./cast-receiver.tsx";
import { PlayOrSendOn, type PlayedHereFrom } from "./cast-to.tsx";
import { WhoseFilm } from "./together-guest-play.tsx";
import { useNotesPlayingHere } from "./playing-here.ts";
import type { Opened } from "./play-start-timing.ts";
import {
  shownOf,
  useOpenedPlayback,
  type DecideAgain,
  type DecideAgainCause,
  type FirstAsk,
} from "./play-opened.ts";
import {
  ControlsUnderTheFilm,
  overTheFilm,
  UpNextContext,
  useSequenceCommands,
  useUpNextOffered,
} from "./up-next.tsx";
import { playsOwnFile, type PlaybackDecision } from "./watching.ts";

/** What PlayView is given: the file, its decision, and its ways to ask again. */
interface PlayViewProps {
  readonly fileId: number;
  readonly decision: PlaybackDecision;
  /**
   * Asks the server again how the file plays: after a subtitle was fetched, another audio track or
   * the original chosen, or the dialogue boost turned on over the original (D336 §3).
   */
  readonly onDecideAgain: DecideAgain;
  /**
   * Told that the server lost the playback, and how to read where it is: true when the page opens
   * it again from there; false, or undefined, tells the person.
   */
  readonly onPlaybackGone?: ((whereNow: () => ReopenAt) => boolean) | undefined;
  /** Told, with where the video stood, that its film can be read again after it could not. */
  readonly onMediaBack?: ((at: ReopenAt) => void) | undefined;
  /**
   * Told where the video stood when its player gave the film's own picture up, for the page to ask
   * for the playback again from there, at the twin (D323 §2).
   */
  readonly onOwnPictureGivenUp?: ((at: ReopenAt) => void) | undefined;
  /** Whether the film waits to be played, as one the person had paused when it was opened again. */
  readonly isStartPaused?: boolean;
  /** The cast this device receives of the film, while it does (D337; cast-receiver.tsx). */
  readonly receiving?: Receiving | undefined;
  /**
   * Whether the page is opening the playback again, this one lost: what it plays is held on screen,
   * and its player's failures, its media gone with the playback, are not told.
   */
  readonly isReconnecting?: boolean;
}

/**
 * A decision, shown: the original playing, or its ladder. The reason is always on screen (D188).
 * Another audio track chosen in the original asks for the decision again, which plays it from the
 * ladder, since a browser plays the original's first (D230 §5).
 */
export function PlayView(props: PlayViewProps): JSX.Element {
  // Whether the film's own file plays in the bare player, nothing drawn over it (bare-player.tsx),
  // offered where the film plays its own file.
  const [isBare, setIsBare] = useState(false);
  // The sound controls under the film, as one panel a cast's remote shows and works.
  const [soundPanel] = useState(newSoundPanel);
  // What the film shows that its person may choose, as one panel a cast's remote shows and works.
  const [picturePanel] = useState(newPicturePanel);
  const onBare = useCallback(() => {
    setIsBare(true);
  }, []);
  const { decision } = props;
  const isOwnFile = playsOwnFile(decision);
  if (isBare && isOwnFile) {
    return (
      <BarePlayer
        url={decision.url}
        resumePositionMs={decision.resumePositionMs}
        soundOffsetMs={decision.soundOffsetMs}
        onBack={() => {
          setIsBare(false);
        }}
      />
    );
  }
  return (
    <SoundPanelContext value={soundPanel}>
      <PicturePanelContext value={picturePanel}>
        <PlayerView {...props} onBare={isOwnFile ? onBare : undefined} />
      </PicturePanelContext>
    </SoundPanelContext>
  );
}

/**
 * The lower quality a stalled player moved to, and where it was (D247 §2), per playback; and the way
 * to move, which keeps the first move.
 */
function useMoveToLowerQuality(
  mediaToken: string,
): [MovedDown | undefined, (offer: LadderOffer, positionMs: number) => void] {
  const [moved, setMoved] = useState<MovedDown | undefined>(undefined);
  const onLowerQuality = useCallback(
    (offer: LadderOffer, positionMs: number) => {
      setMoved((already) => already ?? { mediaToken, offer, positionMs });
    },
    [mediaToken],
  );
  return [moved, onLowerQuality];
}

/** PlayView's player, and the bare player's button where onBare is given. */
function PlayerView(
  props: PlayViewProps & { readonly onBare: (() => void) | undefined },
): JSX.Element {
  const [moved, onLowerQuality] = useMoveToLowerQuality(props.decision.mediaToken);
  // The video playing now, which the host's Watch Together session follows: kept here, above the
  // way of playing, so a move to a lower quality is no film stopped and put on again for everyone
  // (the owner's test, 2026-10-05).
  const [video, setVideo] = useState<HTMLVideoElement | null>(null);
  // Whether its lowest quality cannot keep up, which a host's session tells everyone (D247 §8).
  const [cannotKeepUp, setCannotKeepUp] = useState<CannotKeepUp | undefined>(undefined);
  const { fileId, decision, receiving, onBare } = props;
  const { onDecideAgain, onPlaybackGone, onOwnPictureGivenUp } = props;
  const pressedVideo = useVideoPressedForPlay();
  const offered = useUpNextOffered(fileId, video, { ...decision.preferences, receiving });
  const tracks = useSequenceCommands(offered);
  usePlayerCommands(video, tracks);
  useFullscreenAsTheBrowserIs(video);
  // A cast's receiver fills the window itself (cast-receiver.tsx).
  useWholeWindow(receiving === undefined ? video : null);
  useFullScreenLeftAsBack(receiving === undefined && isTelevision(navigator.userAgent));
  const ways = useMemo(
    () => ({
      fileId,
      decision,
      onDecideAgain,
      onPlaybackGone,
      onOwnPictureGivenUp,
      onLowerQuality,
      onVideo: setVideo,
      onCannotKeepUp: setCannotKeepUp,
      onBare,
    }),
    [fileId, decision, onDecideAgain, onPlaybackGone, onOwnPictureGivenUp, onLowerQuality, onBare],
  );
  const { titleName, resumePositionMs } = props.decision;
  // A cast's receiver shows the film and nothing else (D337 §3).
  const isOwn = receiving === undefined;
  useNotesWhereItPlays(isOwn, { fileId, video, startPositionMs: resumePositionMs });
  return (
    <UpNextContext value={offered}>
      <PlayTop plays={decision.plays} titleName={titleName} isOwn={isOwn}>
        <PlayContext value={ways}>
          <PlayArea isOwn={isOwn}>
            <FilmPlayer
              decision={props.decision}
              moved={moved}
              ladderStartMs={resumePositionMs}
              composition={pageComposition(props, pressedVideo)}
            />
            <ForTesters>
              <FrameTimingNote video={video} />
              {isOwn ? <UnderThePlayer onBare={props.onBare} /> : null}
            </ForTesters>
          </PlayArea>
        </PlayContext>
      </PlayTop>
      <PlaysAlong
        {...{ receiving, fileId, video, cannotKeepUp }}
        delivery={deliveryOf(decision, moved)}
        startPositionMs={resumePositionMs}
        hasNext={tracks.hasNext}
      />
    </UpNextContext>
  );
}

/**
 * The player and its notes, with the player's For testers (for-testers.tsx): the notes and tools for
 * testing, which the film filling the window hides (D418 §1), kept in ⁝'s For testers. A cast's
 * receiver has none, its notes in place.
 */
function PlayArea(props: { readonly isOwn: boolean; readonly children: ReactNode }): JSX.Element {
  const panel = useForTestersPanel();
  return (
    // A remote's first arrow starts on the video, not the trail above it (focus-navigation.ts).
    <div className="play" data-focus-container="" data-focus-start="">
      <ForTestersContext value={props.isOwn ? panel : undefined}>
        {props.children}
        <ForTestersHome />
      </ForTestersContext>
    </div>
  );
}

/**
 * The bare player's button where onBare is given, and the debug link's tools (D380). Casting is no
 * longer offered here: it is started from the status tray's Cast, and moves this film with it (D364
 * §1, §12).
 */
function UnderThePlayer(props: { readonly onBare: (() => void) | undefined }): JSX.Element {
  const { t } = useTranslation();
  const isFullScreen = useIsFullScreen();
  return (
    <>
      {props.onBare === undefined ? null : (
        <button type="button" hidden={isFullScreen} onClick={props.onBare}>
          {t("play.bare.open")}
        </button>
      )}
      <LinkDebugTools />
    </>
  );
}

/**
 * Keeps the player's stage filling the window while the page shows it, on every device, its
 * controls over the picture and nothing of the page under it (D418 §1): leaving the browser's full
 * screen, by ⤢, Escape or a Fire TV's Back the page never hears (D407 §7), leaves the film filling
 * the window, not the page under it. Back then hides the controls where they show, else leaves the
 * film, and has no third state.
 */
function useWholeWindow(video: HTMLVideoElement | null): void {
  useEffect(() => {
    const stage = video?.closest(".film-player__stage");
    return stage instanceof HTMLElement ? keepFillingTheWindow(stage) : undefined;
  }, [video]);
}

/**
 * Puts the player's stage full screen as it opens where the browser itself is full screen (F11, a
 * Mac's full-screen window), as a television's player does (the owner, 2026-10-25); where the
 * browser refuses, it fills the window (fillTheScreen).
 */
function useFullscreenAsTheBrowserIs(video: HTMLVideoElement | null): void {
  useEffect(() => {
    const stage = video?.closest(".film-player__stage");
    const isAsked = !isTelevision(navigator.userAgent) && isBrowserFullscreen(thisBrowserWindow());
    if (isAsked && video !== null && stage instanceof HTMLElement) {
      keepThePressForTheStart(video);
      fillTheScreen(stage, document);
    }
  }, [video]);
}

/**
 * How the Play page composes its player: the player's own controls on every device, never the
 * browser's (D369 §2), and playing at once unless it waits. A cast's receiver autoplays too: with
 * autoplay off (837747a5) a Fire TV's cast started worse, jumping back and stalling, and flashed black
 * (the owner, 2026-10-28: "It's much worse now. It jittered a bunch, and on the kid it
 * started flashing black when I first played"), so it was put back until the start is understood.
 */
function pageComposition(
  props: Parameters<typeof PlayView>[0],
  keptVideo: HTMLVideoElement | undefined,
): FilmComposition {
  // On a television the player's overlay, which a remote reaches, and full screen at once; on a
  // computer or a phone the player's bar, which the arrows and Tab reach as well as a mouse or a
  // finger: the browser's own were in a shadow the arrows could not reach (the owner, 2026-10-27).
  // A cast's receiver, on any device, has a television's controls alone, over the film filling the
  // window (D338 §5).
  const hasTelevisionControls = isTelevision(navigator.userAgent) || props.receiving !== undefined;
  return {
    hasControls: false,
    over: overTheFilm(hasTelevisionControls),
    isAutoPlay: props.isStartPaused !== true,
    isFailureHidden: props.isReconnecting === true,
    askAbout: props.fileId,
    onMediaBack: props.onMediaBack,
    under: PlayUnderOfPage,
    around: PlayTracksOfPage,
    // An iPhone's, readied by the press that opened the page (video-pressed-for-play.ts).
    keptVideo,
  };
}

/**
 * What each way of playing is given: the file, the decision, what follows a sound change or a
 * fetched subtitle, and where a lower quality a stalled player is offered goes (D247 §2).
 */
type PlayProps = Parameters<typeof PlayView>[0] & {
  readonly onLowerQuality: (offer: LadderOffer, positionMs: number) => void;
  /** Told the video element each way of playing shows, and null as it goes. */
  readonly onVideo: (video: HTMLVideoElement | null) => void;
  /** Told when even the lowest quality cannot keep up, and undefined when it can (D247 §8). */
  readonly onCannotKeepUp: (cannotKeepUp: CannotKeepUp | undefined) => void;
  /** Plays the film's own file in the bare player, where it is offered. */
  readonly onBare: (() => void) | undefined;
};

/**
 * A handler that asks how the file plays again, saying why, from where the video stands: the
 * server's answer starts where the person last left the file, by the resume thresholds, so a film
 * whose audio track was changed three minutes in started again from its beginning (a film on
 * staging, 2026-10-26).
 */
function decideAgainFor(
  props: {
    readonly onDecideAgain: DecideAgain;
    readonly videoRef: RefObject<HTMLVideoElement | null>;
    readonly decision: Pick<PlaybackDecision, "resumePositionMs">;
  },
  cause: DecideAgainCause,
): () => void {
  return () => {
    const video = props.videoRef.current;
    props.onDecideAgain(cause, {
      positionMs: positionToMoveFrom(video, props.decision.resumePositionMs),
      isPaused: video?.paused ?? false,
    });
  };
}

/**
 * Notes where this device's own film stands, for a cast started on the casting page to move it to
 * the receiver at its position (D364 §1; playing-here.ts).
 */
function useNotesWhereItPlays(
  isOwn: boolean,
  played: {
    readonly fileId: number;
    readonly video: HTMLVideoElement | null;
    readonly startPositionMs: number;
  },
): void {
  const { fileId, video, startPositionMs } = played;
  const positionNow = useCallback(
    () => positionToMoveFrom(video, startPositionMs),
    [video, startPositionMs],
  );
  const isPausedNow = useCallback(() => video?.paused === true, [video]);
  useNotesPlayingHere(isOwn, fileId, { positionMs: positionNow, isPaused: isPausedNow });
}

/**
 * Where a player moving to a lower quality goes on from: where its video is, once it has loaded
 * enough to say; before that, where it was asked to start. A video not loaded reads 0, and a host
 * resuming Bram at 51:22 was moved to its first minute, whose chunks no one had made (the owner,
 * 2026-10-08).
 */
export function positionToMoveFrom(
  video: Pick<HTMLVideoElement, "readyState" | "currentTime"> | null,
  startPositionMs: number,
): number {
  const haveMetadata = 1; // HTMLMediaElement.HAVE_METADATA, which a plain page may not define
  return video !== null && video.readyState >= haveMetadata
    ? video.currentTime * 1000
    : startPositionMs;
}

/**
 * Keeps the playback's play state on the server, and hands a lower quality the server offered a
 * stalled player on, with where the video is (D247 §2).
 */
function useReportedPlayState(
  props: PlayProps & {
    readonly videoRef: RefObject<HTMLVideoElement | null>;
    readonly readLinkKbps: () => number | undefined;
    readonly failure: string | undefined;
  },
): { readonly refusal: unknown; readonly stopMessage: string | undefined } {
  const { api } = useServices();
  const { onLowerQuality, videoRef } = props;
  const { refusal, stopMessage, lowerQuality } = usePlayState(api, videoRef, {
    mediaToken: props.decision.mediaToken,
    resumePositionMs: props.decision.resumePositionMs,
    readLinkKbps: props.readLinkKbps,
    onGone: props.onPlaybackGone,
    onOwnPictureGivenUp: props.onOwnPictureGivenUp,
    failure: props.failure,
  });
  const startPositionMs = props.decision.resumePositionMs;
  useEffect(() => {
    if (lowerQuality !== undefined) {
      onLowerQuality(lowerQuality, positionToMoveFrom(videoRef.current, startPositionMs));
    }
  }, [lowerQuality, onLowerQuality, videoRef, startPositionMs]);
  return { refusal, stopMessage };
}

/**
 * What sits under a playing video: "Start over", the subtitle menu and offset, finding subtitles,
 * the audio track, levelling and the sound's offset, the choices held in PlayerTracksContext. It
 * keeps the playback's play state on the server, starting where the person left the file.
 */
function UnderTheVideo(
  props: PlayProps & {
    readonly videoRef: RefObject<HTMLVideoElement | null>;
    /** The link's speed as the playback measures it, undefined until it can say (D247 §1). */
    readonly readLinkKbps: () => number | undefined;
    /** What failed in the player, in the viewer's words; undefined while nothing has. */
    readonly failure: string | undefined;
  },
): JSX.Element {
  const { t } = useTranslation();
  const tracks = usePlayerTracks();
  const { refusal: reportRefusal, stopMessage } = useReportedPlayState(props);
  const { onVideo, videoRef } = props;
  useEffect(() => {
    onVideo(videoRef.current);
    return () => {
      onVideo(null);
    };
  }, [onVideo, videoRef]);
  return (
    <>
      {stopMessage === undefined ? null : (
        <p role="alert">{t("play.stoppedByAdmin", { message: stopMessage })}</p>
      )}
      {reportRefusal === undefined || stopMessage !== undefined ? null : (
        <p role="alert">{t("play.positionNotKept", { reason: problemText(t, reportRefusal) })}</p>
      )}
      <OverlayHeld>
        <StartOver videoRef={props.videoRef} />
        <SubtitleControls choice={tracks.subtitles} />
        <FindSubtitles fileId={props.fileId} onFetched={decideAgainFor(props, "subtitleFetched")} />
        <AudioChoiceMenu choice={tracks.audio} />
        <LevellingControl
          level={tracks.level}
          measuring={tracks.measuring}
          state={tracks.levelling}
        />
        <SoundOffsetControl videoRef={props.videoRef} initialMs={props.decision.soundOffsetMs} />
      </OverlayHeld>
    </>
  );
}

/**
 * What the full-screen overlay's menus hold (player-menus.tsx), under the film: hidden while full
 * screen draws the overlay, which holds them then (D407 §8), kept mounted so their state stays.
 */
function OverlayHeld(props: { readonly children: ReactNode }): JSX.Element {
  return (
    <div className="play__overlay-held" hidden={useIsFullScreen()}>
      {props.children}
    </div>
  );
}

/** What the Play page gives what it draws under the film: PlayView's props and its ways. */
const PlayContext = createContext<PlayProps | undefined>(undefined);

/**
 * The player's Under piece: its controls under the film outside full screen (D407 §9), then
 * PlayUnder with the page's props from PlayContext.
 */
function PlayUnderOfPage(props: FilmParts): JSX.Element {
  const ways = use(PlayContext);
  return (
    <>
      <ControlsUnderTheFilm {...props} />
      {ways === undefined ? null : <PlayUnder {...ways} {...props} />}
    </>
  );
}

/**
 * The film's tracks for what the Play page draws over and under it, chosen as its way of playing
 * plays them (player-tracks.tsx), with the page's ways from PlayContext: another track or the boost
 * that cannot play in place asks for the film again from where the video stands.
 */
function PlayTracksOfPage(props: FilmParts & { readonly children: ReactNode }): JSX.Element {
  const ways = use(PlayContext);
  const { ladder, videoRef, children } = props;
  if (ways === undefined) {
    return <>{children}</>;
  }
  const film = { fileId: ways.fileId, decision: ways.decision, videoRef };
  if (ladder === undefined) {
    const asked = { ...ways, videoRef };
    const askAgain = {
      audio: decideAgainFor(asked, "audioChosen"),
      boost: decideAgainFor(asked, "boostChosen"),
      raise: decideAgainFor(asked, "raiseWanted"),
    };
    return (
      <OwnFileTracks {...film} askAgain={askAgain}>
        <MenusOfPage asked={asked} isOnLadder={false}>
          {children}
        </MenusOfPage>
      </OwnFileTracks>
    );
  }
  const decision = { ...ways.decision, resumePositionMs: Math.round(ladder.positionMs) };
  const asked = { ...ways, videoRef, decision };
  return (
    <LadderTracks {...film} ladder={ladder} askAgain={decideAgainFor(asked, "audioChosen")}>
      <MenusOfPage asked={asked} isOnLadder>
        {children}
      </MenusOfPage>
    </LadderTracks>
  );
}

/**
 * Gives the overlay's menus the page's ways (menusWaysOf), around children, with how the film plays
 * here for ⁝'s How it plays (D444): converted on its ladder, else its file as it is (deliveryOf).
 */
function MenusOfPage(props: {
  readonly asked: PlayProps & { readonly videoRef: RefObject<HTMLVideoElement | null> };
  readonly isOnLadder: boolean;
  readonly children: ReactNode;
}): JSX.Element {
  const { asked, isOnLadder } = props;
  const playback = usePlaybackHere(asked.videoRef, isOnLadder ? "converted" : "direct");
  const ways = useMemo(
    () => ({ ...menusWaysOf(asked, isOnLadder), playback }),
    [asked, isOnLadder, playback],
  );
  return <PlayerMenusContext value={ways}>{props.children}</PlayerMenusContext>;
}

/**
 * What the overlay's menus are given by the Play page (player-menus.tsx): the file, the device's
 * sound timing, and what a subtitle fetched, the original chosen (on a ladder) and the bare player
 * ask for, each as the page under the film asks for it.
 */
function menusWaysOf(
  asked: PlayProps & { readonly videoRef: RefObject<HTMLVideoElement | null> },
  isOnLadder: boolean,
): PlayerMenusWays {
  return {
    fileId: asked.fileId,
    soundOffsetMs: asked.decision.soundOffsetMs,
    onSubtitleFetched: decideAgainFor(asked, "subtitleFetched"),
    onOriginalChosen: isOnLadder ? decideAgainFor(asked, "originalChosen") : undefined,
    onBare: asked.onBare,
  };
}

/**
 * What the Play page draws under the film, by the way it plays: what sits under every video, the
 * dialogue boost's switch where the sound offers it, and on a ladder the original's offer, and word
 * that it cannot keep up, which a host's session tells everyone (D247 §8).
 */
function PlayUnder(props: PlayProps & FilmParts): JSX.Element {
  const { boost } = usePlayerTracks();
  const { ladder } = props;
  const decision =
    ladder === undefined
      ? props.decision
      : { ...props.decision, resumePositionMs: Math.round(ladder.positionMs) };
  return (
    <>
      <UnderTheVideo {...props} decision={decision} />
      <OverlayHeld>
        {boost === undefined ? null : <BoostBox state={boost} />}
        {ladder === undefined ? null : (
          <LadderUnder {...props} ladder={ladder} decision={decision} />
        )}
      </OverlayHeld>
    </>
  );
}

/**
 * Under the film on its ladder: the original's offer, and its lowest quality's word that it cannot
 * keep up, which goes to a host's session (D247 §8).
 */
function LadderUnder(
  props: PlayProps &
    FilmParts & {
      readonly ladder: NonNullable<FilmParts["ladder"]>;
    },
): JSX.Element {
  const { ladder, onCannotKeepUp } = props;
  useEffect(() => {
    onCannotKeepUp(ladder.cannotKeepUp);
    return () => {
      onCannotKeepUp(undefined);
    };
  }, [ladder.cannotKeepUp, onCannotKeepUp]);
  const playOriginal = decideAgainFor(props, "originalChosen");
  // A cast's remote offers the original too, carried out here as this offer's press (D440 §5).
  useOriginalOfferedToARemote(playOriginal);
  return <PlayOriginal onChosen={playOriginal} />;
}

/**
 * The player's screen for one file, playing at once: nothing is asked of how the device makes sound
 * (D336 §4). A playback the server lost, as when it restarted, is opened again where it was, playing
 * or paused as it was, while the server answers within about a minute (askUntilAnswered); one lost
 * again within reopenAtMostEveryMs is told. On a device that casts, the film goes on to its receiver
 * instead (D364 §3), unless shouldPlayHere, the page staying where it is, its video's area the
 * cast's remote, until the cast is stopped and the film plays here again (D384 §2). Given atMs, it
 * starts there in place of where the person last left the file: a cast's film played here from where
 * its receiver stood. For a signed-in guest in a Watch Together session that plays the film, the page
 * is the group's film, followed (WhoseFilm): the one page every film plays on (D384 §2).
 */
export function Play(props: {
  readonly fileId: number;
  readonly atMs?: number | undefined;
  /** Whether the film waits paused at atMs, as a cast's receiver had it (D385 §2). */
  readonly isPaused?: boolean | undefined;
  readonly shouldPlayHere?: boolean | undefined;
}): ReactNode {
  return (
    <WhoseFilm
      fileId={props.fileId}
      own={
        <PlayOrSendOn
          fileId={props.fileId}
          atMs={props.atMs}
          isPaused={props.isPaused}
          shouldPlayHere={props.shouldPlayHere === true}
          playHere={(from) => <PlayHere key={from.round} fileId={props.fileId} from={from} />}
        />
      }
    />
  );
}

/** The Play page playing its film on this device, from where it is given to (from). */
function PlayHere(props: { readonly fileId: number; readonly from: PlayedHereFrom }): JSX.Element {
  const { t } = useTranslation();
  const receiving = useReceiving(props.fileId);
  const { opened, held, decideAgain, reopenPlayback, startAtTheTwin, reopenWhenBack } =
    useOpenedPlayback(props.fileId, firstAskOf(receiving, props.from));
  const render = ({ decision, isStartPaused }: Opened): JSX.Element => (
    <PlayView
      key={decision.mediaToken}
      fileId={props.fileId}
      decision={decision}
      onDecideAgain={decideAgain}
      onPlaybackGone={reopenPlayback}
      onMediaBack={reopenWhenBack}
      onOwnPictureGivenUp={startAtTheTwin}
      isStartPaused={isStartPaused === true}
      isReconnecting={opened.state !== "loaded"}
      receiving={receiving}
    />
  );
  // While a lost playback is opened again, the one lost stays on screen, playing what it holds (the
  // owner's test, 2026-10-18: "the page should hold the video open if it can while it tries to
  // reconnect").
  const shown = shownOf(opened, held);
  if (shown === undefined) {
    return <Loaded resource={opened} render={render} />;
  }
  return (
    <>
      {opened.state === "loaded" ? null : <output>{t("play.reopening")}</output>}
      {render(shown)}
    </>
  );
}

/**
 * Where the Play page's first playback starts in place of the resume position, and why it is asked
 * for: a cast received, from where its sender stood; a cast's film played here again, from where its
 * receiver stood (a Play here mid-film played from 0:00, staging 2026-10-27), paused there where the
 * receiver had it paused (D385 §2); else nowhere given.
 */
export function firstAskOf(
  receiving: Receiving | undefined,
  from: PlayedHereFrom,
): FirstAsk | undefined {
  if (receiving !== undefined) {
    const { positionMs, isPaused } = receiving.cast;
    return positionMs === undefined
      ? undefined
      : { cause: "playPage", atMs: positionMs, isPaused: isPaused === true };
  }
  const { atMs, isPaused } = from;
  return atMs === undefined ? undefined : { cause: "castOver", atMs, isPaused: isPaused === true };
}
