// SPDX-License-Identifier: MPL-2.0

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

import { useAudioChoices, type AudioChoice } from "./audio-tracks.tsx";
import { DropDown } from "./drop-down.tsx";
import { ForTestersContext, ForTestersShown } from "./for-testers.tsx";
import { ListEntry, ListOption } from "./list-box.tsx";
import { LevellingControl } from "./levelling.tsx";
import { ModalDialog } from "./modal-dialog.tsx";
import { PlayOriginal } from "./play-original.tsx";
import { keepsOpenAttribute, PopoverButton, type OneControl } from "./popover.tsx";
import { volumeAfter } from "./player-commands.ts";
import { muteClass, SoundControls, type PlayerSound } from "./player-controls.tsx";
import { AudioIcon, MoreIcon, MutedIcon, SoundIcon, SubtitlesIcon } from "./player-icons.tsx";
import type { OverlayControl } from "./player-overlay-state.ts";
import { PlayerTracksContext, type PlayerTracks } from "./player-tracks.tsx";
import { ActionChooser, currentLabelOf, type ActionChoice } from "./route-chooser.tsx";
import { BoostBox } from "./sound.tsx";
import { SoundOffsetControl } from "./sound-delay.tsx";
import { FindSubtitles } from "./subtitle-search.tsx";
import { OffsetControl, useSubtitleChoices, type SubtitleChoice } from "./subtitles.tsx";

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
type OpenMenu = "volume" | "subtitles" | "audio" | "more" | "find" | "testers";

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
  const endFinding = useDialogEndedAtFullScreen(setOpen);
  const hasTesters = use(ForTestersContext) !== undefined;
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
          onTesters={
            hasTesters
              ? () => {
                  setOpen("testers");
                }
              : undefined
          }
        />
      )}
      {open === "find" && ways !== undefined ? (
        <FindSubtitlesDialog ways={ways} onClose={endFinding} />
      ) : null}
      {open === "testers" ? <ForTestersDialog onClose={endFinding} /> : null}
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
  /** Opens ⁝'s For testers, where the player gives the panel (ForTestersContext). */
  readonly onTesters: (() => void) | undefined;
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
          onTesters={props.onTesters}
          onOpenChange={toldOf("more")}
        />
      ) : null}
    </>
  );
}

/**
 * 🔈 (D407 §10, Q11): a pop-up with the volume upright and mute, both keeping it open
 * (keepsOpenAttribute), as + and − move the volume from anywhere (player-commands.ts). A pop-up of
 * its own, not a list: as wide as its slider (the owner, 2026-11-02: "The volume control popup is
 * far too wide"; D408 §2). It grows out of its button: its mute, a speaker as 🔈 is, laid over 🔈,
 * the slider above it, where there is room above (the owner, 2026-11-03: "the speaker icon inside
 * the slider ends up physically at a similar location to the button it spawned from"; Popover's
 * partOverOpener). Focus opens on that mute, where 🔈 was, as the browser's modal gives it, and stays
 * there: the pop-up is one control (OneControl), Up and Down stepping the volume as + and − do, OK
 * closing it (the owner, 2026-11-03: "it should behave as a single component"); the slider takes a
 * pointer alone, and the mute a press.
 */
