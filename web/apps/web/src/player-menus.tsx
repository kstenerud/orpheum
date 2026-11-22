// SPDX-License-Identifier: MPL-2.0

import "./play-words.ts";

import { seekTo } from "@orpheum/player";
import type { PlaybackReport } from "@orpheum/sdk";
import {
  createContext,
  use,
  useCallback,
  useEffect,
  useState,
  type Context,
  type JSX,
  type ReactNode,
  type RefObject,
} from "react";
import { useTranslation } from "react-i18next";

import { DropDown } from "./drop-down.tsx";
import { ForTestersContext, ForTestersShown } from "./for-testers.tsx";
import { QualityChoices, useIsWithinCap } from "./data-cap-choices.tsx";
import { DebugTool, useShowsDebugTools } from "./debug-tools.tsx";
import { HowItPlays } from "./how-it-plays.tsx";
import { ListEntry, ListNote, ListOption } from "./list-box.tsx";
import { DialogClose, ModalDialog } from "./modal-dialog.tsx";
import { OriginalMovedTo, OriginalWaits } from "./play-original.tsx";
import { keepsOpenAttribute, PopoverButton } from "./popover.tsx";
import { muteClass, type PlayerSound } from "./player-controls.tsx";
import { AudioIcon, MutedIcon, SettingsIcon, SoundIcon, SubtitlesIcon } from "./player-icons.tsx";
import type { OverlayControl } from "./player-overlay-state.ts";
import { PlayerTracksContext, type PlayerTracks } from "./player-tracks.tsx";
import {
  playbackNow,
  ProblemReportDialog,
  type ReportedPlayback,
  type SuggestedSoundOffset,
} from "./problem-report.tsx";
import { ActionChooser, currentLabelOf, type ActionChoice } from "./route-chooser.tsx";
import type { FileSoundOffset } from "./sound-delay.tsx";
import { SoundMenu } from "./sound-menu.tsx";
import { FindSubtitles } from "./subtitle-search.tsx";
import { OffsetControl, useSubtitleChoices, type SubtitleChoice } from "./subtitles.tsx";
import { SessionTalk, useSessionVoice } from "./together-voice.tsx";
import { useVolumeRow, VolumeRow } from "./volume-row.tsx";

/**
 * What the page playing gives the overlay's menus beside the film's tracks (PlayerTracksContext):
 * the file, the device's sound timing it started with, how the film plays here (⁝'s How it plays,
 * D444), and what follows a subtitle fetched, the original chosen and the bare player asked for
 * (the last two where they are offered).
 */
export interface PlayerMenusWays {
  readonly fileId: number;
  readonly soundOffsetMs: number;
  /** The file's own sound offset as the playback started (D537 §4); undefined where not given. */
  readonly fileSoundOffset?: FileSoundOffset | undefined;
  /** The film's own file's rate, kbit/s, which a data cap offers its original under (D491); 0 or absent where not known. */
  readonly fileBitRateKbps?: number | undefined;
  readonly playback?: PlaybackReport | undefined;
  readonly onSubtitleFetched: () => void;
  readonly onOriginalChosen?: (() => void) | undefined;
  /**
   * The height of the rung the film plays while the original chosen waits on the ladder for the link
   * to carry it (the decision's isOriginalWaitingForTheLink): ⁝ then says so in place of offering it
   * (D501 §3). Undefined while it does not wait.
   */
  readonly originalWaitsAtHeight?: number | undefined;
  /** Whether the film moved up to the original chosen once the link carried it: ⁝ then says so (D501 §3). */
  readonly isOnTheOriginal?: boolean | undefined;
  readonly onBare?: (() => void) | undefined;
}

/** The page's ways for the overlay's menus; undefined where the page gives none, a cast's receiver. */
export const PlayerMenusContext: Context<PlayerMenusWays | undefined> = createContext<
  PlayerMenusWays | undefined
>(undefined);

/**
 * The menu open now, if one is: 🔈, Subtitles, Sound track, ⁝, or the dialog one opened (Find subtitles,
 * For testers, Report a problem).
 */
