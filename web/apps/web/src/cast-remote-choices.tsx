// SPDX-License-Identifier: MPL-2.0

import "./play-words.ts";
import "./cast-words.ts";

import type {
  CarriesPictureChoices,
  CarriesSoundChoices,
  CastSending,
  PlaybackPicture,
  PlaybackReport,
  PlaybackSound,
  SoundChannel,
} from "@orpheum/sdk";
import { useRef, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { audioTrackLabel } from "./audio-tracks.tsx";
import { playInCast } from "./cast.ts";
import { HowItPlays } from "./how-it-plays.tsx";
import { ListEntry } from "./list-box.tsx";
import { PlayOriginal } from "./play-original.tsx";
import type { PlayerSound, SequenceMoves } from "./player-controls.tsx";
import {
  ChoicesSubmenu,
  KeptOpen,
  MoreMenuOf,
  SoundMenuOf,
  SubtitlesMenuOf,
} from "./over-video-menu.tsx";
import type { ActionChoice } from "./route-chooser.tsx";
import { SoundTimingHeading } from "./sound-menu.tsx";
import { useServices } from "./services.tsx";
import { SoundOffsetInput } from "./sound-delay.tsx";
import { Switch } from "./switch.tsx";
import { SessionTalk } from "./together-voice.tsx";
import type { Previous } from "./around-file.ts";

/** What the remote's menus send the receiver: its sound's choices and its picture's (D434). */
export type ReceiverChoices = CarriesSoundChoices & CarriesPictureChoices;

/** The remote has no overlay to hold shown while a menu is open, as the film's has. */
export function noOverlay(): void {
  // Nothing over the remote waits on a menu.
}

/**
 * The cast's remote's menus, the full-screen overlay's (player-menus.tsx) over the receiver (D444,
 * amending D440 §5), drawn as the player's are wherever the receiver can do what they do (the owner,
 * 2026-11-23: "I want the interfaces to look as similar as possible, aside from things that can't be
 * done due to a device or circumstances"): Talk in a Watch Together session; 💬,
 * the receiver's subtitle tracks; 🎧, its sound track, Level loudness, the dialogue boost and, under
 * Sound timing, That device's timing; ⁝, the original and How it plays. Each choice is sent to the
 * receiver in the cast's session (D434), which carries it out as its own press and says how it stands
 * after: a Fire TV's own controls are hidden while it is controlled (D407 §12; the owner, VQ116's
 * run). Neither the bare player nor Start over: the bar's ⏮ covers the start (the owner,
 * 2026-11-07). A menu with nothing in it is not drawn (the owner: "Any button that would have nothing
 * in its submenu should not display at all"). Each waits for the receiver to say it. No 🔈: the
 * film's level on the receiver is the pad's (useReceiverFilmSound, D548 §6).
 */
export function RemoteMenus(props: {
  readonly playback: PlaybackReport | undefined;
  readonly connection: ReceiverChoices | undefined;
}): JSX.Element {
  const { playback, connection } = props;
  return (
    <>
      <SessionTalk isOverVideo />
      {connection === undefined ? null : (
        <ReceiverChoicesMenus playback={playback} connection={connection} />
      )}
      <ReceiverMore playback={playback} connection={connection} />
    </>
  );
}

/** 💬 and 🎧 over what the receiver says of its picture and its sound, each where it says it. */
function ReceiverChoicesMenus(props: {
  readonly playback: PlaybackReport | undefined;
  readonly connection: ReceiverChoices;
}): JSX.Element {
  const { connection } = props;
  const picture = props.playback?.picture;
  const sound = props.playback?.sound;
  return (
    <>
      {picture === undefined ? null : (
        <ReceiverSubtitles picture={picture} connection={connection} />
      )}
      {sound === undefined ? null : <ReceiverSound sound={sound} connection={connection} />}
    </>
  );
}

/**
 * ⁝ over the receiver's film: the original where its file can play as it is, and How it plays;
 * nothing until the receiver says how it plays or that the original can.
 */
function ReceiverMore(props: {
  readonly playback: PlaybackReport | undefined;
  readonly connection: ReceiverChoices | undefined;
}): JSX.Element | null {
  const { playback, connection } = props;
  const original =
    connection !== undefined && playback?.picture?.canPlayOriginal === true
      ? connection
      : undefined;
  if (playback === undefined && original === undefined) {
    return null;
  }
  return (
    <MoreMenuOf onOpenChange={noOverlay}>
      {original === undefined ? null : (
        <ListEntry>
          <PlayOriginal
            onChosen={() => {
              original.choosePicture({ kind: "original" });
            }}
          />
        </ListEntry>
      )}
      <HowItPlays playback={playback} />
    </MoreMenuOf>
  );
}

/** 💬 over the receiver's subtitles as its own menu names them, Off first; nothing where it offers none. */
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
    <SubtitlesMenuOf onOpenChange={noOverlay}>
      <ChoicesSubmenu name={t("play.subtitles.label")} choices={options} />
    </SubtitlesMenuOf>
  );
}

