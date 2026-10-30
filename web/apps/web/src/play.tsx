// SPDX-License-Identifier: MPL-2.0

import "./play.css";

import { seekTo } from "@orpheum/player";
import type { CannotKeepUp } from "@orpheum/sdk";
import {
  createContext,
  use,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type JSX,
  type ReactNode,
  type RefObject,
} from "react";
import { useTranslation } from "react-i18next";

import { PlayTop } from "./play-up.tsx";
import { AudioMenu, startingAudioTrack, type AudioTrack } from "./audio-tracks.tsx";
import {
  FilmPlayer,
  ladderAudioRenditionIndex,
  type FilmComposition,
  type FilmParts,
  deliveryOf,
  type MovedDown,
} from "./film-player.tsx";
import { problemText } from "./problem.ts";
import { reopenAtMostEveryMs, type ReopenAt } from "./reopen.ts";
import type { LadderOffer } from "./moving-down.ts";
import { usePlayState } from "./play-state.ts";
import { usePlayerCommands } from "./player-commands.ts";
import { fillTheScreen } from "./film-fullscreen.ts";
import { isBrowserFullscreen, isTelevision, thisBrowserWindow } from "./tv-device.ts";
import { useServices } from "./services.tsx";
import { BarePlayer } from "./bare-player.tsx";
import { PlayOriginal } from "./play-original.tsx";
import { FrameTimingNote } from "./frame-timing-note.tsx";
import { BoostSwitch } from "./sound.tsx";
import { SoundOffsetControl } from "./sound-delay.tsx";
import { FindSubtitles } from "./subtitle-search.tsx";
import { Subtitles } from "./subtitles.tsx";
import { LevelSwitch, soundLevelOf, type SoundLevel } from "./levelling.tsx";
import { newSoundPanel, SoundPanelContext } from "./sound-panel.ts";
import { Loaded } from "./title.tsx";
import { PlaysAlong, useReceiving, type Receiving } from "./cast-receiver.tsx";
import { PlayOrSendOn } from "./cast-to.tsx";
import { useNotesPlayingHere } from "./playing-here.ts";
import { openPlayback, type Opened, type Reopening } from "./play-start-timing.ts";
import { useResource, type Resource } from "./use-resource.ts";
import { overTheFilm, UpNextContext, useUpNextOffered } from "./up-next.tsx";
import { watchingOf, type PlaybackDecision, type StartCause } from "./watching.ts";

/** Why a playing page asks how the file plays again. */
type DecideAgainCause = Extract<
  StartCause,
  "subtitleFetched" | "audioChosen" | "originalChosen" | "boostChosen"
>;

/**
 * Asks the server again how the file plays, saying why, and, given at, from where the video stood
 * then rather than where the server last heard it was.
 */
type DecideAgain = (cause: DecideAgainCause, at?: ReopenAt) => void;

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
  const { decision } = props;
  const isOwnFile = decision.delivery === "direct" && decision.onLadder === undefined;
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
  const onBare = (): void => {
    setIsBare(true);
  };
  return (
    <SoundPanelContext value={soundPanel}>
      <PlayerView {...props} onBare={isOwnFile ? onBare : undefined} />
    </SoundPanelContext>
  );
}

