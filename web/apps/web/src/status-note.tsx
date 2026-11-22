// SPDX-License-Identifier: MPL-2.0

import "./status-note.css";

import type { TFunction } from "i18next";
import { useCallback, useState, type JSX, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { WarningIcon, WatchedIcon } from "./icons.tsx";
import { problemText } from "./problem.ts";

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
      readonly onTryAgain?: (() => void) | undefined;
      /**
       * The id the fields a form's refusal is about are described by (field.tsx's useFormFields's
       * problemId), so a screen reader on a marked field hears why.
       */
      readonly id?: string | undefined;
      /**
       * Whether it tells of what happened before the person arrived (a session the server ended), a
       * polite status rather than an alert: nothing they did just failed.
       */
      readonly isFromBefore?: boolean;
    }
  | { readonly kind: "loading"; readonly text: string }
  | { readonly kind: "success"; readonly text: string };

/**
 * How a change that saves itself went (ui.md §14, a form that saves itself): "Saved", or the
 * refusal in its place, or nothing yet.
 */
export type SaveTold = Extract<StatusNoteTold, { kind: "success" | "problem" }> | undefined;

/**
 * A page's status, the one way a page shows one (D538): empty, where the list would be, plain text
 * and its next step; a problem, an alert, its warning sign, its words in the warning yellow, Try
 * again beside them, or under them in a narrow page; loading, muted, a polite status; success, a
 * polite status above its form's button with its tick, kept until the form is used again, and the
 * "Saved" beside a setting that saves itself (useSaveState). Focus stays on Try again as it is
 * pressed. Told nothing, it shows nothing: a form not yet sent, a setting not yet changed.
 */
export function StatusNote(props: {
  readonly told: StatusNoteTold | undefined;
}): JSX.Element | null {
  const { told } = props;
  if (told === undefined) {
    return null;
  }
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
  const content = (
    <>
      <NoteIcon>
        <WarningIcon />
      </NoteIcon>
      <span className="status-line__words">{told.text}</span>
      {told.onTryAgain === undefined ? null : (
        <button type="button" className="status-line__again" onClick={told.onTryAgain}>
          {t("statusNote.tryAgain")}
        </button>
      )}
    </>
  );
  return told.isFromBefore === true ? (
    <output className="status-line status-line--problem" id={told.id}>
      {content}
    </output>
  ) : (
    <div className="status-line status-line--problem" role="alert" id={told.id}>
      {content}
    </div>
  );
}

/**
 * The note of a field whose page keeps its refusals and saves by name: the refusal where it has a
 * problem, "Saved" where its last change was, else nothing.
 */
export function shownSaveTold(t: TFunction, problem: string, isSaved: boolean): SaveTold {
  if (problem !== "") {
    return { kind: "problem", text: problem };
  }
  return isSaved ? { kind: "success", text: t("common.saved") } : undefined;
}

/**
 * One field's saving at once: save sends a change and told says how it went, for its StatusNote;
 * the refusal in the viewer's words (problemText).
 */
export function useSaveState(): {
  readonly told: SaveTold;
  readonly save: (send: () => Promise<void>) => Promise<boolean>;
} {
  const { t } = useTranslation();
  const [told, setTold] = useState<SaveTold>(undefined);
  const save = useCallback(
    async (send: () => Promise<void>): Promise<boolean> => {
      try {
        await send();
        setTold({ kind: "success", text: t("common.saved") });
        return true;
      } catch (cause: unknown) {
        setTold({ kind: "problem", text: problemText(t, cause) });
        return false;
      }
    },
    [t],
  );
  return { told, save };
}
