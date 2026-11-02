// SPDX-License-Identifier: MPL-2.0

import "./player-menus.css";

import { seekTo } from "@orpheum/player";
import {
  createContext,
  use,
  useCallback,
  useEffect,
  useState,
  type JSX,
  type ReactNode,
  type RefObject,
} from "react";
import { useTranslation } from "react-i18next";

import { audioTrackLabel, type AudioChoice } from "./audio-tracks.tsx";
import { DropDown, keepsListOpenAttribute } from "./drop-down.tsx";
import { LevellingControl } from "./levelling.tsx";
import { ModalDialog } from "./modal-dialog.tsx";
import { PlayOriginal } from "./play-original.tsx";
import { SoundControls, type PlayerSound } from "./player-controls.tsx";
import { AudioIcon, MoreIcon, MutedIcon, SoundIcon, SubtitlesIcon } from "./player-icons.tsx";
import type { OverlayControl } from "./player-overlay-state.ts";
import { PlayerTracksContext, type PlayerTracks } from "./player-tracks.tsx";
import { ActionChooser, type ActionChoice } from "./route-chooser.tsx";
import { BoostBox } from "./sound.tsx";
import { SoundOffsetControl } from "./sound-delay.tsx";
import { FindSubtitles } from "./subtitle-search.tsx";
import { OffsetControl, trackKey, useTrackLabel, type SubtitleChoice } from "./subtitles.tsx";

/**
 * What the page playing gives the overlay's menus beside the film's tracks (PlayerTracksContext):
 * the file, the device's sound timing it started with, and what follows a subtitle fetched, the
 * original chosen and the bare player asked for (the last two where they are offered).
 */
export interface PlayerMenusWays {
  readonly fileId: number;
  readonly soundOffsetMs: number;
  readonly onSubtitleFetched: () => void;
  readonly onOriginalChosen?: (() => void) | undefined;
  readonly onBare?: (() => void) | undefined;
}

/** The page's ways for the overlay's menus; undefined where the page gives none, a cast's receiver. */
export const PlayerMenusContext = createContext<PlayerMenusWays | undefined>(undefined);

/** The menu open now, if one is: 🔈, 💬, 🎧, ⁝, or the Find subtitles dialog 💬 opened. */
type OpenMenu = "volume" | "subtitles" | "audio" | "more" | "find";

/**
 * The full-screen overlay's menus, in row 2's trailing group before full screen (D407 §1, §8): 🔈,
 * the volume upright and mute, where the overlay shows it (D407 §10); 💬, Off, the film's subtitle
 * tracks and Find subtitles; 🎧, the audio tracks, where there are more than one; ⁝, the rest of
 * what lies under the film outside full screen. Each reads and writes the film's choices in
 * PlayerTracksContext, so a choice here is the one under the film. onOpenChange is told whether one
 * is open, for the overlay to hold its time and leave its presses to it (D407 §6). Find subtitles is
 * a dialog of its own, which a change of full screen closes, as it does a menu. No track's menu is
 * drawn where the page gives no tracks.
 */
export function PlayerMenus(props: {
  readonly videoRef: RefObject<HTMLVideoElement | null>;
  /** The overlay's controls on this device (controlsShown): which menus to draw. */
  readonly shown: readonly OverlayControl[];
  /** The film's sound, which 🔈 sets. */
  readonly sound?: PlayerSound | undefined;
  readonly onOpenChange: (isOpen: boolean) => void;
}): JSX.Element {
  const tracks = use(PlayerTracksContext);
  const ways = use(PlayerMenusContext);
  const [open, setOpen] = useState<OpenMenu | undefined>(undefined);
  const { onOpenChange, sound } = props;
  const isOpen = open !== undefined;
  useEffect(() => {
    onOpenChange(isOpen);
  }, [isOpen, onOpenChange]);
  const endFinding = useFindingEndedAtFullScreen(setOpen);
  const toldOf =
    (menu: OpenMenu) =>
    (isNowOpen: boolean): void => {
      setOpen((now) => openAfter(now, menu, isNowOpen));
    };
  const findSubtitles =
    ways === undefined
      ? undefined
      : (): void => {
          setOpen("find");
        };
  return (
    <>
      {props.shown.includes("sound") && sound !== undefined ? (
        <VolumeMenu sound={sound} onOpenChange={toldOf("volume")} />
      ) : null}
      {tracks === undefined ? null : (
        <TrackMenus
          tracks={tracks}
          ways={ways}
          videoRef={props.videoRef}
          shown={props.shown}
          toldOf={toldOf}
          onFind={findSubtitles}
        />
      )}
      {open === "find" && ways !== undefined ? (
        <FindSubtitlesDialog ways={ways} onClose={endFinding} />
      ) : null}
    </>
  );
}

