// SPDX-License-Identifier: MPL-2.0

import "./film-player.css";

import {
  keepScreenAwake,
  pauseWhenHidden,
  mediaErrorFailure,
  meterBufferGrowth,
  shapeChunkLoads,
  type LinkMeter,
  type PlaybackFailed,
  type PlaybackFailure,
  type PlayedLevel,
  type SoundChoiceCause,
} from "@orpheum/player";
import { healPageAudio, startGateOf, type CannotKeepUp, type PlaybackReport } from "@orpheum/sdk";
import type { TFunction } from "i18next";
import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type ComponentType,
  type JSX,
  type ReactNode,
  type RefObject,
} from "react";
import { useTranslation } from "react-i18next";

import { startingAudioTrack } from "./audio-tracks.tsx";
import {
  chooseSoundInPlace,
  ladderAudioRenditionIndex,
  ladderSoundLevelOf,
  ladderStartingAudio,
} from "./ladder-sound.ts";
import { ForTesters } from "./for-testers.tsx";
import type { LadderOffer } from "./moving-down.ts";
import { isThisHandheld } from "./handheld.ts";
import { useChunkedPlayback } from "./ladder-playback.ts";
import { rateText } from "./rate-text.ts";
import type { ReopenAt } from "./reopen.ts";
import { usePlaybackFailure } from "./playback-failure.ts";
import { useServices } from "./services.tsx";
import { noteIfStartRefused } from "./start-refused.ts";
import { playingPicture, TracksNote, type PlayingSound } from "./tracks-note.tsx";
import { monotonicMs } from "./use-together.ts";
import { isStartThePages, playsOwnFile, type PlaybackDecision } from "./watching.ts";
import type { SoundLevel } from "./levelling.tsx";

/**
 * What a page knows of the film playing, given to what it composes around the video: the video, the
 * link's speed as the playback measures it, and the way it plays: a chunked one's quality and
 * where it started; none for the original.
 */
export interface FilmParts {
  readonly videoRef: RefObject<HTMLVideoElement | null>;
  /** The link's speed as the playback measures it, undefined until it can say (D247 §1). */
  readonly readLinkKbps: () => number | undefined;
  /** What failed, in the viewer's words; undefined while nothing has. */
  readonly failure: string | undefined;
  readonly ladder?:
    | {
        readonly offer: LadderOffer;
        /** Where on the film it started, in ms. */
        readonly positionMs: number;
        readonly playing: PlayedLevel | undefined;
        /** Even its lowest quality's word that it cannot keep up (D247 §8). */
        readonly cannotKeepUp: CannotKeepUp | undefined;
        /**
         * The sound playing, on a film's ladder whose sounds its tracks name; undefined on a ladder a
         * stalled player moved to, whose sounds no decision named.
         */
        readonly sound: PlayingSound | undefined;
        /**
         * The level of the sound heard, its own track's as the decision's tracks say it, changed
         * once a sound switched to in place is heard, never as it is chosen; undefined on a ladder a
         * stalled player moved to, whose sounds no decision named.
         */
        readonly heardLevel: SoundLevel | undefined;
        /**
         * Plays another of the ladder's sounds, for cause, the boost turned on or off or another
         * track chosen, as the next of its playbacks starts with: for the boost the sound loaded plays
         * on until the other's chunk at the playhead has loaded, and another track cuts in at once
         * (keepAudioRendition, @orpheum/player; D400). Returns
         * whether it switches in place: not where the ladder names no such sound, or its player is
         * the element's own HLS, which chooses its own tracks; the page then asks again.
         */
        readonly playSound: (sound: PlayingSound, cause: SoundChoiceCause) => boolean;
      }
    | undefined;
}

/** What a page draws over or under the video: a component, given the film's parts as its props. */
export type FilmPiece = ComponentType<FilmParts>;

/**
 * What a page holds around the stage and what it draws under it, given the film's parts and those
 * pieces as children: what the pieces over and under the video read, as a context.
 */
export type FilmSurround = ComponentType<FilmParts & { readonly children: ReactNode }>;

/**
 * How a page composes the player: whether the video shows its own controls and plays at once, which
 * file a failure is explained by asking the server of (undefined asks nothing, for a Watch Together
 * guest, who may not ask of the host's files), and what it draws over the video, inside its stage,
 * and under it. Each piece is a component of the page's own, stable across renders, which reads
 * what else it needs from the page's context. A page that keeps one video for every film gives it;
 * else each film makes its own.
 */
