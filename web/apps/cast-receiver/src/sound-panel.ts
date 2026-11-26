// SPDX-License-Identifier: MPL-2.0

import {
  gainAfterVolume,
  soundOffset,
  type ChunkedPlayback,
  type Gain,
  type SoundChoiceCause,
  type SoundOffset,
} from "@orpheum/player";
import {
  dataOrThrow,
  failureText,
  type Api,
  type components,
  type PlaybackSound,
  type RemotesPanel,
  type SoundChoice,
  type SoundTrack,
} from "@orpheum/sdk";

/** What the core decides for a film the receiver plays, as far as its sound goes; an older core's may lack parts. */
export type SoundDecision = Partial<
  Pick<
    components["schemas"]["StartPlaybackOutputBody"],
    "audioTracks" | "preferences" | "startingAudioStreamId" | "tracks"
  >
>;

/** One of the ladder's sounds, as the decision names it. */
type LadderSound = components["schemas"]["SoundTrackBody"];

/** One of the film's audio tracks, as the remote names it. */
function soundTrackOf(track: components["schemas"]["AudioTrackBody"]): SoundTrack {
  return {
    streamId: track.streamId,
    language: track.language,
    kind: track.kind,
    ...(track.containerTitle === undefined ? {} : { containerTitle: track.containerTitle }),
  };
}

/** The audio track a film starts with: the core's choice, else the ladder's first. */
function startingTrackOf(decision: SoundDecision): number {
  const starting = (decision.audioTracks ?? []).find(
    (track) => track.streamId === decision.startingAudioStreamId,
  );
  const first = (decision.tracks?.sounds ?? []).find((each) => !each.isOriginal);
  return starting?.audioTrackIndex ?? first?.audioTrackIndex ?? 0;
}

/** The part of a playback the panel switches the sound of. */
export type SwitchesSound = Pick<ChunkedPlayback, "setAudioRendition" | "canSwitchSoundInPlace">;

/** What a receiver's panel works with: the film's file, the core, the video and the log. */
export interface PanelWays {
  readonly api: Api;
  readonly fileId: number;
  readonly video: HTMLVideoElement;
  readonly note: (line: string) => void;
}

/** Web Audio's parts of the sound panel, a test's stand-ins where given. */
export interface SoundAudio {
  /** The level after the video's volume. */
  readonly gainOf?: ((video: HTMLVideoElement) => Gain) | undefined;
  /** The sound's move from the picture. */
  readonly offsetOf?: ((video: HTMLVideoElement) => SoundOffset) | undefined;
}

/**
 * The Chromecast's sound as its cast's remote works it (D434, D555 §3), as a browser receiver's Sound
 * menu is worked (the web app's sound-panel.ts): its audio track among the file's own where the
 * ladder has a mix of each, switched in place; the dialogue boost where the track has a boosted mix
 * (D222, D336); levelling (D226 §4), by a gain after the volume only once the sound needs one, so a
 * film played as the core levelled it goes through no Web Audio; and how far the device moves its
 * sound from its picture, for this film alone, as the receiver keeps nothing (its README). Each
 * choice of the person's is kept for them as the Play page keeps it. Its film's channel is not said:
 * its volume is the Cast device's own, set through the sender's Cast session (the web app's
 * cast-device-volume.ts). Nor the file's own offset (D537), which asks for the film again.
 */
export class ReceiverSoundPanel implements RemotesPanel<SoundChoice> {
  private readonly listeners = new Set<() => void>();
  private readonly tracks: readonly SoundTrack[];
  private audioTrackIndex: number;
  private isBoosted: boolean;
  private isLevellingOn: boolean;
  private soundOffsetMs = 0;
  private playback: SwitchesSound | undefined;
  private gain: Gain | undefined;
  private offset: SoundOffset | undefined;

  constructor(
    private readonly decision: SoundDecision,
    private readonly ways: PanelWays,
    private readonly audio: SoundAudio = {},
  ) {
    this.tracks = (decision.audioTracks ?? [])
      .filter((track) => this.renditionOf(track.audioTrackIndex, false) !== undefined)
      .map(soundTrackOf);
    this.audioTrackIndex = startingTrackOf(decision);
    const { preferences } = decision;
    this.isBoosted =
      preferences?.isDialogueBoostOn === true &&
      this.renditionOf(this.audioTrackIndex, true) !== undefined;
    this.isLevellingOn = preferences?.isLevellingOn ?? false;
  }

  /** How the player starts the film's sound: on the boosted mix where the boost is on, else its default. */
  startingSound(): { readonly startAudioRenditionIndex?: number } {
    const index = this.isBoosted ? this.renditionOf(this.audioTrackIndex, true) : undefined;
    return index === undefined ? {} : { startAudioRenditionIndex: index };
  }

  /** Has the panel switch playback's sound, the film started; its level set as its sound needs. */
  plays(playback: SwitchesSound): void {
    this.playback = playback;
    this.level();
  }

