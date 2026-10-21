// SPDX-License-Identifier: MPL-2.0

import "./play.css";

import { seekTo, type HlsPlayback } from "@orpheum/player";
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
  type RefObject,
} from "react";
import { useTranslation } from "react-i18next";

import { AudioMenu, mixIndices, startingAudioTrack, type AudioTrack } from "./audio-tracks.tsx";
import {
  audioRenditionIndex,
  FilmPlayer,
  type FilmComposition,
  type FilmParts,
  type MovedDown,
} from "./film-player.tsx";
import { problemText } from "./problem.ts";
import { askUntilAnswered, reopenAtMostEveryMs, type ReopenAt } from "./reopen.ts";
import type { LadderOffer } from "./moving-down.ts";
import { usePlayState } from "./play-state.ts";
import { useServices } from "./services.tsx";
import { BoostSwitch, SoundLine, SoundQuestion } from "./sound.tsx";
import { FindSubtitles } from "./subtitle-search.tsx";
import { Subtitles } from "./subtitles.tsx";
import { camelKey } from "./catalogue-key.ts";
import { LevelSwitch, soundLevelOf, type SoundLevel } from "./levelling.tsx";
import { Loaded } from "./title.tsx";
import { HostPlaysTogether } from "./together-host.tsx";
import { useResource, type Resource } from "./use-resource.ts";
import { watchingOf, type PlaybackDecision, type StartCause } from "./watching.ts";

/** Why a playing page asks how the file plays again. */
type DecideAgainCause = Extract<
  StartCause,
  "soundAnswered" | "soundChanged" | "subtitleFetched" | "audioChosen"
>;

/**
 * A decision, shown: the original playing, or a prepared copy and why. The reason is always on
 * screen (D188). Another audio track chosen in the original asks for the decision again, which
 * prepares a copy, since a browser plays the original's first (D230 §5).
 */
export function PlayView(props: {
  readonly fileId: number;
  readonly decision: PlaybackDecision;
  /** Asks the server again how the file plays: after the sound changed (D222) or a subtitle was fetched. */
  readonly onDecideAgain: (cause: DecideAgainCause) => void;
  /**
   * Told that the server lost the playback, and how to read where it is: true when the page opens
   * it again from there; false, or undefined, tells the person.
   */
  readonly onPlaybackGone?: ((whereNow: () => ReopenAt) => boolean) | undefined;
  /**
   * Told where the video stood when its player gave the film's own picture up, for the page to ask
   * for the playback again from there, at the twin (D323 §2).
   */
  readonly onOwnPictureGivenUp?: ((at: ReopenAt) => void) | undefined;
  /** Whether the film waits to be played, as one the person had paused when it was opened again. */
  readonly isStartPaused?: boolean;
  /**
   * Whether the page is opening the playback again, this one lost: what it plays is held on screen,
   * and its player's failures, its media gone with the playback, are not told.
   */
  readonly isReconnecting?: boolean;
}): JSX.Element {
  // The lower quality a stalled player moved to, and where it was (D247 §2); per playback.
  const [moved, setMoved] = useState<MovedDown | undefined>(undefined);
  // The video playing now, which the host's Watch Together session follows: kept here, above the
  // way of playing, so a move to a lower quality is no film stopped and put on again for everyone
  // (the owner's test, 2026-10-05).
  const [video, setVideo] = useState<HTMLVideoElement | null>(null);
  // Whether its lowest quality cannot keep up, which a host's session tells everyone (D247 §8).
  const [cannotKeepUp, setCannotKeepUp] = useState<CannotKeepUp | undefined>(undefined);
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
  const { titleName } = props.decision;
  return (
    <>
      {titleName === undefined ? null : <h1 className="play__title">{titleName}</h1>}
      <PlayContext value={ways}>
        <div className="play">
          <FilmPlayer
            decision={props.decision}
            moved={moved}
            ladderStartMs={props.decision.resumePositionMs}
            composition={pageComposition(props)}
          />
        </div>
      </PlayContext>
      <HostPlaysTogether
        fileId={props.fileId}
        video={video}
        startPositionMs={props.decision.resumePositionMs}
        cannotKeepUp={cannotKeepUp}
        audioOutput={props.decision.audioOutput}
      />
    </>
  );
}