export interface FilmComposition {
  readonly hasControls: boolean;
  readonly isAutoPlay: boolean;
  readonly askAbout: number | undefined;
  /** Told, with where the video stood, that its film can be read again after it could not. */
  readonly onMediaBack?: ((at: ReopenAt) => void) | undefined;
  readonly over?: FilmPiece;
  readonly under?: FilmPiece;
  /** What holds the stage and what is under it, for both pieces to read; nothing when undefined. */
  readonly around?: FilmSurround;
  readonly keptVideo?: HTMLVideoElement | undefined;
  /** Whether what fails is kept from the viewer, as while the page opens a lost playback again. */
  readonly isFailureHidden?: boolean;
}

/** Where a stalled player moved to a lower quality (D247 §2), for its playback alone. */
export interface MovedDown {
  readonly mediaToken: string;
  readonly offer: LadderOffer;
  readonly positionMs: number;
}

/**
 * How the film is given to the player, as a cast's remote shows it (D371 §4): the file as it is, or
 * converted, its ladder, which a player that moved to a lower quality plays too.
 */
export function deliveryOf(
  decision: PlaybackDecision,
  moved: MovedDown | undefined,
): PlaybackReport["delivery"] {
  return playsOwnFile(decision) && moved === undefined ? "direct" : "converted";
}

/**
 * The film, playing as the server decided, the one player every page composes (the owner,
 * 2026-10-11: "The play view and controls should ideally be reusable and composable"): on the
 * ladder a stalled player moved to (D247 §2), on its film's ladder from ladderStartMs (D260), or the
 * original. Each owns the video and its stage, and says why it cannot play.
 */
export function FilmPlayer(props: {
  readonly decision: PlaybackDecision;
  readonly moved: MovedDown | undefined;
  /** Where a film on its ladder starts, in ms. */
  readonly ladderStartMs: number;
  readonly composition: FilmComposition;
}): JSX.Element {
  const { decision, moved, composition } = props;
  const onLadder = decision.onLadder ?? undefined;
  // AirPlay is the person's own player's, the one with controls (D473), never a film page's hidden one.
  const airPlayToken = composition.hasControls ? decision.mediaToken : undefined;
  if (moved?.mediaToken === decision.mediaToken) {
    return (
      <ChunkedFilm
        offer={moved.offer}
        positionMs={moved.positionMs}
        composition={composition}
        mediaToken={airPlayToken}
      />
    );
  }
  const sound = playingSoundOf(decision);
  if (onLadder !== undefined) {
    return (
      <ChunkedFilm
        offer={onLadder}
        positionMs={props.ladderStartMs}
        composition={composition}
        mediaToken={airPlayToken}
        isOnLadder
        tracks={decision.tracks}
        sound={sound}
      />
    );
  }
  if (decision.delivery !== "direct") {
    return <NotConverted />;
  }
  return (
    <>
      <DirectFilm decision={decision} composition={composition} />
      <ForTesters>
        <TracksNote tracks={decision.tracks} sound={sound} />
      </ForTesters>
    </>
  );
}

/**
 * A film its device cannot play as it is that came with no ladder to play it from: the server
 * converts a film only on its ladder (D344 §1), so this says it cannot be converted.
 */
function NotConverted(): JSX.Element {
  const { t } = useTranslation();
  return <p role="alert">{t("play.cannotConvert")}</p>;
}

/** The sound a decision starts with: its starting track, its boosted mix when the boost is on. */
function playingSoundOf(decision: PlaybackDecision): PlayingSound {
  return {
    audioTrackIndex: startingAudioTrack(decision)?.audioTrackIndex ?? 0,
    isBoosted: decision.preferences.isDialogueBoostOn,
  };
}

/**
 * The black box a film plays in, before any does: the stage's size, with no video in it, and what
 * the page draws over it.
 */
export function EmptyStage(props: { readonly children?: ReactNode }): JSX.Element {
  return <div className="film-player__stage film-player__stage--empty">{props.children}</div>;
}

/**
 * The stage and its video, the page's drawing over it inside the stage, where JASSUB places its
 * canvas too, then the page's drawing under it.
 */