type OpenMenu = "volume" | "subtitles" | "audio" | "more" | "find" | "testers" | "report";

/**
 * The full-screen overlay's menus, in row 2's trailing group before full screen (D407 §1, §8): 🔈,
 * the row of volume sliders, where the overlay shows it (D407 §10, D440 §2); Talk beside it in a
 * Watch Together session, on a device with a microphone (SessionTalk, D440 §3); Subtitles, Off, the film's
 * subtitle tracks and Find subtitles; Sound track, the audio tracks, where there are more than one; ⁝
 * (Settings), the rest of what lies under the film outside full screen. Subtitles and Sound track are two
 * menus, each a button of its own, not one panel (the owner, 2026-11-04), and each opens on its
 * current choice with a choice either side in view where there are (DropDown, D408 §3). Each reads and writes the film's choices in
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
  const opens = (menu: OpenMenu) => (): void => {
    setOpen(menu);
  };
  const { reportProblem, tellTheAdmin } = useReportOpeners(ways, props.videoRef, setOpen);
  return (
    <>
      {props.shown.includes("sound") && sound !== undefined ? (
        <VolumeMenu sound={sound} onOpenChange={toldOf("volume")} />
      ) : null}
      <SessionTalk isOverVideo />
      {tracks === undefined ? null : (
        <TrackMenus
          tracks={tracks}
          ways={ways}
          videoRef={props.videoRef}
          shown={props.shown}
          toldOf={toldOf}
          onFind={ways === undefined ? undefined : opens("find")}
          onReport={reportProblem.open}
          onTellTheAdmin={tellTheAdmin}
          onTesters={hasTesters ? opens("testers") : undefined}
        />
      )}
      <MenuDialog
        open={open}
        ways={ways}
        reported={reportProblem.reported}
        suggested={reportProblem.suggested}
        onClose={endFinding}
      />
    </>
  );
}

/**
 * Report a problem's openers: ⁝'s (open), and the Sound menu's Tell the admin (tellTheAdmin), which
 * suggests the file's sound offset (D537 §5); each takes the player's state as it is chosen, before
 * the person writes (D524 §2). None where the page plays no film of its own.
 */
function useReportOpeners(
  ways: PlayerMenusWays | undefined,
  videoRef: RefObject<HTMLVideoElement | null>,
  setOpen: (menu: OpenMenu) => void,
): {
  readonly reportProblem: {
    readonly open: (() => void) | undefined;
    readonly reported: ReportedPlayback | undefined;
    readonly suggested: SuggestedSoundOffset | undefined;
  };
  readonly tellTheAdmin: ((suggested: SuggestedSoundOffset) => void) | undefined;
} {
  const [reported, setReported] = useState<ReportedPlayback | undefined>(undefined);
  const [suggested, setSuggested] = useState<SuggestedSoundOffset | undefined>(undefined);
  if (ways === undefined) {
    return { reportProblem: { open: undefined, reported, suggested }, tellTheAdmin: undefined };
  }
  const opened = (soundOffset: SuggestedSoundOffset | undefined): void => {
    setReported(playbackNow(ways.fileId, videoRef.current, ways.playback));
    setSuggested(soundOffset);
    setOpen("report");
  };
  return {
    reportProblem: {
      open: () => {
        opened(undefined);
      },
      reported,
      suggested,
    },
    tellTheAdmin: opened,
  };
}

/** The dialog a menu opened, if one did: Find subtitles, For testers or Report a problem. */
function MenuDialog(props: {
  readonly open: OpenMenu | undefined;
  readonly ways: PlayerMenusWays | undefined;
  /** The player's state as Report a problem was chosen. */
  readonly reported: ReportedPlayback | undefined;
  /** The file's sound offset the report suggests, from the Sound menu's Tell the admin. */
  readonly suggested: SuggestedSoundOffset | undefined;
  readonly onClose: () => void;
}): JSX.Element | null {
  const { open, ways, onClose } = props;
  if (open === "find" && ways !== undefined) {
    return <FindSubtitlesDialog ways={ways} onClose={onClose} />;
  }
  if (open === "testers") {
    return <ForTestersDialog onClose={onClose} />;
  }
  return open === "report" ? (
    <ProblemReportDialog
      playback={props.reported}
      soundOffset={props.suggested}
      onClose={onClose}
    />
  ) : null;
}

