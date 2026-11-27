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
import { useEffect, useRef, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { audioTrackLabel } from "./audio-tracks.tsx";
import { useReportFromTheRemote, type RemoteReport } from "./cast-remote-report.tsx";
import { playInCast } from "./cast.ts";
import { HowItPlays } from "./how-it-plays.tsx";
import { ListEntry } from "./list-box.tsx";
import { PlayOriginal } from "./play-original.tsx";
import type { PlayerSound } from "./playback-target.ts";
import type { SequenceMoves } from "./player-controls.tsx";
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
import { useAsPressed } from "./as-pressed.ts";
import { Switch } from "./switch.tsx";
import { SessionTalk } from "./together-voice.tsx";
import type { Previous } from "./around-file.ts";
import { RemoteFindSubtitles, RemoteFindSubtitlesOption } from "./cast-remote-find-subtitles.tsx";
import { RemoteFileTiming, RemoteTellTheAdmin } from "./cast-remote-sound-timing.tsx";
import { RemoteSubtitleTiming } from "./cast-remote-subtitle-timing.tsx";
import { useLive } from "./live.tsx";
import { useKeptByTheSender, type ReceiverChoices } from "./cast-remote-keep.ts";
import { StatusNote } from "./status-note.tsx";
import type { SuggestedSoundOffset } from "./problem-report.tsx";

/** The dialog a remote's menu opened, if one did: Find subtitles, or Tell the admin's report. */
type RemoteDialog =
  | { readonly kind: "findSubtitles" }
  | { readonly kind: "tellTheAdmin"; readonly suggested: SuggestedSoundOffset };

/** The remote has no overlay to hold shown while a menu is open, as the film's has. */
export function noOverlay(): void {
  // Nothing over the remote waits on a menu.
}

/**
 * The cast's remote's menus, the full-screen overlay's (player-menus.tsx) over the receiver (D444,
 * amending D440 §5), drawn as the player's are wherever the receiver can do what they do (the owner,
 * 2026-11-23: "I want the interfaces to look as similar as possible, aside from things that can't be
 * done due to a device or circumstances"): Talk in a Watch Together session; 💬,
 * the receiver's subtitle tracks, their timing and Find subtitles; 🎧, its sound track, Level
 * loudness, the dialogue boost and, under Sound timing, That device's timing and This file's;
 * ⁝, the original and How it plays. Each choice is sent to the
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
  /** The film and session ⁝'s Report a problem takes the receiver's playback from (D524 §2). */
  readonly report?: RemoteReport | undefined;
}): JSX.Element {
  const { playback } = props;
  const reporting = useReportFromTheRemote(props.report, playback);
  const [dialog, setDialog] = useState<RemoteDialog | undefined>(undefined);
  const { sending } = useLive();
  const fileId = sending?.fileId ?? 0;
  // Each choice the person keeps, kept here where the receiver cannot keep it (cast-remote-keep.ts).
  const { connection, problem } = useKeptByTheSender(props.connection, sending);
  const closed = (): void => {
    setDialog(undefined);
  };
  return (
    <>
      {reporting.dialog}
      <SessionTalk isOverVideo />
      {connection === undefined ? null : (
        <ReceiverChoicesMenus
          playback={playback}
          connection={connection}
          fileId={fileId}
          onDialog={setDialog}
        />
      )}
      <ReceiverMore playback={playback} connection={connection} report={reporting.entry} />
      {problem === undefined ? null : <StatusNote told={{ kind: "problem", text: problem }} />}
      {connection === undefined || dialog === undefined ? null : (
        <RemoteMenuDialog
          dialog={dialog}
          fileId={fileId}
          connection={connection}
          onClose={closed}
        />
      )}
    </>
  );
}

/** The dialog a remote's menu opened, over the receiver's file. */
function RemoteMenuDialog(props: {
  readonly dialog: RemoteDialog;
  readonly fileId: number;
  readonly connection: CarriesPictureChoices;
  readonly onClose: () => void;
}): JSX.Element {
  const { dialog, onClose } = props;
  return dialog.kind === "findSubtitles" ? (
    <RemoteFindSubtitles fileId={props.fileId} connection={props.connection} onClose={onClose} />
  ) : (
    <RemoteTellTheAdmin suggested={dialog.suggested} onClose={onClose} />
  );
}

/** 💬 and 🎧 over what the receiver says of its picture and its sound, each where it says it. */
function ReceiverChoicesMenus(props: {
  readonly playback: PlaybackReport | undefined;
  readonly connection: ReceiverChoices;
  /** The receiver's file, 0 while the remote knows none. */
  readonly fileId: number;
  readonly onDialog: (dialog: RemoteDialog) => void;
}): JSX.Element {
  const { connection, fileId, onDialog } = props;
  const picture = props.playback?.picture;
  const sound = props.playback?.sound;
  return (
    <>
      {picture === undefined ? null : (
        <ReceiverSubtitles
          picture={picture}
          connection={connection}
          onFind={
            fileId === 0
              ? undefined
              : () => {
                  onDialog({ kind: "findSubtitles" });
                }
          }
        />
      )}
      {sound === undefined ? null : (
        <ReceiverSound
          sound={sound}
          connection={connection}
          onTellTheAdmin={(soundOffsetMs) => {
            onDialog({ kind: "tellTheAdmin", suggested: { fileId, soundOffsetMs } });
          }}
        />
      )}
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
  /** ⁝'s Report a problem, in the menu where the receiver has said how it plays. */
  readonly report: JSX.Element | null;
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
      {props.report}
      <HowItPlays playback={playback} />
    </MoreMenuOf>
  );
}