function Stage(
  props: FilmParts &
    Pick<
      FilmComposition,
      "hasControls" | "isAutoPlay" | "over" | "under" | "around" | "keptVideo"
    > & {
      readonly src?: string;
      readonly onError?: (error: MediaError | null) => void;
    },
): JSX.Element {
  const {
    over: Over,
    under: Under,
    around: Around,
    videoRef,
    readLinkKbps,
    failure,
    ladder,
  } = props;
  useEffect(() => {
    const video = videoRef.current;
    // Absent where the page is not served over HTTPS, or the browser has no Screen Wake Lock.
    const locks = "wakeLock" in navigator ? navigator.wakeLock : undefined;
    return video === null ? undefined : keepScreenAwake(video, document, locks);
  }, [videoRef]);
  useHealedOnEmptied(videoRef);
  useEffect(() => {
    const video = videoRef.current;
    return video === null ? undefined : pauseWhenHidden(video, document, isThisHandheld());
  }, [videoRef]);
  useStartsWithEnoughAhead(videoRef, props.isAutoPlay);
  const parts = { videoRef, readLinkKbps, failure, ladder };
  const drawn = (
    <>
      <div className="film-player__stage">
        {props.keptVideo === undefined ? (
          // eslint-disable-next-line jsx-a11y-x/media-has-caption -- JASSUB draws the subtitles over the video, not a <track> (cut-a §6)
          <video
            className="film-player__video"
            ref={videoRef}
            src={props.src}
            controls={props.hasControls}
            playsInline
            onError={(event) => props.onError?.(event.currentTarget.error)}
          />
        ) : (
          <KeptVideo {...props} video={props.keptVideo} />
        )}
        {Over === undefined ? null : <Over {...parts} />}
      </div>
      {Under === undefined ? null : <Under {...parts} />}
    </>
  );
  return Around === undefined ? drawn : <Around {...parts}>{drawn}</Around>;
}

/**
 * The page's kept video, placed in the stage as the video the film plays in, as Stage's own would
 * be, and emptied as the film ends, for the next: a tap lets one video play with sound, and a new one
 * per film asked for a tap of its own (readyForSoundAtTap). Placed before any effect of the film's
 * runs, which finds it in videoRef.
 */
function KeptVideo(props: {
  readonly video: HTMLVideoElement;
  readonly videoRef: RefObject<HTMLVideoElement | null>;
  readonly hasControls: boolean;
  readonly isAutoPlay: boolean;
  readonly src?: string | undefined;
  readonly onError?: ((error: MediaError | null) => void) | undefined;
}): JSX.Element {
  const slotRef = useRef<HTMLDivElement>(null);
  const { video, videoRef, hasControls, isAutoPlay, src, onError } = props;
  useLayoutEffect(() => {
    const slot = slotRef.current;
    if (slot === null) {
      return undefined;
    }
    placeVideo(video, slot);
    videoRef.current = video;
    return () => {
      videoRef.current = null;
      video.remove();
      emptyVideo(video);
    };
  }, [video, videoRef]);
  useLayoutEffect(() => {
    showControls(video, { hasControls, isAutoPlay });
  }, [video, hasControls, isAutoPlay]);
  useLayoutEffect(() => {
    if (src !== undefined) {
      video.setAttribute("src", src);
    }
  }, [video, src]);
  useEffect(() => {
    if (onError === undefined) {
      return undefined;
    }
    const failed = (): void => {
      onError(video.error);
    };
    video.addEventListener("error", failed);
    return () => {
      video.removeEventListener("error", failed);
    };
  }, [video, onError]);
  return <div ref={slotRef} className="film-player__kept" />;
}

/** Places a kept video in its slot, dressed as Stage's own, its sound on. */
function placeVideo(video: HTMLVideoElement, slot: HTMLElement): void {
  /* eslint-disable no-param-reassign -- the kept video is dressed as Stage's own would be */
  video.className = "film-player__video";
  video.playsInline = true;
  // A film the last one played muted (playedMutedEvent) starts asking for its sound again.
  video.muted = false;
  /* eslint-enable no-param-reassign -- the video is placed */
  slot.append(video);
}

/**
 * Gives a kept video the player's own controls, as the page composes it; never the autoplay
 * attribute, as its start gate plays it (useStartsWithEnoughAhead).
 */