/** How the Play page composes its player: its own controls, playing at once unless it waits. */
function pageComposition(props: Parameters<typeof PlayView>[0]): FilmComposition {
  return {
    hasControls: true,
    isAutoPlay: props.isStartPaused !== true,
    isFailureHidden: props.isReconnecting === true,
    askAbout: props.fileId,
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

/** A handler that asks how the file plays again, saying why. */
function decideAgainFor(
  props: { readonly onDecideAgain: (cause: DecideAgainCause) => void },
  cause: DecideAgainCause,
): () => void {
  return () => {
    props.onDecideAgain(cause);
  };
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
 * tracks, finding subtitles, the audio track, the device's sound and levelling. It keeps the
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
      {props.decision.audioOutput === undefined ? null : (
        <SoundLine
          current={props.decision.audioOutput}
          onChanged={decideAgainFor(props, "soundChanged")}
        />
      )}
      <LevelSwitch
        fileId={props.fileId}
        level={props.level}
        isInitiallyOn={props.decision.preferences.isLevellingOn}
        videoRef={props.videoRef}
      />
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
 * the original's reason, a prepared copy's reasons, audio tracks and dialogue boost, or a ladder's
 * word that it cannot keep up, which a host's session tells everyone (D247 §8).
 */
function PlayUnder(props: PlayProps & FilmParts): JSX.Element {
  if (props.preparedRef !== undefined) {
    return <PreparedUnder {...props} preparedRef={props.preparedRef} />;
  }
  if (props.ladder !== undefined) {
    return <LadderUnder {...props} ladder={props.ladder} />;
  }
  return <DirectUnder {...props} />;
}

/** Under the original: what sits under every video; its tracks say it plays as it is. */
function DirectUnder(props: PlayProps & FilmParts): JSX.Element {
  return (
    <UnderTheVideo
      {...props}
      level={soundLevelOf(props.decision, undefined)}
      onAudioChosen={decideAgainFor(props, "audioChosen")}
    />
  );
}

/**
 * Under a prepared copy: what sits under every video, why a copy plays, and a switch for the
 * dialogue boost where the track playing has a centre to raise (D217 §3). Another track chosen plays
 * at once (D23).
 */
function PreparedUnder(
  props: PlayProps &
    FilmParts & {
      readonly preparedRef: RefObject<HlsPlayback | undefined>;
    },
): JSX.Element {
  const { t } = useTranslation();
  const { decision } = props;
  const [audioTrackIndex, setAudioTrackIndex] = useState(
    () => startingAudioTrack(decision)?.audioTrackIndex ?? 0,
  );
  const [isBoostOn, setIsBoostOn] = useState(decision.preferences.isDialogueBoostOn);
  const boostMixes = mixIndices(decision, audioTrackIndex);
  const play = (trackIndex: number, isBoosted: boolean): void => {
    const index = audioRenditionIndex(decision, trackIndex, isBoosted);
    if (index !== undefined) {
      props.preparedRef.current?.setAudioRendition(index);
    }
  };
  const reasons = (decision.notDirectReasons ?? [])
    .map((reason) => t(`play.reason.${camelKey(reason)}`))
    .join(", ");
  return (
    <>
      <UnderTheVideo
        {...props}
        level={soundLevelOf(decision, audioTrackIndex)}
        onAudioChosen={(track) => {
          setAudioTrackIndex(track.audioTrackIndex);
          play(track.audioTrackIndex, isBoostOn);
        }}
      />
      <p className="play__reason">{t("play.prepared", { reasons })}</p>
      {boostMixes?.boosted === undefined ? null : (
        <BoostSwitch
          isInitiallyOn={isBoostOn}
          onSwitched={(isOn) => {
            setIsBoostOn(isOn);
            play(audioTrackIndex, isOn);
          }}
        />
      )}
    </>
  );
}

/**
 * Under the film on its ladder: what sits under every video, its play state kept from where it
 * started, not where the person last left the file; its sound carries its levelling, as a prepared
 * copy's does. Its lowest quality's word that it cannot keep up goes to a host's session (D247 §8).
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
  const level: SoundLevel = {
    levelling: {
      isMeasured: ladder.offer.soundCarriedGainDb !== 0,
      gainDb: ladder.offer.soundCarriedGainDb,
    },
    carriedGainDb: ladder.offer.soundCarriedGainDb,
  };
  return (
    <UnderTheVideo
      {...props}
      decision={decision}
      level={level}
      onAudioChosen={decideAgainFor(props, "audioChosen")}
    />
  );
}

/** A decision for the Play page, and whether its film waits to be played, as one opened again. */
interface Opened {
  readonly decision: PlaybackDecision;
  readonly isStartPaused?: boolean;
}

/**
 * The player's screen for one file. A device not yet asked how it makes sound is asked first,
 * and the decision asked for again with the answer (D222). A playback the server lost, as when it
 * restarted, is opened again where it was, playing or paused as it was, while the server answers
 * within about a minute (askUntilAnswered); one lost again within reopenAtMostEveryMs is told.
 */
export function Play(props: { readonly fileId: number }): JSX.Element {
  const { session } = useServices();
  const { t } = useTranslation();
  const { opened, held, decideAgain, reopenPlayback, startAtTheTwin } = useOpenedPlayback(
    props.fileId,
  );
  const render = ({ decision, isStartPaused }: Opened): JSX.Element =>
    decision.audioOutput === undefined && session.deviceId() !== undefined ? (
      <div className="play">
        <SoundQuestion
          onAnswered={() => {
            decideAgain("soundAnswered");
          }}
        />
      </div>
    ) : (
      <PlayView
        key={decision.mediaToken}
        fileId={props.fileId}
        decision={decision}
        onDecideAgain={decideAgain}
        onPlaybackGone={reopenPlayback}
        onOwnPictureGivenUp={startAtTheTwin}
        isStartPaused={isStartPaused === true}
        isReconnecting={opened.state !== "loaded"}
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

/** What the Play page shows: its decision, or, while the next is asked for, the one held. */
function shownOf(opened: Resource<Opened>, held: Opened | undefined): Opened | undefined {
  if (opened.state === "loaded") {
    return opened.value;
  }
  return opened.state === "loading" ? held : undefined;
}

/**
 * A playback opened again from where it was: the server lost it, or its player gave the film's own
 * picture up and is offered the twin in its place (D323 §2).
 */
interface Reopening {
  readonly whereNow: () => ReopenAt;
  readonly cause: "playbackGone" | "ownPictureGivenUp";
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
  return { reopenPlayback, startAtTheTwin };
}

/**
 * The Play page's decision for the file, asked for again when decideAgain says why, and opened again
 * where a playback the server lost is when its new one comes, once reopenPlayback is told so, which
 * says whether it is: not within reopenAtMostEveryMs of the last. held is what played before, shown
 * while the new one is asked for.
 */
function useOpenedPlayback(fileId: number): {
  readonly opened: Resource<Opened>;
  readonly held: Opened | undefined;
  readonly decideAgain: (cause: DecideAgainCause) => void;
  readonly reopenPlayback: (whereNow: () => ReopenAt) => boolean;
  readonly startAtTheTwin: (at: ReopenAt) => void;
} {
  const { api, playableFormats } = useServices();
  // Why the next decision is asked for: the page's opening, unless something asked it again.
  const nextCauseRef = useRef<StartCause>("playPage");
  // Where a playback opened again is, read as its new one comes, for that to start from, and why it
  // is: the server lost it, or its player gave the film's own picture up; and when the last lost one
  // was opened again, on the page's clock.
  const reopenRef = useRef<
    | { readonly whereNow: () => ReopenAt; readonly cause: "playbackGone" | "ownPictureGivenUp" }
    | undefined
  >(undefined);
  const [held, setHeld] = useState<Opened | undefined>(undefined);
  const [opened, reload] = useResource(`play/${String(fileId)}`, async (): Promise<Opened> => {
    const cause = nextCauseRef.current;
    nextCauseRef.current = "playPage";
    const reopen = reopenRef.current;
    reopenRef.current = undefined;
    const watching = watchingOf(api);
    if (reopen === undefined) {
      setHeld(undefined);
      return { decision: await watching.decide(fileId, playableFormats(), cause) };
    }
    // The server may still be starting: asked again for about a minute.
    const decision = await askUntilAnswered(() =>
      watching.decide(fileId, playableFormats(), reopen.cause),
    );
    const at = reopen.whereNow();
    const resumePositionMs = Math.round(at.positionMs);
    return { decision: { ...decision, resumePositionMs }, isStartPaused: at.isPaused };
  });
  const decideAgain = useCallback(
    (cause: DecideAgainCause) => {
      nextCauseRef.current = cause;
      reload();
    },
    [reload],
  );
  const { reopenPlayback, startAtTheTwin } = useReopening({
    reopenRef,
    reload,
    hold: () => {
      setHeld(opened.state === "loaded" ? opened.value : undefined);
    },
  });
  return { opened, held, decideAgain, reopenPlayback, startAtTheTwin };
}