/**
 * 🎧 over the receiver's sound, as the player's Sound menu is (sound-menu.tsx): its sound track
 * where its file has more than one, Level loudness, the dialogue boost where its film has a boosted
 * mix, and, under Sound timing, That device's timing where it says one. The file's own timing is the
 * player's alone until a receiver takes it (plans: the next batch).
 */
function ReceiverSound(props: {
  readonly sound: PlaybackSound;
  readonly connection: CarriesSoundChoices;
}): JSX.Element {
  const { t, i18n } = useTranslation();
  const { sound, connection } = props;
  const words = {
    unknownLanguage: t("play.audio.unknownLanguage"),
    commentary: t("play.audio.commentary"),
    described: t("play.audio.described"),
  };
  const tracks: ActionChoice[] = sound.audioTracks.map((track) => ({
    key: String(track.streamId),
    label: audioTrackLabel(track, i18n.language, words),
    isCurrent: track.streamId === sound.audioStreamId,
    onChoose: () => {
      connection.chooseSound({ kind: "audioTrack", streamId: track.streamId });
    },
  }));
  return (
    <SoundMenuOf onOpenChange={noOverlay}>
      <ChoicesSubmenu name={t("play.audio.menu")} choices={tracks} />
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
      {sound.soundOffsetMs === undefined ? null : (
        <>
          <SoundTimingHeading />
          <KeptOpen>
            <RemoteSoundOffset ms={sound.soundOffsetMs} connection={connection} />
          </KeptOpen>
        </>
      )}
    </SoundMenuOf>
  );
}

/**
 * The receiver's sound timing, for a TV that plays its sound late (the owner, 2026-11-06: "Put a
 * control in watch together and cast so that I can adjust the sound timing"): as the receiver said
 * it stands, each arrow moving it a step from there or a number typed setting it, which the receiver
 * keeps with its device as its own press. As an entry of 🎧's list under Sound timing, the player's
 * stepper (SoundOffsetInput), "That device" leading it, as the player's "This device" does.
 */
function RemoteSoundOffset(props: {
  readonly ms: number;
  readonly connection: CarriesSoundChoices;
}): JSX.Element {
  const { t } = useTranslation();
  const { ms, connection } = props;
  let shown = t("cast.soundOffset.inStep");
  if (ms !== 0) {
    shown =
      ms > 0 ? t("cast.soundOffset.laterBy", { ms }) : t("cast.soundOffset.earlierBy", { ms: -ms });
  }
  return (
    <SoundOffsetInput
      label={t("cast.soundOffset.thatDevice")}
      fieldLabel={t("cast.soundOffset.field")}
      valueText={shown}
      ms={ms}
      stepLabels={{
        earlier: t("cast.soundOffset.thatDeviceEarlier"),
        later: t("cast.soundOffset.thatDeviceLater"),
      }}
      onChange={(next) => {
        connection.chooseSound({ kind: "soundOffset", ms: next });
      }}
    />
  );
}

/**
 * One of the receiver's switches on the remote (switch.tsx): as the receiver said it stands, and as pressed here
 * until the receiver says again (its key changes with what it says).
 */
function RemoteSwitch(props: {
  readonly label: string;
  readonly isOn: boolean;
  readonly onSwitched: (isOn: boolean) => void;
}): JSX.Element {
  const [isOn, setIsOn] = useState(props.isOn);
  return (
    <Switch
      label={props.label}
      isOn={isOn}
      onSwitch={(isNowOn) => {
        setIsOn(isNowOn);
        props.onSwitched(isNowOn);
      }}
    />
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
export function useReceiverFilmSound(
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
 * The remote's ⏮ (D440 §5), as the player's (D407 §2): the episode before the cast's film, previous,
 * as the remote's one read of what plays around it says (D477 §2), which the receiver is handed to
 * play (D364 §3); none, and no button, where nothing plays before it, as on the player (the plan's
 * "restart or previous" names no restart the player has).
 */
export function useRemotePrevious(
  sent: CastSending,
  previous: Previous | undefined,
): SequenceMoves {
  const { api } = useServices();
  if (previous === undefined || previous.unavailable !== undefined) {
    return {};
  }
  const { fileId, itemId } = previous;
  return {
    previous: () => {
      playInCast(api, sent.castId, { fileId, itemId }).catch(() => undefined); // the cast goes on as it was
    },
  };
}