/** Subtitles, Sound track and ⁝ among those shown, over the film's tracks (PlayerMenus). */
function TrackMenus(props: {
  readonly tracks: PlayerTracks;
  readonly ways: PlayerMenusWays | undefined;
  readonly videoRef: RefObject<HTMLVideoElement | null>;
  readonly shown: readonly OverlayControl[];
  readonly toldOf: (menu: OpenMenu) => (isNowOpen: boolean) => void;
  readonly onFind: (() => void) | undefined;
  /** Opens ⁝'s Report a problem, where the page plays a film of its own (D524 §2). */
  readonly onReport: (() => void) | undefined;
  /** Opens Report a problem suggesting a file's sound offset, from the Sound menu (D537 §5). */
  readonly onTellTheAdmin: ((suggested: SuggestedSoundOffset) => void) | undefined;
  /** Opens ⁝'s For testers, where the player gives the panel (ForTestersContext). */
  readonly onTesters: (() => void) | undefined;
}): JSX.Element {
  const { tracks, shown, toldOf, ways, onTellTheAdmin } = props;
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
        <SoundMenu
          tracks={tracks}
          timing={
            ways === undefined || onTellTheAdmin === undefined
              ? undefined
              : {
                  deviceInitialMs: ways.soundOffsetMs,
                  file:
                    ways.fileSoundOffset === undefined
                      ? undefined
                      : { fileId: ways.fileId, initial: ways.fileSoundOffset },
                  onTellTheAdmin,
                }
          }
          videoRef={props.videoRef}
          onOpenChange={toldOf("audio")}
        />
      ) : null}
      {shown.includes("more") ? (
        <MoreMenu
          tracks={tracks}
          ways={props.ways}
          videoRef={props.videoRef}
          onReport={props.onReport}
          onTesters={props.onTesters}
          onOpenChange={toldOf("more")}
        />
      ) : null}
    </>
  );
}

/**
 * 🔈 (D407 §10, Q11, D440 §2): a pop-up with the row of volume sliders (VolumeRow), the film's first,
 * upright with its mute, then, in a Watch Together session, each other person's, all keeping it open
 * (keepsOpenAttribute), as + and − move the film's volume from anywhere (player-commands.ts). A
 * pop-up of its own, not a list: as wide as its sliders (the owner, 2026-11-02: "The volume control
 * popup is far too wide"; D408 §2). It grows out of its button: the film's mute, a speaker as 🔈 is,
 * laid over 🔈, the slider above it, where there is room above (the owner, 2026-11-03: "the speaker
 * icon inside the slider ends up physically at a similar location to the button it spawned from";
 * Popover's partOverOpener). Focus opens on that mute, where 🔈 was, as the browser's modal gives it:
 * the row is one control (OneControl), Up and Down stepping the focused slider, the film's as + and −
 * do, Left and Right moving focus along the row and closing it past either end, OK closing it (the
 * owner, 2026-11-03: "it should behave as a single component"); the sliders take a pointer or a
 * finger, and the mute a press. Each opening starts on the film's.
 */
