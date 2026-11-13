// SPDX-License-Identifier: MPL-2.0

/**
 * The film's tracks and sound as the person chooses them: the subtitles, the audio track, levelling
 * and the dialogue boost, held around the player's stage (FilmComposition's around) for every piece
 * over or under the film to read and work through PlayerTracksContext. The choices live here, not in
 * the menus and switches that show them: the subtitles stay drawn, levelling's gain applied, and a
 * cast's remote reads and works the sound (sound-panel.ts) whether or not any menu is drawn, as a
 * menu that opens only when asked, or a receiver that draws none, needs (D407 §8).
 */

import {
  createContext,
  use,
  useMemo,
  useState,
  type JSX,
  type ReactNode,
  type RefObject,
} from "react";

import { useAudioChoice, type AudioChoice, type AudioTrack } from "./audio-tracks.tsx";
import type { FilmParts } from "./film-player.tsx";
import {
  hasACentre,
  ladderAudioRenditionIndex,
  ladderSoundLevel,
  playTrackChosen,
  soundChoiceCause,
  useOwnTrackChoice,
} from "./ladder-sound.ts";
import type { FileWait } from "./file-waits.ts";
import {
  useLevelGain,
  useMeasuredLevel,
  useRaiseLeftToTheLadder,
  type SoundLevel,
} from "./levelling.tsx";
import { useOwnChoice, type OwnChoiceState, type OwnChoiceWays } from "./sound.tsx";
import { useSubtitleChoice, type SubtitleChoice } from "./subtitles.tsx";
import { playsOwnFile, type PlaybackDecision } from "./watching.ts";

/** The film's tracks and sound as they stand, and the ways to change them. */
export interface PlayerTracks {
  readonly subtitles: SubtitleChoice;
  readonly audio: AudioChoice;
  readonly levelling: OwnChoiceState;
  /** The sound playing now, which levelling levels, taken up once it is measured as it plays. */
  readonly level: SoundLevel;
  /** How long measuring the sound playing will take, while it is not measured. */
  readonly measuring: FileWait | undefined;
  /** The dialogue boost, undefined where the sound playing has no boost to offer. */
  readonly boost: OwnChoiceState | undefined;
}

/** The film's tracks, for what the page draws over or under it; undefined outside a film. */
export const PlayerTracksContext = createContext<PlayerTracks | undefined>(undefined);

/** The film's tracks, for a piece drawn over or under it. */
export function usePlayerTracks(): PlayerTracks {
  const tracks = use(PlayerTracksContext);
  if (tracks === undefined) {
    throw new Error("a piece of the player outside its PlayerTracksContext");
  }
  return tracks;
}

/** What a film's tracks are chosen for, and how its way of playing plays a choice. */
export interface FilmTracks {
  readonly fileId: number;
  readonly decision: PlaybackDecision;
  readonly videoRef: RefObject<HTMLVideoElement | null>;
  /** The sound playing now, which levelling levels. */
  readonly level: SoundLevel;
  /** Plays a track chosen and kept, given the boost as it stands. */
  readonly onAudioChosen: (track: AudioTrack, isBoostOn: boolean) => void;
  /** The boost: how it starts and what its switching does, and whether the sound offers it. */
  readonly boost: OwnChoiceWays & { readonly isOffered: boolean };
  /**
   * Asks for the film again, onto its ladder, where levelling raises its sound and the device's Web
   * Audio is too late to (D436); undefined where the sound playing is the ladder's already.
   */
  readonly onRaiseLeftToTheLadder?: () => void;
  readonly children: ReactNode;
}

/**
 * Holds the film's track choices, starting as the person left them, for children to read through
 * PlayerTracksContext: the chosen subtitles drawn over the video, levelling's gain applied, and the
 * audio track, levelling and the boost, while offered, shown to and worked by a cast's remote.
 */
