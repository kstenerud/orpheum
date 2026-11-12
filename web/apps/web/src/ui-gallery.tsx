// SPDX-License-Identifier: MPL-2.0

import "./ui-gallery.css";

import "./gallery-words.ts";

import { startingTuning, type TogetherVoice, type VoicePartner } from "@orpheum/sdk";
import { useRef, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { ConfirmButton } from "./confirm-button.tsx";
import { IconButton } from "./icon-button.tsx";
import {
  AboutIcon,
  CastIcon,
  ChevronDownIcon,
  Clapperboard,
  FilmsIcon,
  FilmStripIcon,
  FindDevicesIcon,
  GearIcon,
  GoogleCastIcon,
  LibrariesIcon,
  MenuIcon,
  MicrophoneIcon,
  MoreIcon,
  PosterClock,
  ResetIcon,
  RowArrowIcon,
  SeriesIcon,
  SortArrowIcon,
  SortIcon,
  StillReadingIcon,
  TogetherIcon,
  UncertainMatchIcon,
  WarningIcon,
  WatchedIcon,
} from "./icons.tsx";
import { Cluster, Stack } from "./layout.tsx";
import { ListBox, ListNote, ListOption } from "./list-box.tsx";
import { PlayUpwardContext } from "./play-up.tsx";
import { PlayerControls, type PlayerSource } from "./player-controls.tsx";
import { OverlayRows } from "./player-overlay.tsx";
import { PlayerTracksContext, type PlayerTracks } from "./player-tracks.tsx";
import { PopoverButton } from "./popover.tsx";
import {
  AudioIcon,
  BackIcon,
  CloseIcon,
  FullScreenIcon,
  LeaveFullScreenIcon,
  SettingsIcon,
  MutedIcon,
  NextIcon,
  PauseIcon,
  PlayIcon,
  PreviousIcon,
  SeekBackIcon,
  SeekForwardIcon,
  SoundIcon,
  SubtitlesIcon,
} from "./player-icons.tsx";
import { RouteChooser } from "./route-chooser.tsx";
import { StatusNotes, StatusTray, type CueSubject } from "./status-cues.tsx";
import { Timeline } from "./timeline.tsx";
import { SessionVoiceContext, type VoiceInUse } from "./together-voice.tsx";
import type { Upward } from "./upward.ts";

/** The poster tones the cues are shown over: a pale poster, a mid one, a dark one and a busy one. */
const tones = ["light", "medium", "dark", "busy"] as const;

/** Each tone's poster class, written out whole, so a search for a stylesheet's class finds it. */
const toneClass: Readonly<Record<(typeof tones)[number], string>> = {
  light: "ui-gallery__poster--light",
  medium: "ui-gallery__poster--medium",
  dark: "ui-gallery__poster--dark",
  busy: "ui-gallery__poster--busy",
};

/** One way a title can be, by the cues it shows, each with its name's key. */
const states: readonly { readonly key: string; readonly subject: CueSubject }[] = [
  { key: "gallery.plain", subject: {} },
  { key: "gallery.cannotPlay", subject: { unavailable: { reason: "nodeAway", nodeName: "nas" } } },
  { key: "gallery.guessed", subject: { isGuessed: true } },
  {
    key: "gallery.stillReading",
    subject: { unavailable: { reason: "stillReading", nodeName: "nas" } },
  },
  {
    key: "gallery.cannotPlayAndGuessed",
    subject: {
      unavailable: { reason: "rootMissing", nodeName: "nas", path: "/media" },
      isGuessed: true,
    },
  },
  {
    key: "gallery.stillReadingAndGuessed",
    subject: { unavailable: { reason: "stillReading", nodeName: "nas" }, isGuessed: true },
  },
  { key: "gallery.watched", subject: { isWatched: true } },
  { key: "gallery.guessedAndWatched", subject: { isGuessed: true, isWatched: true } },
];

/**
 * The client's own parts, drawn with made-up data, to see how they look before a library has them
 * (the owner, 2026-10-17: "a demo page … that shows the UI elements"): the banner's status tray
 * with each sign it can show, each status cue over posters pale, mid, dark and busy, the notes a
 * page says them with, the icons alone, the buttons, the pop-ups and layouts the widgets are built
 * from (D408) and the player's widgets (D369 §1). It reads nothing from the server; the menu
 * leads to it while the admin has the pages for development on, and its address is /ui-gallery.
 */
export function UiGallery(): JSX.Element {
  const { t } = useTranslation();
  return (
    <article className="ui-gallery">
      <h1>{t("gallery.heading")}</h1>
      <p>{t("gallery.what")}</p>
      <BannerTrays />
      {tones.map((tone) => (
        <PostersOfTone key={tone} tone={tone} />
      ))}
      <section aria-labelledby="gallery-notes">
        <h2 id="gallery-notes">{t("gallery.notes")}</h2>
        {states.slice(1).map((state) => (
          <div key={state.key} className="ui-gallery__notes">
            <h3>{t(state.key)}</h3>
            <StatusNotes subject={state.subject} />
          </div>
        ))}
      </section>
      <IconsAndButtons />
      <PopUpsAndLayouts />
      <PlayerWidgets />
    </article>
  );
}

/**
 * Which signs a banner's tray shows, each with its name's key: Watch Together's and Cast's always,
 * each off or on (D364 §12).
 */
const trays: readonly {
  readonly key: string;
  readonly needsAttention: boolean;
  readonly isTogetherOn: boolean;
  readonly isCastOn: boolean;
}[] = [
  { key: "gallery.trayOff", needsAttention: false, isTogetherOn: false, isCastOn: false },
  { key: "gallery.trayNeedsAttention", needsAttention: true, isTogetherOn: false, isCastOn: false },
  { key: "gallery.trayTogether", needsAttention: false, isTogetherOn: true, isCastOn: false },
  { key: "gallery.trayCasting", needsAttention: false, isTogetherOn: false, isCastOn: true },
  { key: "gallery.trayBoth", needsAttention: true, isTogetherOn: true, isCastOn: true },
];

/** A tray sign's classes, off or on, as the tray draws them (status-tray.tsx). */
function traySignClass(isOn: boolean): string {
  return isOn ? "icon-button icon-button--running" : "icon-button";
}

/**
 * The banner's end as each sign of its status tray shows there (D280): beside the realm and the
 * menu, in the tray's own classes, the signs drawn without what they do.
 */
function BannerTrays(): JSX.Element {
  const { t } = useTranslation();
  return (
    <section aria-labelledby="gallery-trays">
      <h2 id="gallery-trays">{t("gallery.trays")}</h2>
      {trays.map((tray) => (
        <div key={tray.key} className="ui-gallery__tray">
          <h3>{t(tray.key)}</h3>
          <div className="ui-gallery__banner">
            <div className="app-shell__end">
              <span className="app-shell__realm">{t("gallery.realm")}</span>
              <span className="status-tray icon-tray">
                {tray.needsAttention ? (
                  <span className="icon-button status-tray__button status-tray__button--warning">
                    <WarningIcon />
                  </span>
                ) : null}
                <span className={traySignClass(tray.isTogetherOn)}>
                  <TogetherIcon />
                </span>
                <span className={traySignClass(tray.isCastOn)}>
                  <CastIcon />
                </span>
              </span>
              <span className="app-shell__menu">
                <span className="ui-gallery__menu-mark">
                  <MenuIcon />
                </span>
              </span>
            </div>
          </div>
        </div>
      ))}
    </section>
  );
}

/** Every state's cues over posters of one tone. */
function PostersOfTone(props: { readonly tone: (typeof tones)[number] }): JSX.Element {
  const { t } = useTranslation();
  const { tone } = props;
  return (
    <section aria-labelledby={`gallery-${tone}`}>
      <h2 id={`gallery-${tone}`}>{t(`gallery.tone.${tone}`)}</h2>
      <ul className="poster-grid">
        {states.map((state) => (
          <li key={state.key} className="poster-card">
            <span className="poster-card__link">
              <span className="poster-card__art">
                <span
                  className={`poster ui-gallery__poster ${toneClass[tone]}`}
                  aria-hidden="true"
                />
              </span>
              <span className="poster-card__name">{t(state.key)}</span>
            </span>
            <StatusTray subject={state.subject} />
          </li>
        ))}
      </ul>
    </section>
  );
}

/** The icons alone, every one the app draws, and the buttons. */
function IconsAndButtons(): JSX.Element {
  return (
    <>
      <Icons />
      <Buttons />
    </>
  );
}

/** The icons alone, every one the app draws: icons.tsx's whole set, and the player's. */
function Icons(): JSX.Element {
  const { t } = useTranslation();
  return (
    <section aria-labelledby="gallery-icons">
      <h2 id="gallery-icons">{t("gallery.icons")}</h2>
      <p className="ui-gallery__icons">
        <WarningIcon />
        <UncertainMatchIcon isRinged={true} />
        <UncertainMatchIcon isRinged={false} />
        <StillReadingIcon isRinged={true} />
        <StillReadingIcon isRinged={false} />
        <WatchedIcon />
      </p>
      <p className="ui-gallery__icons">
        <MenuIcon />
        <TogetherIcon />
        <CastIcon />
        <MicrophoneIcon isOn={true} />
        <MicrophoneIcon isOn={false} />
        <AboutIcon />
        <ResetIcon />
        <ChevronDownIcon />
        <span className="ui-gallery__blank-mark">
          <Clapperboard />
        </span>
      </p>
      <PlacesAndToolsIcons />
      <p className="ui-gallery__icons">
        <PlayIcon />
        <PauseIcon />
        <SeekBackIcon />
        <SeekForwardIcon />
        <SoundIcon />
        <MutedIcon />
        <FullScreenIcon />
        <LeaveFullScreenIcon />
        <CloseIcon />
        <PreviousIcon />
        <NextIcon />
        <SubtitlesIcon />
        <AudioIcon />
        <SettingsIcon />
        <BackIcon />
      </p>
    </section>
  );
}

/** The icons of the banner's places, a listing's tools, a row's arrows, Cast's menu and a poster. */
function PlacesAndToolsIcons(): JSX.Element {
  return (
    <p className="ui-gallery__icons">
      <FilmsIcon className="app-shell__place-glyph" />
      <SeriesIcon className="app-shell__place-glyph" />
      <LibrariesIcon className="app-shell__place-glyph" />
      <MoreIcon />
      <GearIcon />
      <SortIcon />
      <SortArrowIcon isAscending={true} />
      <SortArrowIcon isAscending={false} />
      <span className="icon-button">
        <RowArrowIcon direction="back" />
      </span>
      <span className="icon-button">
        <RowArrowIcon direction="on" />
      </span>
      <GoogleCastIcon />
      <FindDevicesIcon />
      <FilmStripIcon />
      <span className="ui-gallery__blank-mark">
        <PosterClock />
      </span>
    </p>
  );
}

/** The buttons: plain, disabled, one that asks first, and a chooser. */
function Buttons(): JSX.Element {
  const { t } = useTranslation();
  return (
    <section aria-labelledby="gallery-buttons">
      <h2 id="gallery-buttons">{t("gallery.buttons")}</h2>
      <p className="ui-gallery__buttons">
        <button type="button">{t("gallery.button")}</button>
        <button type="button" disabled>
          {t("gallery.buttonDisabled")}
        </button>
      </p>
      <ConfirmButton
        label={t("gallery.confirm")}
        consequence={t("gallery.confirmConsequence")}
        confirmLabel={t("gallery.confirmYes")}
        onConfirm={() => undefined}
      />
      <RouteChooser
        fieldLabel={t("gallery.chooser")}
        current={t("gallery.chooserCurrent")}
        options={[
          {
            key: "gallery",
            label: t("gallery.chooserCurrent"),
            to: { screen: "ui-gallery" },
            isCurrent: true,
          },
        ]}
        isAView
      />
    </section>
  );
}

/**
 * The parts the widgets are built from (D408): a pop-up at its button holding a list that opens on
 * its current choice, with a line that only informs and wraps (§7), and the layout primitives,
 * `Stack` and `Cluster`, each laying out three buttons.
 */
function PopUpsAndLayouts(): JSX.Element {
  const { t } = useTranslation();
  const choices = ["gallery.choiceOne", "gallery.choiceTwo", "gallery.choiceThree"];
  return (
    <section aria-labelledby="gallery-pop-ups">
      <h2 id="gallery-pop-ups">{t("gallery.popUps")}</h2>
      <PopoverButton
        label={t("gallery.popUp")}
        opener={t("gallery.popUp")}
        openerClassName="ui-gallery__opener"
        openerNaming={{}}
        isFromItsTop={false}
      >
        <ListBox isFromItsTop={false}>
          <ListNote>{t("gallery.popUpNote")}</ListNote>
          {choices.map((choice, index) => (
            <ListOption key={choice} isCurrent={index === 1} onChoose={() => undefined}>
              {t(choice)}
            </ListOption>
          ))}
        </ListBox>
      </PopoverButton>
      <h3>{t("gallery.stack")}</h3>
      <Stack space="2">
        {choices.map((choice) => (
          <button key={choice} type="button">
            {t(choice)}
          </button>
        ))}
      </Stack>
      <h3>{t("gallery.cluster")}</h3>
      <Cluster space="3">
        {choices.map((choice) => (
          <button key={choice} type="button">
            {t(choice)}
          </button>
        ))}
      </Cluster>
    </section>
  );
}

/**
 * A film standing at 24:26 of 1:42:05, playing, for the player's widgets to show: it plays nothing
 * and moves nowhere.
 */
const galleryFilm: PlayerSource = {
  atMs: 1_466_000,
  durationMs: 6_125_000,
  isPaused: false,
  isReady: true,
  playOrPause: () => undefined,
  seekTo: () => undefined,
  sound: {
    volume: 0.7,
    isMuted: false,
    canSetVolume: true,
    setVolume: () => undefined,
    setMuted: () => undefined,
  },
};

/**
 * The player's widgets (D369 §1), drawn by the components themselves: an icon button on the page
 * and over a film, a timeline, and the player's bar as a computer shows it over a film and as a
 * cast's remote shows it on its page.
 */
function PlayerWidgets(): JSX.Element {
  const { t } = useTranslation();
  return (
    <section aria-labelledby="gallery-player">
      <h2 id="gallery-player">{t("gallery.player")}</h2>
      <p className="ui-gallery__buttons">
        <IconButton
          label={t("gallery.iconButton")}
          icon={<CloseIcon />}
          onClick={() => undefined}
        />
        <span className="ui-gallery__film ui-gallery__film--strip">
          <IconButton
            label={t("gallery.iconButtonOverVideo")}
            icon={<PlayIcon />}
            isOverVideo
            onClick={() => undefined}
          />
        </span>
      </p>
      <h3>{t("gallery.timeline")}</h3>
      <Timeline
        label={t("gallery.timeline")}
        atMs={galleryFilm.atMs}
        durationMs={galleryFilm.durationMs}
        onSeek={() => undefined}
      />
      <h3>{t("gallery.overlayRows")}</h3>
      <p>{t("gallery.overlayPopUps")}</p>
      <GalleryOverlayRows />
      <h3>{t("gallery.barOverFilm")}</h3>
      <div className="ui-gallery__film">
        <div className="ui-gallery__bar">
          <PlayerControls
            source={galleryFilm}
            controls={["play", "fullScreen", "sound", "timeline"]}
            isOverVideo
          />
        </div>
      </div>
      <h3>{t("gallery.barRemote")}</h3>
      <PlayerControls source={galleryFilm} controls={["play", "steps", "timeline"]} />
    </section>
  );
}

/**
 * The film's tracks the gallery's overlay chooses among: two sound tracks and two subtitle tracks,
 * so Subtitles and Sound track each draw their pop-up; it chooses nothing.
 */
const galleryTracks: PlayerTracks = {
  subtitles: {
    tracks: [
      {
        streamId: 5,
        language: "en",
        isDefault: false,
        isForced: false,
        isHearingImpaired: false,
        assUrl: "",
        vttUrl: "",
      },
      {
        streamId: 7,
        language: "fr",
        isDefault: false,
        isForced: false,
        isHearingImpaired: false,
        assUrl: "",
        vttUrl: "",
      },
    ],
    chosenTrackKey: undefined,
    choose: () => undefined,
    offsetMs: 0,
    shiftOffset: () => undefined,
    failure: undefined,
  },
  audio: {
    tracks: [
      { streamId: 80, audioTrackIndex: 0, language: "en", kind: "plain" },
      { streamId: 81, audioTrackIndex: 1, language: "en", kind: "commentary" },
    ],
    chosenStreamId: 80,
    choose: () => undefined,
    problem: undefined,
  },
  levelling: { isOn: false, switchTo: () => undefined, problem: undefined },
  level: { levelling: { isMeasured: true, gainDb: -2 }, carriedGainDb: 0 },
  measuring: undefined,
  boost: undefined,
};

/** The page above the gallery's film, for the overlay's Up: the gallery itself. */
const galleryUpward: Upward = {
  crumbs: [{ label: "UI gallery", to: { screen: "ui-gallery" }, isIt: () => true }],
  here: "",
};

/**
 * The player's full-screen overlay's three rows (D407 §1), the real ones (OverlayRows), all shown,
 * over a film's ground, as a computer draws them: its Up, the end time, the scrub bar and the buttons
 * with 🔈, Subtitles, Sound track and ⁝, each opening its own pop-up. The overlay's showing, hiding
 * and presses are PlayerOverlay's, not drawn here.
 */
/** The others in the gallery's Watch Together session, two of whose names start alike (D440 §2). */
const galleryPartners: readonly VoicePartner[] = [
  { deviceId: 11, memberId: 2, name: "Gran", state: "connected" },
  { deviceId: 12, memberId: 3, name: "Gail", state: "connected" },
  { deviceId: 13, memberId: 4, name: "Dan", state: "connected" },
];

/**
 * A Watch Together session's voice for the gallery's overlay: its people, whose volumes its 🔈 row
 * sets here alone, heard by no one; voice off, so Talk shows where the device has a microphone.
 */
function useGalleryVoice(): VoiceInUse {
  const [partnerGains, setPartnerGains] = useState<Readonly<Record<number, number>>>({});
  const snapshot = {
    status: "off" as const,
    failure: undefined,
    browserReason: undefined,
    isHearing: true,
    isMuted: false,
    partnerGains,
    isConnected: true,
    partners: galleryPartners,
    tuning: startingTuning,
  };
  const nothing = (): void => undefined;
  const voice: TogetherVoice = {
    snapshot: () => snapshot,
    subscribe: () => nothing,
    hear: nothing,
    start: () => Promise.resolve(),
    stop: nothing,
    leave: nothing,
    setMuted: nothing,
    setPartnerGain: (memberId, gain) => {
      setPartnerGains((gains) => ({ ...gains, [memberId]: gain }));
    },
    readMicrophoneLevel: () => 0,
  };
  return { voice, snapshot, tap: { isWaiting: false, play: nothing } };
}

function GalleryOverlayRows(): JSX.Element {
  const voice = useGalleryVoice();
  const videoRef = useRef<HTMLVideoElement | null>(null);
  const playRef = useRef<HTMLButtonElement | null>(null);
  const scrubRef = useRef<HTMLInputElement | null>(null);
  const buttonsRef = useRef<HTMLDivElement | null>(null);
  const areaRef = useRef<HTMLDivElement | null>(null);
  return (
    <SessionVoiceContext value={voice}>
      <PlayUpwardContext value={galleryUpward}>
        <PlayerTracksContext value={galleryTracks}>
          <div className="ui-gallery__film">
            <div className="player-overlay" data-shown="full">
              <OverlayRows
                source={galleryFilm}
                isTv={false}
                refs={{ playRef, scrubRef, buttonsRef, videoRef }}
                convenience={{
                  film: { videoRef, readLinkKbps: () => undefined, failure: undefined },
                  offer: undefined,
                  hasOffer: false,
                  isOverlayFull: true,
                  onOffered: () => undefined,
                  areaRef,
                }}
                moves={undefined}
                onMenuOpenChange={() => undefined}
              />
            </div>
          </div>
        </PlayerTracksContext>
      </PlayUpwardContext>
    </SessionVoiceContext>
  );
}