/** PlayView's player, and the bare player's button where onBare is given. */
function PlayerView(
  props: PlayViewProps & { readonly onBare: (() => void) | undefined },
): JSX.Element {
  // The lower quality a stalled player moved to, and where it was (D247 §2); per playback.
  const [moved, setMoved] = useState<MovedDown | undefined>(undefined);
  // The video playing now, which the host's Watch Together session follows: kept here, above the
  // way of playing, so a move to a lower quality is no film stopped and put on again for everyone
  // (the owner's test, 2026-10-05).
  const [video, setVideo] = useState<HTMLVideoElement | null>(null);
  // Whether its lowest quality cannot keep up, which a host's session tells everyone (D247 §8).
  const [cannotKeepUp, setCannotKeepUp] = useState<CannotKeepUp | undefined>(undefined);
  usePlayerCommands(video);
  useFullscreenAsTheBrowserIs(video);
  const { mediaToken } = props.decision;
  const onLowerQuality = useCallback(
    (offer: LadderOffer, positionMs: number) => {
      setMoved((already) => already ?? { mediaToken, offer, positionMs });
    },
    [mediaToken],
  );
  const { fileId, decision, onDecideAgain, onPlaybackGone, onOwnPictureGivenUp } = props;
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
    }),
    [fileId, decision, onDecideAgain, onPlaybackGone, onOwnPictureGivenUp, onLowerQuality],
  );
  const { titleName, resumePositionMs } = props.decision;
  const { receiving } = props;
  // A cast's receiver shows the film and nothing else (D337 §3).
  const isOwn = receiving === undefined;
  useNotesWhereItPlays(isOwn, { fileId, video, startPositionMs: resumePositionMs });
  const offered = useUpNextOffered(fileId, video, { ...decision.preferences, receiving });
  return (
    <UpNextContext value={offered}>
      <PlayTop plays={decision.plays} titleName={titleName} isOwn={isOwn}>
        <PlayContext value={ways}>
          {/* A remote's first arrow starts on the video, not the trail above it (focus-navigation.ts). */}
          <div className="play" data-focus-container="" data-focus-start="">
            <FilmPlayer
              decision={props.decision}
              moved={moved}
              ladderStartMs={resumePositionMs}
              composition={pageComposition(props)}
            />
            <FrameTimingNote video={video} />
            {isOwn ? <UnderThePlayer onBare={props.onBare} /> : null}
          </div>
        </PlayContext>
      </PlayTop>
      <PlaysAlong
        {...{ receiving, fileId, video, cannotKeepUp }}
        delivery={deliveryOf(decision, moved)}
        startPositionMs={resumePositionMs}
        hasNext={offered.upNext !== undefined && offered.upNext.episode.unavailable === undefined}
      />
    </UpNextContext>
  );
}

/**
 * The bare player's button where onBare is given. Casting is no longer offered here: it is started
 * from the status tray's Cast, and moves this film with it (D364 §1, §12).
 */