function VolumeMenu(props: {
  readonly sound: PlayerSound;
  readonly onOpenChange: (isOpen: boolean) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { sound } = props;
  const isSilent = sound.isMuted || sound.volume === 0;
  return (
    <PopoverButton
      label={t("playerControls.volume")}
      opener={isSilent ? <MutedIcon /> : <SoundIcon />}
      openerClassName="icon-button icon-button--over-video"
      openerNaming={{ "aria-label": t("playerControls.volume") }}
      isFromItsTop
      partOverOpener={`.${muteClass}`}
      oneControl={volumeStepping(sound)}
      onOpenChange={props.onOpenChange}
    >
      <div {...{ [keepsOpenAttribute]: "" }}>
        <SoundControls sound={sound} isOverVideo={false} isUpright isRangeFocusable={false} />
      </div>
    </PopoverButton>
  );
}

/**
 * Up and Down in 🔈's pop-up: a step of the volume each (volumeAfter), Up turning a muted film's
 * sound on, as + and − do (player-commands.ts).
 */
function volumeStepping(sound: PlayerSound): OneControl {
  return {
    up: () => {
      sound.setVolume(volumeAfter(sound.volume, 1));
      if (sound.isMuted) {
        sound.setMuted(false);
      }
    },
    down: () => {
      sound.setVolume(volumeAfter(sound.volume, -1));
    },
  };
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

/**
 * Ends the dialog open from a menu (Find subtitles, For testers), as a change of full screen does;
 * returns the way to end it.
 */
function useDialogEndedAtFullScreen(
  setOpen: (open: (now: OpenMenu | undefined) => OpenMenu | undefined) => void,
): () => void {
  const endFinding = useCallback(() => {
    setOpen((now) => (now === "find" || now === "testers" ? undefined : now));
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
  const trackChoices = useSubtitleChoices(props.choice);
  const { onFind } = props;
  const find: ActionChoice[] =
    onFind === undefined
      ? []
      : [{ key: "find", label: t("play.findSubtitles.open"), isCurrent: false, onChoose: onFind }];
  const options = [...trackChoices, ...find];
  return (
    <ActionChooser
      fieldLabel={t("play.subtitles.label")}
      current={currentLabelOf(trackChoices)}
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
  const { t } = useTranslation();
  const options = useAudioChoices(props.choice);
  return (
    <ActionChooser
      fieldLabel={t("play.audio.label")}
      current={currentLabelOf(options)}
      openerIcon={<AudioIcon />}
      isOverVideo
      onOpenChange={props.onOpenChange}
      options={options}
    />
  );
}

/**
 * ⁝ (D407 §8, Q8): Start over, the subtitles' offset, the device's sound timing, levelling, the
 * dialogue boost, the original (its quality entry, D418 §4) and the bare player where they are
 * offered, and For testers where the player gives it (item 9 of ui-audit-2026-11-04.md). Its
 * steppers' presses keep it open (ListEntry's isKeptOpen); its other buttons close it, as a choice
 * does.
 */
function MoreMenu(props: {
  readonly tracks: PlayerTracks;
  readonly ways: PlayerMenusWays | undefined;
  readonly videoRef: RefObject<HTMLVideoElement | null>;
  readonly onTesters: (() => void) | undefined;
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
      isFromItsTop
      onOpenChange={props.onOpenChange}
    >
      <ListOption
        onChoose={() => {
          startOver(videoRef);
        }}
      >
        {t("play.startOver")}
      </ListOption>
      {subtitles.tracks.length === 0 ? null : (
        <KeptOpen>
          <OffsetControl offsetMs={subtitles.offsetMs} onMove={subtitles.shiftOffset} isInAList />
        </KeptOpen>
      )}
      {ways === undefined ? null : (
        <KeptOpen>
          <SoundOffsetControl videoRef={videoRef} initialMs={ways.soundOffsetMs} isInAList />
        </KeptOpen>
      )}
      {ways === undefined ? null : (
        <ListEntry>
          <LevellingControl
            level={tracks.level}
            measuring={tracks.measuring}
            state={tracks.levelling}
          />
        </ListEntry>
      )}
      {boost === undefined ? null : (
        <ListEntry>
          <BoostBox state={boost} />
        </ListEntry>
      )}
      <MoreOfThePage ways={ways} />
      {props.onTesters === undefined ? null : (
        <ListOption onChoose={props.onTesters}>{t("play.forTesters.open")}</ListOption>
      )}
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
        <ListEntry>
          <PlayOriginal onChosen={onOriginalChosen} />
        </ListEntry>
      )}
      {onBare === undefined ? null : (
        <ListOption onChoose={onBare}>{t("play.bare.open")}</ListOption>
      )}
    </>
  );
}

/**
 * A place of ⁝ whose buttons step a value, which keep the list open for the next press: Earlier and
 * Later beside what they move, in one row that wraps (the stepper's isInAList), not each a whole
 * place as a menu's choice is (D407 §8), set in as the choices' text is.
 */
function KeptOpen(props: { readonly children: ReactNode }): JSX.Element {
  return (
    <ListEntry isKeptOpen isInsetAsChoices>
      {props.children}
    </ListEntry>
  );
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

/**
 * ⁝'s For testers: the player's notes and tools for testing (for-testers.tsx), in a dialog of its
 * own, as Find subtitles is, so ⁝ stays short and they read at full width.
 */
function ForTestersDialog(props: { readonly onClose: () => void }): JSX.Element {
  const { t } = useTranslation();
  return (
    <ModalDialog label={t("play.forTesters.heading")} onClose={props.onClose}>
      <ForTestersShown />
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
        startOver(props.videoRef);
      }}
    >
      {t("play.startOver")}
    </button>
  );
}

/** Plays the film from its start. */
function startOver(videoRef: RefObject<HTMLVideoElement | null>): void {
  const video = videoRef.current;
  if (video !== null) {
    seekTo(video, 0);
  }
}