  readonly subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  };

  /** How the film sounds now, as its remote shows it; undefined for a film with no ladder sound. */
  readonly read = (): PlaybackSound | undefined => {
    if (this.renditionOf(this.audioTrackIndex, this.isBoosted) === undefined) {
      return undefined;
    }
    const track = (this.decision.audioTracks ?? []).find(
      (each) => each.audioTrackIndex === this.audioTrackIndex,
    );
    const canBoost = this.renditionOf(this.audioTrackIndex, true) !== undefined;
    return {
      ...(canBoost ? { isBoostOn: this.isBoosted } : {}),
      isLevellingOn: this.isLevellingOn,
      ...(track === undefined ? {} : { audioStreamId: track.streamId }),
      audioTracks: this.tracks,
      soundOffsetMs: this.soundOffsetMs,
    };
  };

  readonly choose = (choice: SoundChoice): void => {
    switch (choice.kind) {
      case "audioTrack":
        this.chooseTrack(choice.streamId);
        return;
      case "boost":
        if (
          this.switchTo(this.audioTrackIndex, choice.isOn, choice.isOn ? "boostOn" : "boostOff")
        ) {
          this.keep({ isDialogueBoostOn: choice.isOn });
        }
        return;
      case "levelling":
        this.chooseLevelling(choice.isOn);
        return;
      case "soundOffset":
        this.chooseOffset(choice.ms);
        return;
      case "fileSoundOffset":
      case "channel":
        // Neither is said, so no remote offers it.
        return;
    }
  };

  private chooseLevelling(isOn: boolean): void {
    this.isLevellingOn = isOn;
    this.level();
    this.keep({ isLevellingOn: isOn });
    this.told();
  }

  private chooseOffset(ms: number): void {
    this.soundOffsetMs = ms;
    this.offset ??= (this.audio.offsetOf ?? soundOffset)(this.ways.video);
    this.offset.setMs(ms);
    this.told();
  }

  private chooseTrack(streamId: number): void {
    const track = (this.decision.audioTracks ?? []).find((each) => each.streamId === streamId);
    if (track === undefined) {
      return;
    }
    const isBoosted = this.isBoosted && this.renditionOf(track.audioTrackIndex, true) !== undefined;
    if (this.switchTo(track.audioTrackIndex, isBoosted, "trackChosen")) {
      this.keepTrack(streamId);
    }
  }

  /** Plays the track's mix in place, levelled once it is heard; whether the ladder has it. */
  private switchTo(audioTrackIndex: number, isBoosted: boolean, cause: SoundChoiceCause): boolean {
    const index = this.renditionOf(audioTrackIndex, isBoosted);
    const { playback } = this;
    if (index === undefined || playback?.canSwitchSoundInPlace !== true) {
      return false;
    }
    this.audioTrackIndex = audioTrackIndex;
    this.isBoosted = isBoosted;
    playback.setAudioRendition(index, cause, () => {
      this.level();
    });
    this.told();
    return true;
  }

  /** The ladder's rendition of the track's mix, by its place in the master playlist. */
  private renditionOf(audioTrackIndex: number, isBoosted: boolean): number | undefined {
    const index = (this.decision.tracks?.sounds ?? []).findIndex(
      (each) =>
        !each.isOriginal &&
        each.audioTrackIndex === audioTrackIndex &&
        each.isBoosted === isBoosted,
    );
    return index < 0 ? undefined : index;
  }

  /**
   * Sets the gain the sound playing needs (D226 §4), as the Play page's levelling.tsx does: the
   * sound's levelling where it is on, less what its chunks carry already; a gain made only once one
   * is needed.
   */
  private level(): void {
    const sound = this.soundPlaying();
    if (sound === undefined) {
      return;
    }
    const { levelling, carriedGainDb } = sound;
    const wantedDb = this.isLevellingOn && levelling.isMeasured ? levelling.gainDb : 0;
    const db = wantedDb - carriedGainDb;
    if (db === 0 && this.gain === undefined) {
      return;
    }
    this.gain ??= (this.audio.gainOf ?? gainAfterVolume)(this.ways.video);
    this.gain.setDb(db);
  }

  /** The ladder's sound playing now, as the decision names it. */
  private soundPlaying(): LadderSound | undefined {
    const index = this.renditionOf(this.audioTrackIndex, this.isBoosted);
    return index === undefined ? undefined : this.decision.tracks?.sounds?.[index];
  }

  private told(): void {
    for (const listener of this.listeners) {
      listener();
    }
  }

  /** Keeps one of the person's own choices, as the Play page's switch does. */
  private keep(body: { isLevellingOn: boolean } | { isDialogueBoostOn: boolean }): void {
    void this.ways.api
      .PUT("/me/preferences", { body })
      .then(dataOrThrow)
      .catch((cause: unknown) => {
        this.ways.note(`could not keep the sound's choice: ${failureText(cause)}`);
      });
  }

  /** Keeps the person's audio track in the file, as the Play page's choice does (D230). */
  private keepTrack(streamId: number): void {
    void this.ways.api
      .PUT("/files/{id}/preferences", {
        params: { path: { id: this.ways.fileId } },
        body: { audioChoice: { streamId } },
      })
      .then(dataOrThrow)
      .catch((cause: unknown) => {
        this.ways.note(`could not keep the audio track: ${failureText(cause)}`);
      });
  }
}