export function VolumeMenu(props: {
  readonly sound: PlayerSound;
  readonly onOpenChange: (isOpen: boolean) => void;
  /** What the film's slider is said to set, where not the film's volume here (VolumeRow). */
  readonly filmTitle?: string | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  const { sound, onOpenChange } = props;
  const voice = useSessionVoice();
  const row = useVolumeRow(sound, voice);
  const { setFocusedAt } = row;
  const isSilent = sound.isMuted || sound.volume === 0;
  return (
    <PopoverButton
      label={t("playerControls.volume")}
      opener={isSilent ? <MutedIcon /> : <SoundIcon />}
      openerClassName="icon-button icon-button--over-video"
      openerNaming={{ "aria-label": t("playerControls.volume") }}
      isFromItsTop
      partOverOpener={`.${muteClass}`}
      oneControl={row.oneControl}
      onOpenChange={(isOpen) => {
        if (isOpen) {
          setFocusedAt(0);
        }
        onOpenChange(isOpen);
      }}
    >
      <div {...{ [keepsOpenAttribute]: "" }}>
        <VolumeRow sound={sound} voice={voice} row={row} filmTitle={props.filmTitle} />
      </div>
    </PopoverButton>
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

/**
 * Subtitles (💬): Off, the film's subtitle tracks, then Find subtitles where the page offers it (D407
 * §8), named by the choice showing: "Subtitles: Off".
 */
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
  return (
    <SubtitlesMenuOf
      options={[...trackChoices, ...find]}
      current={currentLabelOf(trackChoices)}
      onOpenChange={props.onOpenChange}
    />
  );
}

/**
 * Sound track's menu over options, the receiver's audio tracks on the cast's remote (D444), named by
 * the one playing: "Sound track: English" (the owner, 2026-11-04: "Use sound track"). The player's
 * own tracks are the Sound menu's first entry (sound-menu.tsx, D538 §8).
 */
export function SoundTrackMenuOf(props: {
  readonly options: readonly ActionChoice[];
  readonly onOpenChange: (isOpen: boolean) => void;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <ActionChooser
      fieldLabel={t("play.audio.menu")}
      current={currentLabelOf(props.options)}
      openerIcon={<AudioIcon />}
      isOverVideo
      onOpenChange={props.onOpenChange}
      options={props.options}
    />
  );
}

/**
 * Subtitles' menu over options, Off and the film's subtitle tracks: the player's (SubtitlesMenu, with
 * Find subtitles after them), or the receiver's on the cast's remote (D444).
 */
export function SubtitlesMenuOf(props: {
  readonly options: readonly ActionChoice[];
  /** What the menu is named by, the choice showing: "Subtitles: Off". */
  readonly current: string;
  readonly onOpenChange: (isOpen: boolean) => void;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <ActionChooser
      fieldLabel={t("play.subtitles.label")}
      current={props.current}
      openerIcon={<SubtitlesIcon />}
      isOverVideo
      onOpenChange={props.onOpenChange}
      options={props.options}
    />
  );
}

/**
 * ⁝ (D407 §8, Q8), in the order of the revamp's menu spec (D538): Start over, then
 * Quality, a submenu of Automatic and the device's data use where the film plays on its ladder,
 * the original file among them for one playback where debug tools are on (D538 §10); then the
 * subtitles' timing, the bare player where it is offered, Report a problem, For testers where the
 * player gives it (item 9 of ui-audit-2026-11-04.md), and last How it plays, the line that informs
 * (D444). It keeps no sound setting: those are the Sound menu's (D538 §8). Its stepper's presses
 * keep it open (ListEntry's isKeptOpen); its other buttons close it, as a choice does.
 */
function MoreMenu(props: {
  readonly tracks: PlayerTracks;
  readonly ways: PlayerMenusWays | undefined;
  readonly videoRef: RefObject<HTMLVideoElement | null>;
  readonly onReport: (() => void) | undefined;
  readonly onTesters: (() => void) | undefined;
  readonly onOpenChange: (isOpen: boolean) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { tracks, ways, videoRef } = props;
  const { subtitles } = tracks;
  const onBare = ways?.onBare;
  return (
    <MoreMenuOf onOpenChange={props.onOpenChange}>
      <ListOption
        onChoose={() => {
          startOver(videoRef);
        }}
      >
        {t("play.startOver")}
      </ListOption>
      {ways === undefined ? null : <QualityOfThePage ways={ways} videoRef={videoRef} />}
      {subtitles.tracks.length === 0 ? null : (
        <KeptOpen>
          <OffsetControl offsetMs={subtitles.offsetMs} onMove={subtitles.shiftOffset} />
        </KeptOpen>
      )}
      {onBare === undefined ? null : (
        <ListOption onChoose={onBare}>{t("play.bare.open")}</ListOption>
      )}
      {props.onReport === undefined ? null : (
        <ListOption onChoose={props.onReport}>{t("problemReport.open")}</ListOption>
      )}
      {props.onTesters === undefined ? null : (
        <ListOption onChoose={props.onTesters}>{t("play.forTesters.open")}</ListOption>
      )}
      <HowItPlays playback={ways?.playback} />
    </MoreMenuOf>
  );
}

/**
 * ⁝'s pop-up (Settings) over its places (children): the player's (MoreMenu), or the cast's remote's
 * over the receiver's (D444).
 */
export function MoreMenuOf(props: {
  readonly onOpenChange: (isOpen: boolean) => void;
  readonly children: ReactNode;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <DropDown
      label={t("playerControls.settings")}
      opener={<SettingsIcon />}
      openerClassName="icon-button icon-button--over-video"
      openerNaming={{ "aria-label": t("playerControls.settings") }}
      isFromItsTop
      onOpenChange={props.onOpenChange}
    >
      {props.children}
    </DropDown>
  );
}

/**
 * ⁝'s Quality where the page plays a film of its own (QualityChoices), the original's entry last in
 * its list (OriginalOfTheQuality), which a device under a data cap is offered only where the cap is
 * at or above the file's rate (D491); and, once the film moved up to the original chosen, off its
 * ladder and so with no Quality, word that it plays the original (OriginalMovedTo, D501 §3), a debug
 * tool as the entry is.
 */
function QualityOfThePage(props: {
  readonly ways: PlayerMenusWays;
  readonly videoRef: RefObject<HTMLVideoElement | null>;
}): JSX.Element {
  const { ways } = props;
  const isOffered = useIsWithinCap(ways.fileBitRateKbps ?? 0);
  const isOriginalShown = useShowsDebugTools() && ways.onOriginalChosen !== undefined && isOffered;
  return (
    <>
      <QualityChoices videoRef={props.videoRef} isChildShown={isOriginalShown}>
        <OriginalOfTheQuality
          onChosen={isOffered ? ways.onOriginalChosen : undefined}
          waitsAtHeight={ways.originalWaitsAtHeight}
        />
      </QualityChoices>
      {ways.isOnTheOriginal === true ? (
        <DebugTool>
          <ListNote>
            <OriginalMovedTo />
          </ListNote>
        </DebugTool>
      ) : null}
    </>
  );
}

/**
 * The Quality list's last entry, the original file, a debug tool chosen for this playback alone
 * (D538 §10; the owner, 2026-10-29: "It's useful as a debug tool though"): Play the original file,
 * which loads the file's own source in place of the ladder, or, while the original chosen waits on
 * the ladder for the link, word that it plays its rung until then (OriginalWaits, D501 §3). Nothing
 * where it is not offered, nor while the person's Show debug tools is off.
 */
function OriginalOfTheQuality(props: {
  readonly onChosen: (() => void) | undefined;
  readonly waitsAtHeight: number | undefined;
}): JSX.Element | null {
  const { t } = useTranslation();
  if (props.onChosen === undefined) {
    return null;
  }
  return (
    <DebugTool>
      {props.waitsAtHeight === undefined ? (
        <ListOption onChoose={props.onChosen}>{t("play.playOriginal")}</ListOption>
      ) : (
        <ListNote>
          <OriginalWaits height={props.waitsAtHeight} />
        </ListNote>
      )}
    </DebugTool>
  );
}

/**
 * A place of a menu whose buttons step a value, a stepper's, which keep the list open for the next
 * press: − and + beside what they move, not each a whole place as a menu's choice is (D407 §8), set
 * in as the choices' text is.
 */
export function KeptOpen(props: { readonly children: ReactNode }): JSX.Element {
  return (
    <ListEntry isKeptOpen isInsetAsChoices>
      {props.children}
    </ListEntry>
  );
}

/** Find subtitles, opened from Subtitles in a dialog of its own, closing once one is fetched (D407 §8). */
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
    <ModalDialog
      label={t("play.forTesters.heading")}
      onClose={props.onClose}
      footer={<DialogClose onClose={props.onClose} />}
    >
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