function UnderThePlayer(props: { readonly onBare: (() => void) | undefined }): JSX.Element | null {
  const { t } = useTranslation();
  return props.onBare === undefined ? null : (
    <button type="button" onClick={props.onBare}>
      {t("play.bare.open")}
    </button>
  );
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
    if (isAsked && stage instanceof HTMLElement) {
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
function pageComposition(props: Parameters<typeof PlayView>[0]): FilmComposition {
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
};

/**
 * A handler that asks how the file plays again, saying why, from where the video stands: the
 * server's answer starts where the person last left the file, by the resume thresholds, so a film
 * whose audio track was changed three minutes in started again from its beginning (The Unknown on
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
  useNotesPlayingHere(isOwn, fileId, positionNow);
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
 * What sits under a playing video: "Start over", the subtitle menu and offset for the decision's
 * tracks, finding subtitles, the audio track, levelling and the sound's offset. It keeps the
 * playback's play state on the server, starting where the person left the file. A chosen audio
 * track is kept for the person, and onAudioChosen() plays it.
 */
function UnderTheVideo(
  props: PlayProps & {
    readonly videoRef: RefObject<HTMLVideoElement | null>;
    readonly onAudioChosen: (track: AudioTrack) => void;
    /** The sound playing now, which levelling levels. */
    readonly level: SoundLevel;
    /** The link's speed as the playback measures it, undefined until it can say (D247 §1). */
    readonly readLinkKbps: () => number | undefined;
    /** What failed in the player, in the viewer's words; undefined while nothing has. */
    readonly failure: string | undefined;
  },
): JSX.Element {
  const { t } = useTranslation();
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
      <StartOver videoRef={props.videoRef} />
      <Subtitles
        fileId={props.fileId}
        tracks={props.decision.subtitles ?? []}
        preferences={props.decision.preferences}
        videoRef={props.videoRef}
      />
      <FindSubtitles fileId={props.fileId} onFetched={decideAgainFor(props, "subtitleFetched")} />
      <AudioMenu fileId={props.fileId} decision={props.decision} onChosen={props.onAudioChosen} />
      <LevelSwitch
        fileId={props.fileId}
        level={props.level}
        isInitiallyOn={props.decision.preferences.isLevellingOn}
        videoRef={props.videoRef}
      />
      <SoundOffsetControl videoRef={props.videoRef} initialMs={props.decision.soundOffsetMs} />
    </>
  );
}

/** The button that takes the film back to its start. */
function StartOver(props: { readonly videoRef: RefObject<HTMLVideoElement | null> }): JSX.Element {
  const { t } = useTranslation();
  return (
    <button
      type="button"
      className="play__start-over"
      onClick={() => {
        const video = props.videoRef.current;
        if (video !== null) {
          seekTo(video, 0);
        }
      }}
    >
      {t("play.startOver")}
    </button>
  );
}

/** What the Play page gives what it draws under the film: PlayView's props and its ways. */
const PlayContext = createContext<PlayProps | undefined>(undefined);

/** PlayUnder, with the page's props from PlayContext: the player's Under piece. */
function PlayUnderOfPage(props: FilmParts): JSX.Element | null {
  const ways = use(PlayContext);
  return ways === undefined ? null : <PlayUnder {...ways} {...props} />;
}

/**
 * What the Play page draws under the film, by the way it plays: what sits under every video, and
 * the original's reason, or a ladder's audio tracks, dialogue boost and word that it cannot keep up, which a host's session tells everyone (D247 §8).
 */
function PlayUnder(props: PlayProps & FilmParts): JSX.Element {
  if (props.ladder !== undefined) {
    return <LadderUnder {...props} ladder={props.ladder} />;
  }
  return <DirectUnder {...props} />;
}

/**
 * Under the original: what sits under every video; its tracks say it plays as it is. Its sound is the
 * film's own, which the device mixes down itself, so the dialogue boost, where the track has a
 * centre to raise, starts off here: turned on, it is kept, then the film is asked for again from
 * where the video stands, and the server answers with its ladder carrying the boosted mix (D336 §1,
 * §3).
 */
function DirectUnder(props: PlayProps & FilmParts): JSX.Element {
  const boostChosen = decideAgainFor(props, "boostChosen");
  return (
    <>
      <UnderTheVideo
        {...props}
        level={soundLevelOf(props.decision)}
        onAudioChosen={decideAgainFor(props, "audioChosen")}
      />
      {hasACentre(props.decision) ? (
        <BoostSwitch
          isInitiallyOn={false}
          onSwitched={() => undefined}
          onKept={(isOn) => {
            if (isOn) {
              boostChosen();
            }
          }}
        />
      ) : null}
    </>
  );
}

/**
 * Whether the track the original plays has a centre for the dialogue boost to raise: more than two
 * channels, as the server's boosted mix needs (D222, D336 §2).
 */
export function hasACentre(decision: PlaybackDecision): boolean {
  const index = startingAudioTrack(decision)?.audioTrackIndex ?? 0;
  const sound = (decision.tracks.fileSounds ?? []).find((each) => each.audioTrackIndex === index);
  return sound !== undefined && sound.channelCount > 2;
}

/**
 * Under the film on its ladder: what sits under every video, its play state kept from where it
 * started, not where the person last left the file; its sound carries its levelling; and a switch
 * for the dialogue boost where the track playing has a boosted mix (D217 §3, D222), which changes the ladder's sound as soon as the other mix has loaded at the playhead, playing on meanwhile. The boosted mix carries
 * its track's gain, as the standard one does (D226 §3), so the level stays. Its lowest quality's word
 * that it cannot keep up goes to a host's session (D247 §8).
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
  const decision = { ...props.decision, resumePositionMs: Math.round(ladder.positionMs) };
  const level = ladderSoundLevel(props.decision, ladder.offer.soundCarriedGainDb);
  const { sound, playSound } = ladder;
  const canBoost =
    sound !== undefined &&
    ladderAudioRenditionIndex(props.decision.tracks, { ...sound, isBoosted: true }) !== undefined;
  return (
    <>
      <UnderTheVideo
        {...props}
        decision={decision}
        level={level}
        // Asked for again, not switched in place as the boost is: the offer's carried gain and the
        // decision's levelling are the starting track's alone, so another would play at the wrong level.
        onAudioChosen={decideAgainFor({ ...props, decision }, "audioChosen")}
      />
      {canBoost ? (
        <BoostSwitch
          isInitiallyOn={props.decision.preferences.isDialogueBoostOn}
          onSwitched={(isOn) => {
            playSound({ audioTrackIndex: sound.audioTrackIndex, isBoosted: isOn });
          }}
        />
      ) : null}
      <PlayOriginal onChosen={decideAgainFor({ ...props, decision }, "originalChosen")} />
    </>
  );
}

/**
 * The level of a ladder's sound: the gain its chunks carry, levelled; or, for a sound that carries
 * none, the film's own levelling from the decision, so a person who plays without levelling, whose
 * sound is offered unlevelled, is told it plays as mixed and can turn levelling on (D226 §4).
 */
export function ladderSoundLevel(decision: PlaybackDecision, carriedGainDb: number): SoundLevel {
  return carriedGainDb === 0
    ? { levelling: decision.levelling, carriedGainDb: 0 }
    : { levelling: { isMeasured: true, gainDb: carriedGainDb }, carriedGainDb };
}

/**
 * The player's screen for one file, playing at once: nothing is asked of how the device makes sound
 * (D336 §4). A playback the server lost, as when it restarted, is opened again where it was, playing
 * or paused as it was, while the server answers within about a minute (askUntilAnswered); one lost
 * again within reopenAtMostEveryMs is told. On a device that casts, the film goes on to its receiver
 * instead (D364 §3), unless shouldPlayHere: the remote's "Play on this device instead" (§4). Given atMs,
 * it starts there in place of where the person last left the file: a cast's remote playing here from
 * where its receiver stood.
 */
export function Play(props: {
  readonly fileId: number;
  readonly atMs?: number | undefined;
  readonly shouldPlayHere?: boolean | undefined;
}): ReactNode {
  return (
    <PlayOrSendOn
      fileId={props.fileId}
      atMs={props.atMs}
      shouldPlayHere={props.shouldPlayHere === true}
    >
      <PlayHere fileId={props.fileId} atMs={props.atMs} />
    </PlayOrSendOn>
  );
}

/** The Play page playing its film on this device. */
function PlayHere(props: {
  readonly fileId: number;
  readonly atMs: number | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  const receiving = useReceiving(props.fileId);
  const { opened, held, decideAgain, reopenPlayback, startAtTheTwin, reopenWhenBack } =
    useOpenedPlayback(props.fileId, firstAskOf(receiving, props.atMs));
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
 * for: a cast received, from where its sender stood; a remote's Play here from, from where its
 * receiver stood (a Play here mid-film played from 0:00, staging 2026-10-27); else nowhere given.
 */
function firstAskOf(
  receiving: Receiving | undefined,
  atMs: number | undefined,
): FirstAsk | undefined {
  if (receiving !== undefined) {
    const { positionMs } = receiving.cast;
    return positionMs === undefined ? undefined : { cause: "playPage", atMs: positionMs };
  }
  return atMs === undefined ? undefined : { cause: "castOver", atMs };
}

/** Where the Play page's first playback starts, and why it is asked for. */
interface FirstAsk {
  readonly cause: StartCause;
  readonly atMs: number;
}

/** What the Play page shows: its decision, or, while the next is asked for, the one held. */
function shownOf(opened: Resource<Opened>, held: Opened | undefined): Opened | undefined {
  if (opened.state === "loaded") {
    return opened.value;
  }
  return opened.state === "loading" ? held : undefined;
}

/**
 * How the Play page opens its playback again: a lost one at most every reopenAtMostEveryMs, saying
 * whether it does, and one whose player gave the film's own picture up from where the video stood,
 * asked for again at the twin (D323 §2). What played before is held on screen meanwhile.
 */
function useReopening(parts: {
  readonly reopenRef: RefObject<Reopening | undefined>;
  readonly reload: () => void;
  readonly hold: () => void;
}): {
  readonly reopenPlayback: (whereNow: () => ReopenAt) => boolean;
  readonly startAtTheTwin: (at: ReopenAt) => void;
  readonly reopenWhenBack: (at: ReopenAt) => void;
} {
  const { reopenRef, reload, hold } = parts;
  // When the last lost one was opened again, on the page's clock.
  const lastReopenMsRef = useRef<number | undefined>(undefined);
  const reopen = useCallback(
    (asked: Reopening): void => {
      reopenRef.current = asked;
      hold();
      reload();
    },
    [reopenRef, reload, hold],
  );
  const reopenPlayback = useCallback(
    (whereNow: () => ReopenAt): boolean => {
      const nowMs = performance.now();
      const lastMs = lastReopenMsRef.current;
      if (lastMs !== undefined && nowMs - lastMs < reopenAtMostEveryMs) {
        return false;
      }
      lastReopenMsRef.current = nowMs;
      reopen({ whereNow, cause: "playbackGone" });
      return true;
    },
    [reopen],
  );
  const startAtTheTwin = useCallback(
    (at: ReopenAt): void => {
      reopen({ whereNow: () => at, cause: "ownPictureGivenUp" });
    },
    [reopen],
  );
  const reopenWhenBack = useCallback(
    (at: ReopenAt): void => {
      reopen({ whereNow: () => at, cause: "mediaBack" });
    },
    [reopen],
  );
  return { reopenPlayback, startAtTheTwin, reopenWhenBack };
}

/**
 * The Play page's decision for the file, asked for again when decideAgain says why, and opened again
 * where a playback the server lost is when its new one comes, once reopenPlayback is told so, which
 * says whether it is: not within reopenAtMostEveryMs of the last. held is what played before, shown
 * while the new one is asked for.
 */
function useOpenedPlayback(
  fileId: number,
  first: FirstAsk | undefined,
): {
  readonly opened: Resource<Opened>;
  readonly held: Opened | undefined;
  readonly decideAgain: DecideAgain;
  readonly reopenPlayback: (whereNow: () => ReopenAt) => boolean;
  readonly startAtTheTwin: (at: ReopenAt) => void;
  readonly reopenWhenBack: (at: ReopenAt) => void;
} {
  const { api, playableFormats } = useServices();
  // Why the next decision is asked for: the page's opening, unless something asked it again; and
  // where the video stood when it did, for the next to start from, when it said.
  const nextCauseRef = useRef<StartCause>(first?.cause ?? "playPage");
  const nextAtRef = useRef<ReopenAt | undefined>(
    first === undefined ? undefined : { positionMs: first.atMs, isPaused: false },
  );
  // Where a playback opened again is, read as its new one comes, for that to start from, and why it
  // is: the server lost it, or its player gave the film's own picture up; and when the last lost one
  // was opened again, on the page's clock.
  const reopenRef = useRef<Reopening | undefined>(undefined);
  const [held, setHeld] = useState<Opened | undefined>(undefined);
  const [opened, reload] = useResource(`play/${String(fileId)}`, async (): Promise<Opened> => {
    const cause = nextCauseRef.current;
    const askedAt = nextAtRef.current;
    nextCauseRef.current = "playPage";
    nextAtRef.current = undefined;
    const reopen = reopenRef.current;
    reopenRef.current = undefined;
    if (reopen === undefined) {
      setHeld(undefined);
    }
    return await openPlayback({
      watching: watchingOf(api),
      fileId,
      playableFormats: playableFormats(),
      cause,
      askedAt,
      reopen,
    });
  });
  const decideAgain = useCallback(
    (cause: DecideAgainCause, at?: ReopenAt) => {
      nextCauseRef.current = cause;
      nextAtRef.current = at;
      reload();
    },
    [reload],
  );
  const { reopenPlayback, startAtTheTwin, reopenWhenBack } = useReopening({
    reopenRef,
    reload,
    hold: () => {
      setHeld(opened.state === "loaded" ? opened.value : undefined);
    },
  });
  return { opened, held, decideAgain, reopenPlayback, startAtTheTwin, reopenWhenBack };
}