function showControls(
  video: HTMLVideoElement,
  composed: Pick<FilmComposition, "hasControls" | "isAutoPlay">,
): void {
  /* eslint-disable no-param-reassign -- as Stage's own video's attributes */
  video.controls = composed.hasControls;
  video.autoplay = false;
  /* eslint-enable no-param-reassign -- only these */
}

/**
 * Plays the video at each of its loads, where the page composes it to play at once, as the autoplay
 * attribute did, but only once its start gate has enough loaded ahead (start-gate.ts in the SDK):
 * the browser plays at canplay, and a Fire TV's cast, resumed with 1.4 s loaded, ran dry and stalled
 * (the owner, 2026-10-28). It loads ahead meanwhile, as an autoplaying video does. A play the
 * browser refuses for want of a press leaves it paused, as it left an autoplaying one, and tells the
 * core's log and the player's overlay (noteIfStartRefused, VQ272); a session
 * this video leads plays it its own way, muted if it must (together-follow.ts).
 */
function useStartsWithEnoughAhead(
  videoRef: RefObject<HTMLVideoElement | null>,
  isAutoPlay: boolean,
): void {
  useLayoutEffect(() => {
    const video = videoRef.current;
    if (video === null || !isAutoPlay) {
      return undefined;
    }
    const gate = startGateOf(video);
    const start = (): void => {
      gate.play(() => {
        // A refusal leaves it paused for the person's press.
        video.play().catch(noteIfStartRefused(video));
      });
    };
    video.preload = "auto";
    video.addEventListener("loadstart", start);
    if (video.networkState !== HTMLMediaElement.NETWORK_EMPTY) {
      start();
    }
    return () => {
      video.removeEventListener("loadstart", start);
      gate.cancel();
    };
  }, [videoRef, isAutoPlay]);
}

/**
 * Stops the video and lets go of its film, the source hls.js gave it too, which it leaves to whoever
 * changed it last (its BufferController): the video waits for the next.
 */
function emptyVideo(video: HTMLVideoElement): void {
  video.pause();
  video.removeAttribute("src");
  video.replaceChildren();
  video.load();
}

/**
 * Heals the page's audio session each time the video's media is torn down (emptied), as when its
 * player moves to another quality or rebuilds its decoding: where an iPhone's session became stuck in
 * the silent switch's category (WebKit bug 323104, page-audio.ts).
 */
function useHealedOnEmptied(videoRef: RefObject<HTMLVideoElement | null>): void {
  useEffect(() => {
    const video = videoRef.current;
    if (video === null) {
      return undefined;
    }
    video.addEventListener("emptied", healPageAudio);
    return () => {
      video.removeEventListener("emptied", healPageAudio);
    };
  }, [videoRef]);
}

/** The original file, playing; when the browser cannot play it, why, in words. */
function DirectFilm(props: {
  readonly decision: PlaybackDecision;
  readonly composition: FilmComposition;
}): JSX.Element {
  const { t } = useTranslation();
  const videoRef = useRef<HTMLVideoElement>(null);
  const [failure, setFailure] = usePlaybackFailure(props.composition.askAbout, {
    onMediaBack: props.composition.onMediaBack,
    videoRef,
  });
  const readLinkKbps = useBufferMeter(videoRef, props.decision.fileBitRateKbps);
  return (
    <>
      <Stage
        {...props.composition}
        videoRef={videoRef}
        readLinkKbps={readLinkKbps}
        failure={failure}
        src={props.decision.url}
        onError={(error) => {
          setFailure(playbackFailureText(t, mediaErrorFailure(error)));
        }}
      />
      {failure === undefined || props.composition.isFailureHidden === true ? null : (
        <p role="alert">{t("play.directFailed", { reason: failure })}</p>
      )}
    </>
  );
}

/**
 * The film on its ladder, every quality of it cut into chunks, from the one offered, stepping down
 * by itself and back up with care: from where it starts (D260), or where a stalled player moved to
 * it (D247 §2, §10). It says which quality plays, and when its lowest cannot keep up (D247 §8). On
 * its film's ladder it starts with the sound the decision names, boosted as the person left the
 * boost, and plays another its page chooses once that one's chunk at the playhead has loaded,
 * playing on meanwhile (playSound).
 */