/**
 * 💬 over the receiver's subtitles as its own menu names them, Off first, their timing under them,
 * and Find subtitles where the remote knows the file, as the player's Subtitles menu is; nothing
 * where it offers no track and none can be found.
 */
function ReceiverSubtitles(props: {
  readonly picture: PlaybackPicture;
  readonly connection: CarriesPictureChoices;
  readonly onFind: (() => void) | undefined;
}): JSX.Element | null {
  const { t } = useTranslation();
  const { picture, connection, onFind } = props;
  if (picture.subtitles.length === 0 && onFind === undefined) {
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
      {picture.subtitles.length === 0 ? null : (
        <RemoteSubtitleTiming offsetMs={picture.subtitleOffsetMs} connection={connection} />
      )}
      {onFind === undefined ? null : <RemoteFindSubtitlesOption onFind={onFind} />}
    </SubtitlesMenuOf>
  );
}

/**
 * 🎧 over the receiver's sound, as the player's Sound menu is (sound-menu.tsx): its sound track
 * where its file has more than one, Level loudness, the dialogue boost where its film has a boosted
 * mix, and, under Sound timing, That device's timing and the file's own (D537), each where it says
 * one, the file's opening Tell the admin's report by onTellTheAdmin.
 */
function ReceiverSound(props: {
  readonly sound: PlaybackSound;
  readonly connection: CarriesSoundChoices;
  readonly onTellTheAdmin: (ownMs: number) => void;
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
            label={t("play.boost")}
            isOn={sound.isBoostOn}
            onSwitched={(isOn) => {
              connection.chooseSound({ kind: "boost", isOn });
            }}
          />
        </ListEntry>
      )}
      <ReceiverSoundTiming
        sound={sound}
        connection={connection}
        onTellTheAdmin={props.onTellTheAdmin}
      />
    </SoundMenuOf>
  );
}

/** 🎧's Sound timing on the remote: That device's timing and This file's, each where it says one. */
function ReceiverSoundTiming(props: {
  readonly sound: PlaybackSound;
  readonly connection: CarriesSoundChoices;
  readonly onTellTheAdmin: (ownMs: number) => void;
}): JSX.Element | null {
  const { sound, connection } = props;
  if (sound.soundOffsetMs === undefined && sound.fileSoundOffset === undefined) {
    return null;
  }
  return (
    <>
      <SoundTimingHeading />
      {sound.soundOffsetMs === undefined ? null : (
        <KeptOpen>
          <RemoteSoundOffset ms={sound.soundOffsetMs} connection={connection} />
        </KeptOpen>
      )}
      {sound.fileSoundOffset === undefined ? null : (
        <RemoteFileTiming
          offset={sound.fileSoundOffset}
          connection={connection}
          onTellTheAdmin={props.onTellTheAdmin}
        />
      )}
    </>
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
  const { connection } = props;
  const [ms, press] = useAsPressed(props.ms, (next) => {
    connection.chooseSound({ kind: "soundOffset", ms: next });
  });
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
      onChange={press}
    />
  );
}

/**
 * One of the receiver's switches on the remote (switch.tsx): as the receiver said it stands, and as
 * pressed here until the receiver says it (useAsPressed).
 */
function RemoteSwitch(props: {
  readonly label: string;
  readonly isOn: boolean;
  readonly onSwitched: (isOn: boolean) => void;
}): JSX.Element {
  const [isOn, press] = useAsPressed(props.isOn, props.onSwitched);
  return <Switch label={props.label} isOn={isOn} onSwitch={press} />;
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
  // muted sets the level, then sound on), before the render that shows the first; it ends with the
  // choice.
  const [chosen, setChosen] = useState<ChosenChannel>();
  const chosenRef = useRef<ChosenChannel | undefined>(undefined);
  useEffect(() => {
    if (chosen === undefined) {
      chosenRef.current = undefined;
    }
  }, [chosen]);
  if (film === undefined || connection === undefined) {
    return undefined;
  }
  const isStill = (made: ChosenChannel | undefined): made is ChosenChannel =>
    made?.said.level === film.level && made.said.isMuted === film.isMuted;
  if (chosen !== undefined && !isStill(chosen)) {
    // The receiver has said something else: the choice is over, and is not shown again when the
    // receiver comes back to what it said then (its TV's own remote putting the volume back).
    setChosen(undefined);
  }
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
