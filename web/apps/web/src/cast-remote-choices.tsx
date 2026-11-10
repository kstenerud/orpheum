// SPDX-License-Identifier: MPL-2.0

import type {
  CarriesPictureChoices,
  CarriesSoundChoices,
  CastSending,
  PlaybackPicture,
  PlaybackReport,
  PlaybackSound,
  SoundChannel,
} from "@orpheum/sdk";
import { longestSoundOffsetMs } from "@orpheum/player";
import { useRef, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { audioTrackLabel } from "./audio-tracks.tsx";
import { playInCast } from "./cast.ts";
import { HowItPlays } from "./how-it-plays.tsx";
import { ListEntry } from "./list-box.tsx";
import { PlayOriginal } from "./play-original.tsx";
import type { PlayerSound, SequenceMoves } from "./player-controls.tsx";
import {
  KeptOpen,
  MoreMenuOf,
  SoundTrackMenuOf,
  SubtitlesMenuOf,
  VolumeMenu,
} from "./player-menus.tsx";
import { currentLabelOf, type ActionChoice } from "./route-chooser.tsx";
import { useServices } from "./services.tsx";
import { soundOffsetStepMs, TimingRow } from "./sound-delay.tsx";
import { SessionTalk } from "./together-voice.tsx";
import { usePrevious } from "./around-file.ts";

/** What the remote's menus send the receiver: its sound's choices and its picture's (D434). */
export type ReceiverChoices = CarriesSoundChoices & CarriesPictureChoices;

/** The remote has no overlay to hold shown while a menu is open, as the film's has. */
export function noOverlay(): void {
  // Nothing over the remote waits on a menu.
}

/**
 * The cast's remote's menus, the full-screen overlay's (player-menus.tsx) over the receiver (D444,
 * amending D440 §5): 🔈, the row of sliders, the film's first, its level on the receiver apart from
 * the TV's own; Talk beside it in a Watch Together session; Subtitles and Sound track as the
 * receiver offers them; ⁝ with the receiver's sound timing, levelling, the dialogue boost, the
 * original and How it plays. Each choice is sent to the receiver in the cast's session (D434), which
 * carries it out as its own press and says how it stands after: a Fire TV's own controls are hidden
 * while it is controlled (D407 §12; the owner, VQ116's run). Neither the bare player nor Start over:
 * the bar's ⏮ covers the start (the owner, 2026-11-07). Each waits for the receiver to say it.
 */
export function RemoteMenus(props: {
  readonly playback: PlaybackReport | undefined;
  readonly connection: ReceiverChoices | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  const { playback, connection } = props;
  const film = useReceiverFilmSound(playback?.sound?.channels?.[0], connection);
  const receiver =
    connection === undefined
      ? undefined
      : { connection, sound: playback?.sound, picture: playback?.picture };
  return (
    <>
      {film === undefined ? null : (
        <VolumeMenu sound={film} filmTitle={t("cast.filmLevel")} onOpenChange={noOverlay} />
      )}
      <SessionTalk isOverVideo />
      {receiver === undefined ? null : <ReceiverTracks receiver={receiver} />}
      <MoreMenuOf onOpenChange={noOverlay}>
        {receiver === undefined ? null : <ReceiverMore receiver={receiver} />}
        <HowItPlays playback={playback} />
      </MoreMenuOf>
    </>
  );
}

/** The receiver as the remote's menus work it: its choices, and its sound and picture as it says. */
interface WorkedReceiver {
  readonly connection: ReceiverChoices;
  readonly sound: PlaybackSound | undefined;
  readonly picture: PlaybackPicture | undefined;
}

/** Subtitles and Sound track over the receiver's, as it offers them. */
function ReceiverTracks(props: { readonly receiver: WorkedReceiver }): JSX.Element {
  const { connection, sound, picture } = props.receiver;
  return (
    <>
      {picture === undefined ? null : (
        <ReceiverSubtitles picture={picture} connection={connection} />
      )}
      {sound === undefined ? null : <ReceiverSoundTrack sound={sound} connection={connection} />}
    </>
  );
}

/** ⁝'s places over the receiver: its sound's (ReceiverSoundPlaces), and the original where offered. */
function ReceiverMore(props: { readonly receiver: WorkedReceiver }): JSX.Element {
  const { connection, sound, picture } = props.receiver;
  return (
    <>
      {sound === undefined ? null : <ReceiverSoundPlaces sound={sound} connection={connection} />}
      {picture?.canPlayOriginal === true ? (
        <ListEntry>
          <PlayOriginal
            onChosen={() => {
              connection.choosePicture({ kind: "original" });
            }}
          />
        </ListEntry>
      ) : null}
    </>
  );
}

/** The receiver's subtitles as its own menu names them, Off first; nothing where it offers none. */
function ReceiverSubtitles(props: {
  readonly picture: PlaybackPicture;
  readonly connection: CarriesPictureChoices;
}): JSX.Element | null {
  const { t } = useTranslation();
  const { picture, connection } = props;
  if (picture.subtitles.length === 0) {
    return null;
  }
  const choose = (key: string | undefined) => (): void => {
    connection.choosePicture({ kind: "subtitles", key });
  };
  const options: ActionChoice[] = [
    { key: "", label: t("play.subtitles.off"), isCurrent: picture.subtitleKey === undefined },
    ...picture.subtitles.map((each) => ({
      key: each.key,
      label: each.label,
      isCurrent: each.key === picture.subtitleKey,
    })),
  ].map((each) => ({ ...each, onChoose: choose(each.key === "" ? undefined : each.key) }));
  return (
    <SubtitlesMenuOf options={options} current={currentLabelOf(options)} onOpenChange={noOverlay} />
  );
}

/** The receiver's audio tracks, where its file has more than one. */
function ReceiverSoundTrack(props: {
  readonly sound: PlaybackSound;
  readonly connection: CarriesSoundChoices;
}): JSX.Element | null {
  const { t, i18n } = useTranslation();
  const { sound, connection } = props;
  if (sound.audioTracks.length < 2) {
    return null;
  }
  const words = {
    unknownLanguage: t("play.audio.unknownLanguage"),
    commentary: t("play.audio.commentary"),
    described: t("play.audio.described"),
  };
  const options: ActionChoice[] = sound.audioTracks.map((track) => ({
    key: String(track.streamId),
    label: audioTrackLabel(track, i18n.language, words),
    isCurrent: track.streamId === sound.audioStreamId,
    onChoose: () => {
      connection.chooseSound({ kind: "audioTrack", streamId: track.streamId });
    },
  }));
  return <SoundTrackMenuOf options={options} onOpenChange={noOverlay} />;
}

/**
 * ⁝'s places for the receiver's sound: its timing, levelling, and the dialogue boost where its film
 * has a boosted mix.
 */
function ReceiverSoundPlaces(props: {
  readonly sound: PlaybackSound;
  readonly connection: CarriesSoundChoices;
}): JSX.Element {
  const { t } = useTranslation();
  const { sound, connection } = props;
  return (
    <>
      {sound.soundOffsetMs === undefined ? null : (
        <KeptOpen>
          <RemoteSoundOffset ms={sound.soundOffsetMs} connection={connection} />
        </KeptOpen>
      )}
      <ListEntry>
        <RemoteSwitch
          key={`levelling-${String(sound.isLevellingOn)}`}
          label={t("play.level.switch")}
          isOn={sound.isLevellingOn}
          onSwitched={(isOn) => {
            connection.chooseSound({ kind: "levelling", isOn });
          }}
        />
      </ListEntry>
      {sound.isBoostOn === undefined ? null : (
        <ListEntry>
          <RemoteSwitch
            key={`boost-${String(sound.isBoostOn)}`}
            label={t("play.boost")}
            isOn={sound.isBoostOn}
            onSwitched={(isOn) => {
              connection.chooseSound({ kind: "boost", isOn });
            }}
          />
        </ListEntry>
      )}
    </>
  );
}

/**
 * The receiver's sound timing, for a TV that plays its sound late (the owner, 2026-11-06: "Put a
 * control in watch together and cast so that I can adjust the sound timing"): as the receiver said
 * it stands, each press moving it a step from there, which the receiver keeps with its device as its
 * own press. As an entry of ⁝'s list, its text on a line of its own, its buttons in a row below, as
 * the player's (SoundOffsetControl).
 */
function RemoteSoundOffset(props: {
  readonly ms: number;
  readonly connection: CarriesSoundChoices;
}): JSX.Element {
  const { t } = useTranslation();
  const { ms, connection } = props;
  const move = (byMs: number): void => {
    const next = Math.min(Math.max(ms + byMs, -longestSoundOffsetMs), longestSoundOffsetMs);
    connection.chooseSound({ kind: "soundOffset", ms: next });
  };
  let shown = t("cast.soundOffset.inStep");
  if (ms !== 0) {
    shown =
      ms > 0 ? t("cast.soundOffset.laterBy", { ms }) : t("cast.soundOffset.earlierBy", { ms: -ms });
  }
  return (
    <TimingRow isInAList text={shown} notes={null}>
      <button
        type="button"
        onClick={() => {
          move(-soundOffsetStepMs);
        }}
      >
        {t("play.soundOffset.earlier", { ms: soundOffsetStepMs })}
      </button>
      <button
        type="button"
        onClick={() => {
          move(soundOffsetStepMs);
        }}
      >
        {t("play.soundOffset.later", { ms: soundOffsetStepMs })}
      </button>
    </TimingRow>
  );
}

/**
 * One of the receiver's switches on the remote: as the receiver said it stands, and as pressed here
 * until the receiver says again (its key changes with what it says).
 */
function RemoteSwitch(props: {
  readonly label: string;
  readonly isOn: boolean;
  readonly onSwitched: (isOn: boolean) => void;
}): JSX.Element {
  const [isOn, setIsOn] = useState(props.isOn);
  return (
    <label className="sound__switch">
      <input
        type="checkbox"
        checked={isOn}
        onChange={(event) => {
          const isNowOn = event.currentTarget.checked;
          setIsOn(isNowOn);
          props.onSwitched(isNowOn);
        }}
      />
      {props.label}
    </label>
  );
}

/** A channel as chosen on the remote (next), and as the receiver said it when it was (said). */
interface ChosenChannel {
  readonly said: SoundChannel;
  readonly next: SoundChannel;
}

/**
 * The film's level and mute on the receiver (D440 §2, §5), as 🔈's film slider works them: its
 * channel there, apart from the TV's own volume, lasting for the session, not saved. As the receiver
 * said them, and as moved here until it says them again; each move sent to the receiver, which
 * carries it out. Undefined while the receiver says no film channel (film).
 */
function useReceiverFilmSound(
  film: SoundChannel | undefined,
  connection: CarriesSoundChoices | undefined,
): PlayerSound | undefined {
  // What was chosen here, with what the receiver said when it was: shown until the receiver says
  // anything else. The ref is read by a move made in the same press as another (a drag up from
  // muted sets the level, then sound on), before the render that shows the first.
  const [chosen, setChosen] = useState<ChosenChannel>();
  const chosenRef = useRef<ChosenChannel | undefined>(undefined);
  if (film === undefined || connection === undefined) {
    return undefined;
  }
  const isStill = (made: ChosenChannel | undefined): made is ChosenChannel =>
    made?.said.level === film.level && made.said.isMuted === film.isMuted;
  const shown = isStill(chosen) ? chosen.next : film;
  const choose = (change: Partial<Pick<SoundChannel, "level" | "isMuted">>): void => {
    const latest = chosenRef.current;
    const next = { ...(isStill(latest) ? latest.next : film), ...change };
    const made = { said: film, next };
    chosenRef.current = made;
    setChosen(made);
    connection.chooseSound({ kind: "channel", ...next });
  };
  return {
    volume: shown.level,
    isMuted: shown.isMuted,
    canSetVolume: true,
    setVolume: (level) => {
      choose({ level });
    },
    setMuted: (isMuted) => {
      choose({ isMuted });
    },
  };
}

/**
 * The remote's ⏮ (D440 §5), as the player's (D407 §2): the episode before the cast's film, which the
 * receiver is handed to play (D364 §3); none, and no button, where nothing plays before it, as on
 * the player (the plan's "restart or previous" names no restart the player has).
 */
export function useRemotePrevious(sent: CastSending): SequenceMoves {
  const { api } = useServices();
  const previous = usePrevious(sent.fileId);
  if (previous === undefined || previous.unavailable !== undefined) {
    return {};
  }
  const { fileId } = previous;
  return {
    previous: () => {
      playInCast(api, sent.castId, { fileId }).catch(() => undefined); // the cast goes on as it was
    },
  };
}