function ChunkedFilm(props: {
  readonly offer: LadderOffer;
  readonly positionMs: number;
  readonly isOnLadder?: boolean;
  readonly composition: FilmComposition;
  /** The playback's media token, which AirPlay's grant is asked for by (D473); none offers no AirPlay. */
  readonly mediaToken?: string | undefined;
  /** What the film on its ladder may play, which its tracks are named from as its level moves. */
  readonly tracks?: PlaybackDecision["tracks"] | undefined;
  readonly sound?: PlayingSound | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  const { offer, positionMs } = props;
  const videoRef = useRef<HTMLVideoElement>(null);
  const [failure, setFailure] = usePlaybackFailure(props.composition.askAbout, {
    onMediaBack: props.composition.onMediaBack,
    videoRef,
  });
  const onFailed = useCallback(
    (seen: PlaybackFailure, whatFailed?: string) => {
      setFailure(playbackFailureText(t, seen), whatFailed);
    },
    [setFailure, t],
  );
  const { tracks } = props;
  // A playback's own start on its ladder starts by the link its device remembers (D381).
  const shouldStartByRememberedLink = props.isOnLadder === true && !isStartThePages(offer);
  const starting = { offer, positionMs, tracks, sound: props.sound, shouldStartByRememberedLink };
  const { playbackRef, playing, cannotKeepUp, sound, heardLevel, playSound } = useLadderPlayback(
    videoRef,
    { ...starting, mediaToken: props.mediaToken },
    onFailed,
  );
  const height = playing?.height ?? offer.height;
  // Its tracks are named while the level playing is one of the pictures they list.
  const hasTracks = tracks !== undefined && playingPicture(tracks, height) !== undefined;
  return (
    <>
      <Stage
        {...props.composition}
        videoRef={videoRef}
        readLinkKbps={() => playbackRef.current?.estimateKbps()}
        failure={failure}
        ladder={{ offer, positionMs, playing, cannotKeepUp, sound, heardLevel, playSound }}
      />
      {hasTracks && sound !== undefined ? (
        <ForTesters>
          <TracksNote tracks={tracks} height={height} sound={sound} />
        </ForTesters>
      ) : null}
      <LadderNotes
        offer={offer}
        playing={playing}
        cannotKeepUp={cannotKeepUp}
        isOnLadder={props.isOnLadder === true}
        hasTracks={hasTracks}
      />
      {/* Its qualities are cut as it plays, no copy prepared (the owner's test, 2026-10-18). */}
      {failure === undefined || props.composition.isFailureHidden === true ? null : (
        <p role="alert">{t("play.ladderFailed", { reason: failure })}</p>
      )}
    </>
  );
}

/**
 * A chunked playback (useChunkedPlayback) with the sound it plays, among its tracks' sounds: the one
 * it starts with, the boosted mix as the person left the boost, and another chosen by playSound, the
 * boost turned on or off or another track, heard once its chunk at the playhead has loaded, which a
 * playback started again, as for another place, starts with too; and the level of the sound heard,
 * which changes as the new one is heard, so the old never plays at the new one's gain.
 */
function useLadderPlayback(
  videoRef: RefObject<HTMLVideoElement | null>,
  starting: {
    readonly offer: LadderOffer;
    readonly positionMs: number;
    readonly tracks: PlaybackDecision["tracks"] | undefined;
    readonly sound: PlayingSound | undefined;
    readonly shouldStartByRememberedLink: boolean;
    readonly mediaToken: string | undefined;
  },
  onFailed: PlaybackFailed,
): ReturnType<typeof useChunkedPlayback> & {
  readonly sound: PlayingSound | undefined;
  readonly heardLevel: SoundLevel | undefined;
  readonly playSound: (sound: PlayingSound, cause: SoundChoiceCause) => boolean;
} {
  const { offer, positionMs, tracks, shouldStartByRememberedLink, mediaToken } = starting;
  useShapedByTheTabsLink();
  const [sound, setSound] = useState(starting.sound);
  const [heard, setHeard] = useState(starting.sound);
  const startAudioRef = useRef(ladderStartingAudio(tracks, starting.sound));
  const chunked = useChunkedPlayback(
    videoRef,
    { offer, positionMs, startAudioRef, shouldStartByRememberedLink, mediaToken },
    onFailed,
  );
  const { playbackRef } = chunked;
  const playSound = useCallback(
    (chosen: PlayingSound, cause: SoundChoiceCause): boolean => {
      const index = ladderAudioRenditionIndex(tracks, chosen);
      const isInPlace = chooseSoundInPlace(playbackRef.current, index, {
        cause,
        heard: () => {
          setHeard(chosen);
        },
      });
      // The next of its playbacks starts with it, switched in place or not, as the boost did.
      if (index !== undefined) {
        setSound(chosen);
        startAudioRef.current = index;
      }
      return isInPlace;
    },
    [tracks, playbackRef],
  );
  const heardLevel = useMemo(() => ladderSoundLevelOf(tracks, heard), [tracks, heard]);
  return { ...chunked, sound, heardLevel, playSound };
}

/**
 * Measures the link of a video playing a file of fileBitRateKbps as it is, from how fast its buffer
 * grows (D247 §1); none for a file whose bit rate is not known.
 */
export function useBufferMeter(
  videoRef: RefObject<HTMLVideoElement | null>,
  fileBitRateKbps: number,
): () => number | undefined {
  const meterRef = useRef<LinkMeter | undefined>(undefined);
  useEffect(() => {
    const video = videoRef.current;
    if (video === null || fileBitRateKbps <= 0) {
      return undefined;
    }
    const meter = meterBufferGrowth(video, { fileBitRateKbps, monotonicMs });
    meterRef.current = meter;
    return () => {
      meter.stop();
      meterRef.current = undefined;
    };
  }, [videoRef, fileBitRateKbps]);
  return () => meterRef.current?.estimateKbps();
}

/**
 * Has the tab's chunk loads shaped by its debug link, its cap and its drop (D380), where the page
 * has one: handed to the player here, with the player, so the home page loads no player for it.
 */
function useShapedByTheTabsLink(): void {
  const { linkDebug } = useServices();
  useEffect(() => {
    shapeChunkLoads(linkDebug);
  }, [linkDebug]);
}

/** What a chunked playback says: the quality playing, and when even the lowest cannot keep up. */
export function LadderNotes(props: {
  readonly offer: LadderOffer;
  readonly playing: PlayedLevel | undefined;
  readonly cannotKeepUp: CannotKeepUp | undefined;
  /** Whether the playback started on its film's ladder (D260), rather than moved to it after a stall (D253). */
  readonly isOnLadder?: boolean;
  /** Whether the tracks playing are named beside it, which say the quality in its place. */
  readonly hasTracks?: boolean;
}): JSX.Element {
  const { t } = useTranslation();
  const { offer, cannotKeepUp } = props;
  const lowest = rateText(cannotKeepUp?.lowestKbps ?? offer.startAtBps / 1000);
  return (
    <>
      {props.hasTracks === true ? null : (
        <ForTesters>
          <QualityLine {...props} />
        </ForTesters>
      )}
      {cannotKeepUp !== undefined || offer.isBelowTheLowest ? (
        <p className="needs-fixing">
          <output>
            {cannotKeepUp?.linkKbps === undefined
              ? t("play.cannotKeepUpUnmeasured", { lowest })
              : t("play.cannotKeepUp", { link: rateText(cannotKeepUp.linkKbps), lowest })}
          </output>
        </p>
      ) : null}
    </>
  );
}

/** The quality a chunked playback plays, where no tracks are named: starting, then as it plays. */
function QualityLine(props: {
  readonly offer: LadderOffer;
  readonly playing: PlayedLevel | undefined;
  readonly isOnLadder?: boolean;
}): JSX.Element {
  const { t } = useTranslation();
  const { offer, playing } = props;
  const isOnLadder = props.isOnLadder === true;
  const startingKey = isOnLadder ? "play.ladderStarting" : "play.lowerQualityStarting";
  const playingKey = isOnLadder ? "play.ladderPlaying" : "play.lowerQuality";
  return (
    <p className="play__reason">
      {playing === undefined
        ? t(startingKey, { height: offer.height, rate: rateText(offer.qualityKbps) })
        : t(playingKey, { height: playing.height, rate: rateText(playing.averageBitrate / 1000) })}
    </p>
  );
}

/** Why a video could not play, in the viewer's words, by the player's code for it. */
export function playbackFailureText(t: TFunction, failure: PlaybackFailure): string {
  return t(`play.failure.${failure}`);
}
