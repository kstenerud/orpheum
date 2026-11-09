// SPDX-License-Identifier: MPL-2.0

import "./status-cues.css";

import { createElement, type JSX, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { unavailableText } from "./problem.ts";
import { StillReadingIcon, StillReadingNote } from "./still-reading.tsx";
import { UncertainMatchIcon, UncertainMatchNote } from "./uncertain-match.tsx";
import { WarningIcon } from "./warning-sign.tsx";
import { UnplayableSign } from "./unplayable-sign.tsx";
import { WatchedIcon, WatchedNote } from "./watched-mark.tsx";

/** What a status cue reads of a film, series or episode. */
export interface CueSubject {
  /** Why none of its files can be read now, when none can (D241 §3). */
  readonly unavailable?: Parameters<typeof unavailableText>[1] | undefined;
  /** Whether its match is a guess no person confirmed (D301 §4). */
  readonly isGuessed?: boolean | undefined;
  /** Whether its person watched it: a video by playing or by hand, a series or season every episode (D370). */
  readonly isWatched?: boolean | undefined;
}

/**
 * One status cue: whether a subject has it, its icon over a poster, and its note on a page. Over a
 * poster it is only an icon, never a control and with no tooltip, which made the posters messy and
 * took a controller's focus; the title's page says what it means (the owner, 2026-10-25).
 */
interface StatusCue {
  readonly name: string;
  readonly isShown: (subject: CueSubject) => boolean;
  readonly icon: () => JSX.Element;
  readonly note: (props: { readonly subject: CueSubject }) => JSX.Element | null;
}

/** A cue's icon over a poster, named for a screen reader by what it means, and nothing more. */
function PosterCueIcon(props: {
  readonly label: string;
  readonly children: ReactNode;
}): JSX.Element {
  return (
    <span className="poster-cue">
      {props.children}
      <span className="poster-cue__name">{props.label}</span>
    </span>
  );
}

/** A cannot-play cue's icon over a poster: the warning triangle (D278). */
function CannotPlayIcon(): JSX.Element {
  const { t } = useTranslation();
  return (
    <PosterCueIcon label={t("warning.cannotPlay")}>
      <WarningIcon />
    </PosterCueIcon>
  );
}

/** An uncertain match's icon over a poster (D301 §4). */
function UncertainMatchCueIcon(): JSX.Element {
  const { t } = useTranslation();
  return (
    <PosterCueIcon label={t("warning.uncertainMatch")}>
      <UncertainMatchIcon isRinged={true} />
    </PosterCueIcon>
  );
}

/** A file still being read's icon over a poster (D304 §4). */
function StillReadingCueIcon(): JSX.Element {
  const { t } = useTranslation();
  return (
    <PosterCueIcon label={t("warning.stillReading")}>
      <StillReadingIcon isRinged={true} />
    </PosterCueIcon>
  );
}

/** A watched video's or series' icon over a poster: the white check (D370 §2). */
function WatchedCueIcon(): JSX.Element {
  const { t } = useTranslation();
  return (
    <PosterCueIcon label={t("watched.watched")}>
      <WatchedIcon />
    </PosterCueIcon>
  );
}

/** A subject's cannot-play note: the warning sign, and why, as the reader needs it (D241 §3). */
function CannotPlayNote(props: { readonly subject: CueSubject }): JSX.Element | null {
  const { t } = useTranslation();
  const { unavailable } = props.subject;
  return unavailable === undefined ? null : (
    <output className="status-note status-note--cannot-play">
      <UnplayableSign unavailable={unavailable} /> {unavailableText(t, unavailable)}
    </output>
  );
}

/**
 * Every status cue, in the order they are shown, a poster's tray from its top-right corner down
 * and a page's notes from the top: the warning of one that cannot play always first, the most
 * urgent (the owner, 2026-10-15: "The warning icon should always be first (top). We keep this order
 * as a static list in case we add more statuses later"). A new status is a new row, in its place.
 */
export const statusCues: readonly StatusCue[] = [
  {
    name: "cannotPlay",
    isShown: (subject) => subject.unavailable !== undefined && !isStillReading(subject),
    icon: CannotPlayIcon,
    note: CannotPlayNote,
  },
  {
    name: "uncertainMatch",
    isShown: (subject) => subject.isGuessed === true,
    icon: UncertainMatchCueIcon,
    note: UncertainMatchNote,
  },
  {
    // Not a fault: shown from its listing, it plays once read (D304 §4).
    name: "stillReading",
    isShown: isStillReading,
    icon: StillReadingCueIcon,
    note: StillReadingNote,
  },
  {
    // The least urgent, no problem and no activity: last (D370 §2). Never on an extra, which the
    // server never says is watched.
    name: "watched",
    isShown: (subject) => subject.isWatched === true,
    icon: WatchedCueIcon,
    note: WatchedNote,
  },
];

/** Whether a subject's files are only waiting to be read, which is no warning (D304 §4). */
function isStillReading(subject: CueSubject): boolean {
  return subject.unavailable?.reason === "stillReading";
}

/** A poster's tray of status cues, in statusCues' order; nothing when it has none (D278, D301 §4). */
export function StatusTray(props: { readonly subject: CueSubject }): JSX.Element | null {
  const shown = statusCues.filter((cue) => cue.isShown(props.subject));
  return shown.length === 0 ? null : (
    <span className="poster-card__cues">
      {shown.map((cue) => createElement(cue.icon, { key: cue.name }))}
    </span>
  );
}

/** A page's status notes, in statusCues' order, each its icon and what it means. */
export function StatusNotes(props: { readonly subject: CueSubject }): JSX.Element {
  return (
    <>
      {statusCues
        .filter((cue) => cue.isShown(props.subject))
        .map((cue) => createElement(cue.note, { key: cue.name, subject: props.subject }))}
    </>
  );
}
