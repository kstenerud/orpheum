// SPDX-License-Identifier: MPL-2.0

import "./ui-gallery.css";

import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import { ConfirmButton } from "./confirm-button.tsx";
import { StatusNotes, StatusTray, type CueSubject } from "./status-cues.tsx";
import { StillReadingIcon } from "./still-reading.tsx";
import { UncertainMatchIcon } from "./uncertain-match.tsx";
import { WarningIcon } from "./warning-sign.tsx";

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
];

/**
 * The client's own parts, drawn with made-up data, to see how they look before a library has them
 * (the owner, 2026-10-17: "a demo page … that shows the UI elements"): each status cue over posters
 * pale, mid, dark and busy, the notes a page says them with, the icons alone, and the buttons. It
 * reads nothing from the server, and no menu leads to it: its address is /ui-gallery.
 */
export function UiGallery(): JSX.Element {
  const { t } = useTranslation();
  return (
    <article className="ui-gallery">
      <h1>{t("gallery.heading")}</h1>
      <p>{t("gallery.what")}</p>
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
    </article>
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
          <li key={state.key} className="poster-grid__item">
            <span className="poster-grid__card">
              <span className={`poster ui-gallery__poster ${toneClass[tone]}`} aria-hidden="true" />
              <span className="poster-grid__name">{t(state.key)}</span>
            </span>
            <StatusTray subject={state.subject} />
          </li>
        ))}
      </ul>
    </section>
  );
}

/** The icons alone, and the buttons. */
function IconsAndButtons(): JSX.Element {
  const { t } = useTranslation();
  return (
    <>
      <section aria-labelledby="gallery-icons">
        <h2 id="gallery-icons">{t("gallery.icons")}</h2>
        <p className="ui-gallery__icons">
          <WarningIcon />
          <UncertainMatchIcon isRinged={true} />
          <UncertainMatchIcon isRinged={false} />
          <StillReadingIcon isRinged={true} />
          <StillReadingIcon isRinged={false} />
        </p>
      </section>
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
      </section>
    </>
  );
}