/** 💬, 🎧 and ⁝ among those shown, over the film's tracks (PlayerMenus). */
function TrackMenus(props: {
  readonly tracks: PlayerTracks;
  readonly ways: PlayerMenusWays | undefined;
  readonly videoRef: RefObject<HTMLVideoElement | null>;
  readonly shown: readonly OverlayControl[];
  readonly toldOf: (menu: OpenMenu) => (isNowOpen: boolean) => void;
  readonly onFind: (() => void) | undefined;
}): JSX.Element {
  const { tracks, shown, toldOf } = props;
  return (
    <>
      {shown.includes("subtitles") ? (
        <SubtitlesMenu
          choice={tracks.subtitles}
          onFind={props.onFind}
          onOpenChange={toldOf("subtitles")}
        />
      ) : null}
      {shown.includes("audio") ? (
        <AudioMenuOverTheFilm choice={tracks.audio} onOpenChange={toldOf("audio")} />
      ) : null}
      {shown.includes("more") ? (
        <MoreMenu
          tracks={tracks}
          ways={props.ways}
          videoRef={props.videoRef}
          onOpenChange={toldOf("more")}
        />
      ) : null}
    </>
  );
}

/**
 * 🔈 (D407 §10, Q11): a pop-up with the volume upright and mute, both keeping it open
 * (keepsListOpenAttribute), as + and − move the volume from anywhere (player-commands.ts).
 */
