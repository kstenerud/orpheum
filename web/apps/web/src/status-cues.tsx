// SPDX-License-Identifier: MPL-2.0

import "./status-cues.css";

import { createElement, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { unavailableText } from "./problem.ts";
import { UncertainMatchNote, UncertainMatchSign } from "./uncertain-match.tsx";
import { UnplayableSign } from "./unplayable-sign.tsx";

/** What a status cue reads of a film, series or episode. */
export interface CueSubject {
  /** Why none of its files can be read now, when none can (D241 §3). */
  readonly unavailable?: Parameters<typeof unavailableText>[1] | undefined;
  /** Whether its match is a guess no person confirmed (D301 §4). */
  readonly isGuessed?: boolean | undefined;
}

/** One status cue: whether a subject has it, its sign over a poster, and its note on a page. */
interface StatusCue {
  readonly name: string;
  readonly isShown: (subject: CueSubject) => boolean;
  readonly sign: (props: { readonly subject: CueSubject }) => JSX.Element | null;
  readonly note: (props: { readonly subject: CueSubject }) => JSX.Element | null;
}

/** A subject's cannot-play sign over its poster (D278). */
function CannotPlaySign(props: { readonly subject: CueSubject }): JSX.Element | null {
  const { unavailable } = props.subject;
  return unavailable === undefined ? null : <UnplayableSign unavailable={unavailable} />;
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
    isShown: (subject) => subject.unavailable !== undefined,
    sign: CannotPlaySign,
    note: CannotPlayNote,
  },
  {
    name: "uncertainMatch",
    isShown: (subject) => subject.isGuessed === true,
    sign: UncertainMatchSign,
    note: UncertainMatchNote,
  },
];

/** A poster's tray of status cues, in statusCues' order; nothing when it has none (D278, D301 §4). */
export function StatusTray(props: { readonly subject: CueSubject }): JSX.Element | null {
  const shown = statusCues.filter((cue) => cue.isShown(props.subject));
  return shown.length === 0 ? null : (
    <span className="poster-grid__cues">
      {shown.map((cue) => createElement(cue.sign, { key: cue.name, subject: props.subject }))}
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
