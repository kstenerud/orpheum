// SPDX-License-Identifier: MPL-2.0

import "./ui-gallery.css";

import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import { MenuIcon } from "./app-shell.tsx";
import { ConfirmButton } from "./confirm-button.tsx";
import { IconButton } from "./icon-button.tsx";
import { PlayerControls, type PlayerSource } from "./player-controls.tsx";
import {
  CloseIcon,
  FullScreenIcon,
  LeaveFullScreenIcon,
  MutedIcon,
  PauseIcon,
  PlayIcon,
  SeekBackIcon,
  SeekForwardIcon,
  SoundIcon,
} from "./player-icons.tsx";
import { Clapperboard } from "./poster.tsx";
import { ChevronDownIcon, RouteChooser } from "./route-chooser.tsx";
import { AboutIcon, ResetIcon } from "./server-settings.tsx";
import { StatusNotes, StatusTray, type CueSubject } from "./status-cues.tsx";
import { CastIcon, TogetherIcon } from "./status-tray.tsx";
import { StillReadingIcon } from "./still-reading.tsx";
import { Timeline } from "./timeline.tsx";
import { MicrophoneIcon } from "./together-voice.tsx";
import { UncertainMatchIcon } from "./uncertain-match.tsx";
import { WarningIcon } from "./warning-sign.tsx";
import { WatchedIcon } from "./watched-mark.tsx";

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
  { key: "gallery.cannotPlay", subject: { unavailable: { reason: "node_away", nodeName: "nas" } } },
  { key: "gallery.guessed", subject: { isGuessed: true } },
  {
    key: "gallery.stillReading",
    subject: { unavailable: { reason: "still_reading", nodeName: "nas" } },
  },
  {
    key: "gallery.cannotPlayAndGuessed",
    subject: {
      unavailable: { reason: "root_missing", nodeName: "nas", path: "/media" },
      isGuessed: true,
    },
  },
  {
    key: "gallery.stillReadingAndGuessed",
    subject: { unavailable: { reason: "still_reading", nodeName: "nas" }, isGuessed: true },
  },
  { key: "gallery.watched", subject: { isWatched: true } },
  { key: "gallery.guessedAndWatched", subject: { isGuessed: true, isWatched: true } },
];

/**
 * The client's own parts, drawn with made-up data, to see how they look before a library has them
 * (the owner, 2026-10-17: "a demo page … that shows the UI elements"): the banner's status tray
 * with each sign it can show, each status cue over posters pale, mid, dark and busy, the notes a
 * page says them with, the icons alone, the buttons and the player's widgets (D369 §1). It reads nothing from the server; the menu
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
  return isOn
    ? "status-tray__button status-tray__button--on"
    : "status-tray__button status-tray__button--off";
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
              <span className="status-tray">
                {tray.needsAttention ? (
                  <span className="status-tray__button status-tray__button--warning">
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

/** The icons alone, every one the app draws. */
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
      </p>
    </section>
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
