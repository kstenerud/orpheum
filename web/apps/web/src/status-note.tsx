// SPDX-License-Identifier: MPL-2.0

import "./status-note.css";

import type { JSX, ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { WarningIcon, WatchedIcon } from "./icons.tsx";

/**
 * What a status note says (D538): that a list or a panel is empty and the next step; that something
 * could not be done, and Try again where trying again can help; that it is still loading; or how a
 * submitted form went.
 */
export type StatusNoteTold =
  | {
      readonly kind: "empty";
      /** Why there is nothing, a sentence. */
      readonly text: string;
      readonly detail?: string;
      /** The next step: its words and what it does. */
      readonly next?: { readonly label: string; readonly onPress: () => void };
    }
  | {
      readonly kind: "problem";
      /** What could not be done, never only "Error". */
      readonly text: string;
      /** Where trying again can help. */
      readonly onTryAgain?: () => void;
    }
  | { readonly kind: "loading"; readonly text: string }
  | { readonly kind: "success"; readonly text: string };

/**
 * A page's status, the one way a page shows one (D538): empty, where the list would be, plain text
 * and its next step; a problem, an alert, its warning sign, its words in the warning yellow, Try
 * again beside them, or under them in a narrow page; loading, muted, a polite status; success, a
 * polite status above its form's button with its tick, kept until the form is used again. Focus
 * stays on Try again as it is pressed.
 */
export function StatusNote(props: { readonly told: StatusNoteTold }): JSX.Element {
  const { told } = props;
  switch (told.kind) {
    case "empty": {
      return <EmptyNote told={told} />;
    }
    case "problem": {
      return <ProblemNote told={told} />;
    }
    case "loading": {
      return (
        <output className="status-line status-line--loading" aria-live="polite">
          {told.text}
        </output>
      );
    }
    case "success": {
      return (
        <output className="status-line status-line--success">
          <NoteIcon>
            <WatchedIcon />
          </NoteIcon>
          <span>{told.text}</span>
        </output>
      );
    }
  }
}

/** A status note's icon, decoration beside its words. */
function NoteIcon(props: { readonly children: ReactNode }): JSX.Element {
  return (
    <span className="status-line__icon" aria-hidden="true">
      {props.children}
    </span>
  );
}

/** An empty list's or panel's note: why, its detail, its next step (StatusNote). */
function EmptyNote(props: {
  readonly told: Extract<StatusNoteTold, { kind: "empty" }>;
}): JSX.Element {
  const { told } = props;
  return (
    <div className="status-line status-line--empty">
      <p className="status-line__sentence">{told.text}</p>
      {told.detail === undefined ? null : <p className="status-line__detail">{told.detail}</p>}
      {told.next === undefined ? null : (
        <button type="button" onClick={told.next.onPress}>
          {told.next.label}
        </button>
      )}
    </div>
  );
}

/** What could not be done, and Try again (StatusNote). */
function ProblemNote(props: {
  readonly told: Extract<StatusNoteTold, { kind: "problem" }>;
}): JSX.Element {
  const { t } = useTranslation();
  const { told } = props;
  return (
    <div className="status-line status-line--problem" role="alert">
      <NoteIcon>
        <WarningIcon />
      </NoteIcon>
      <span className="status-line__words">{told.text}</span>
      {told.onTryAgain === undefined ? null : (
        <button type="button" className="status-line__again" onClick={told.onTryAgain}>
          {t("statusNote.tryAgain")}
        </button>
      )}
    </div>
  );
}