export function PlayerTracksProvider(props: FilmTracks): JSX.Element {
  const { fileId, decision, videoRef } = props;
  const subtitles = useSubtitleChoice({
    fileId,
    tracks: decision.subtitles ?? [],
    preferences: decision.preferences,
    videoRef,
  });
  const boost = useOwnChoice("isDialogueBoostOn", props.boost);
  const audio = useAudioChoice({
    fileId,
    decision,
    onChosen: (track) => {
      props.onAudioChosen(track, boost.isOn);
    },
  });
  // Levelling as the server keeps it, which a playback asked for again is decided by.
  const [isLevellingKept, setIsLevellingKept] = useState(decision.preferences.isLevellingOn);
  const levelling = useOwnChoice("isLevellingOn", {
    isInitiallyOn: decision.preferences.isLevellingOn,
    onSwitched: () => undefined,
    onKept: setIsLevellingKept,
  });
  const playing = audio.tracks.find((track) => track.streamId === audio.chosenStreamId);
  const { level, measuring } = useMeasuredLevel(fileId, props.level, playing?.audioTrackIndex);
  useLevelGain(level, levelling.isOn, { videoRef, isOwnFile: playsOwnFile(decision) });
  useRaiseLeftToTheLadder(fileId, level, {
    isLevellingKept,
    askAgain: props.onRaiseLeftToTheLadder,
  });
  const { isOffered } = props.boost;
  const tracks = useMemo(
    () => ({ subtitles, audio, levelling, level, measuring, boost: isOffered ? boost : undefined }),
    [subtitles, audio, levelling, level, measuring, boost, isOffered],
  );
  return <PlayerTracksContext value={tracks}>{props.children}</PlayerTracksContext>;
}

/** What a way of playing gives its tracks: the file, its decision and the video. */
interface FilmOf {
  readonly fileId: number;
  readonly decision: PlaybackDecision;
  readonly videoRef: RefObject<HTMLVideoElement | null>;
  readonly children: ReactNode;
}

/**
 * The tracks of the original. Its sound is the film's own, which the device mixes down itself, so
 * the dialogue boost, where the track has a centre to raise, starts off here: turned on, it is kept,
 * then askAgain.boost asks for the film again from where the video stands, and the server answers
 * with its ladder carrying the boosted mix (D336 §1, §3). Another audio track chosen switches in
 * place where the element lists its file's tracks, as Safari's does, levelled by its own and said to
 * the server, whose stall's offer starts it; else askAgain.audio asks for the film again, onto its
 * ladder, once (the owner's answers of 2026-10-31).
 */
export function OwnFileTracks(
  props: FilmOf & {
    readonly askAgain: {
      readonly audio: () => void;
      readonly boost: () => void;
      readonly raise: () => void;
    };
  },
): JSX.Element {
  const { decision, askAgain } = props;
  const { level, onAudioChosen } = useOwnTrackChoice(decision, props.videoRef, {
    askAgain: askAgain.audio,
  });
  const boost = {
    isInitiallyOn: false,
    onSwitched: () => undefined,
    onKept: (isOn: boolean) => {
      if (isOn) {
        askAgain.boost();
      }
    },
    isOffered: hasACentre(decision),
  };
  return (
    <PlayerTracksProvider
      {...props}
      level={level}
      onAudioChosen={(track) => {
        onAudioChosen(track);
      }}
      boost={boost}
      onRaiseLeftToTheLadder={askAgain.raise}
    />
  );
}

/**
 * The tracks of the film on its ladder: its sound levelled by its own track's levelling, changed as
 * a new sound is heard, and the dialogue boost offered where the track playing has a boosted mix
 * (D217 §3, D222). The boost turned on or off and another audio track chosen go one way: the
 * ladder's sound changes as soon as the other has loaded at the playhead, the picture playing on,
 * the old sound heard meanwhile (audio-switch-and-retiming-2026-11-01.md §1.2); where it cannot,
 * askAgain asks for the film again. A Watch Together leader's choice is its own and reloads no one's
 * player; a cast's receiver follows its remote's by the same path (sound-panel.ts).
 */
export function LadderTracks(
  props: FilmOf & {
    readonly ladder: NonNullable<FilmParts["ladder"]>;
    readonly askAgain: () => void;
  },
): JSX.Element {
  const { ladder, decision, askAgain } = props;
  const { sound, playSound } = ladder;
  const level = ladder.heardLevel ?? ladderSoundLevel(decision, ladder.offer.soundCarriedGainDb);
  const { tracks } = decision;
  const canBoost =
    sound !== undefined &&
    ladderAudioRenditionIndex(tracks, { ...sound, isBoosted: true }) !== undefined;
  // The boost as the person left it, kept across a track with no centre to raise, whose switch hides.
  const boost = {
    isInitiallyOn: decision.preferences.isDialogueBoostOn,
    onSwitched: (isOn: boolean) => {
      if (sound !== undefined) {
        const boosted = { audioTrackIndex: sound.audioTrackIndex, isBoosted: isOn };
        playSound(boosted, soundChoiceCause(boosted));
      }
    },
    isOffered: canBoost,
  };
  return (
    <PlayerTracksProvider
      {...props}
      level={level}
      onAudioChosen={(track, isBoostOn) => {
        playTrackChosen(
          { audioTrackIndex: track.audioTrackIndex, isBoostOn, tracks },
          { playSound, askAgain },
        );
      }}
      boost={boost}
    />
  );
}