function VolumeMenu(props: {
  readonly sound: PlayerSound;
  readonly onOpenChange: (isOpen: boolean) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { sound } = props;
  const isSilent = sound.isMuted || sound.volume === 0;
  return (
    <DropDown
      label={t("playerControls.volume")}
      opener={isSilent ? <MutedIcon /> : <SoundIcon />}
      openerClassName="icon-button icon-button--over-video"
      openerNaming={{ "aria-label": t("playerControls.volume") }}
      listClassName="player-menus__volume"
      isFromItsTop
      onOpenChange={props.onOpenChange}
    >
      <KeptOpen>
        <SoundControls sound={sound} isOverVideo={false} isUpright />
      </KeptOpen>
    </DropDown>
  );
}

/** The menu open after menu tells it opened or closed, now open before. */
function openAfter(
  now: OpenMenu | undefined,
  menu: OpenMenu,
  isNowOpen: boolean,
): OpenMenu | undefined {
  if (isNowOpen) {
    return menu;
  }
  return now === menu ? undefined : now;
}

/** Ends Find subtitles' dialog, as a change of full screen does; returns the way to end it. */
function useFindingEndedAtFullScreen(
  setOpen: (open: (now: OpenMenu | undefined) => OpenMenu | undefined) => void,
): () => void {
  const endFinding = useCallback(() => {
    setOpen((now) => (now === "find" ? undefined : now));
  }, [setOpen]);
  useEffect(() => {
    document.addEventListener("fullscreenchange", endFinding);
    return () => {
      document.removeEventListener("fullscreenchange", endFinding);
    };
  }, [endFinding]);
  return endFinding;
}

/** 💬: Off, the film's subtitle tracks, then Find subtitles where the page offers it (D407 §8). */
function SubtitlesMenu(props: {
  readonly choice: SubtitleChoice;
  readonly onFind: (() => void) | undefined;
  readonly onOpenChange: (isOpen: boolean) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const labelOf = useTrackLabel();
  const { tracks, chosenTrackKey, choose } = props.choice;
  const off: ActionChoice = {
    key: "off",
    label: t("play.subtitles.off"),
    isCurrent: chosenTrackKey === undefined,
    onChoose: () => {
      choose(undefined);
    },
  };
  const trackChoices = tracks.map((track): ActionChoice => {
    const key = trackKey(track);
    const label = labelOf(track);
    return {
      key: key ?? label,
      label,
      isCurrent: key !== undefined && key === chosenTrackKey,
      onChoose: () => {
        choose(key);
      },
    };
  });
  const { onFind } = props;
  const find: ActionChoice[] =
    onFind === undefined
      ? []
      : [{ key: "find", label: t("play.findSubtitles.open"), isCurrent: false, onChoose: onFind }];
  const options = [off, ...trackChoices, ...find];
  return (
    <ActionChooser
      fieldLabel={t("play.subtitles.label")}
      current={options.find((option) => option.isCurrent)?.label ?? off.label}
      openerIcon={<SubtitlesIcon />}
      isOverVideo
      onOpenChange={props.onOpenChange}
      options={options}
    />
  );
}

/** 🎧: the film's audio tracks (D407 §8), named by the one playing: the chosen, else the first. */
function AudioMenuOverTheFilm(props: {
  readonly choice: AudioChoice;
  readonly onOpenChange: (isOpen: boolean) => void;
}): JSX.Element {
  const { t, i18n } = useTranslation();
  const { tracks, chosenStreamId, choose } = props.choice;
  const words = {
    unknownLanguage: t("play.audio.unknownLanguage"),
    commentary: t("play.audio.commentary"),
    described: t("play.audio.described"),
  };
  // With no choice marked, the playback plays its first track, as the select under the film shows.
  const playingId = chosenStreamId ?? tracks[0]?.streamId;
  const options = tracks.map((track): ActionChoice => ({
    key: String(track.streamId),
    label: audioTrackLabel(track, i18n.language, words),
    isCurrent: track.streamId === playingId,
    onChoose: () => {
      choose(track);
    },
  }));
  return (
    <ActionChooser
      fieldLabel={t("play.audio.label")}
      current={options.find((option) => option.isCurrent)?.label ?? ""}
      openerIcon={<AudioIcon />}
      isOverVideo
      onOpenChange={props.onOpenChange}
      options={options}
    />
  );
}

/**
 * ⁝ (D407 §8, Q8): Start over, the subtitles' offset, the device's sound timing, levelling, the
 * dialogue boost, the original and the bare player where they are offered. Its steppers' presses
 * keep it open (keepsListOpenAttribute); its other buttons close it, as a choice does.
 */
function MoreMenu(props: {
  readonly tracks: PlayerTracks;
  readonly ways: PlayerMenusWays | undefined;
  readonly videoRef: RefObject<HTMLVideoElement | null>;
  readonly onOpenChange: (isOpen: boolean) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { tracks, ways, videoRef } = props;
  const { subtitles, boost } = tracks;
  return (
    <DropDown
      label={t("playerControls.more")}
      opener={<MoreIcon />}
      openerClassName="icon-button icon-button--over-video"
      openerNaming={{ "aria-label": t("playerControls.more") }}
      listClassName="player-menus__more"
      isFromItsTop
      onOpenChange={props.onOpenChange}
    >
      <li>
        <StartOver videoRef={videoRef} />
      </li>
      {subtitles.tracks.length === 0 ? null : (
        <KeptOpen>
          <OffsetControl offsetMs={subtitles.offsetMs} onMove={subtitles.shiftOffset} />
        </KeptOpen>
      )}
      {ways === undefined ? null : (
        <KeptOpen>
          <SoundOffsetControl videoRef={videoRef} initialMs={ways.soundOffsetMs} />
        </KeptOpen>
      )}
      {ways === undefined ? null : (
        <li>
          <LevellingControl fileId={ways.fileId} level={tracks.level} state={tracks.levelling} />
        </li>
      )}
      {boost === undefined ? null : (
        <li>
          <BoostBox state={boost} />
        </li>
      )}
      <MoreOfThePage ways={ways} />
    </DropDown>
  );
}

/** ⁝'s places the page offers where it may: the original (a debug tool), the bare player. */
function MoreOfThePage(props: { readonly ways: PlayerMenusWays | undefined }): JSX.Element {
  const { t } = useTranslation();
  const onOriginalChosen = props.ways?.onOriginalChosen;
  const onBare = props.ways?.onBare;
  return (
    <>
      {onOriginalChosen === undefined ? null : (
        <li>
          <PlayOriginal onChosen={onOriginalChosen} />
        </li>
      )}
      {onBare === undefined ? null : (
        <li>
          <button type="button" onClick={onBare}>
            {t("play.bare.open")}
          </button>
        </li>
      )}
    </>
  );
}

/** A place of ⁝ whose buttons step a value, which keep the list open for the next press. */
function KeptOpen(props: { readonly children: ReactNode }): JSX.Element {
  return <li {...{ [keepsListOpenAttribute]: "" }}>{props.children}</li>;
}

/** Find subtitles, opened from 💬 in a dialog of its own, closing once one is fetched (D407 §8). */
function FindSubtitlesDialog(props: {
  readonly ways: PlayerMenusWays;
  readonly onClose: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { ways, onClose } = props;
  return (
    <ModalDialog label={t("play.findSubtitles.heading")} onClose={onClose}>
      <FindSubtitles
        fileId={ways.fileId}
        isOpenAtOnce
        onFetched={() => {
          onClose();
          ways.onSubtitleFetched();
        }}
      />
    </ModalDialog>
  );
}

/** The button that takes the film back to its start, under the film and in ⁝. */
export function StartOver(props: {
  readonly videoRef: RefObject<HTMLVideoElement | null>;
}): JSX.Element {
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
