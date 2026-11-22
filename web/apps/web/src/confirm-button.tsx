// SPDX-License-Identifier: MPL-2.0

import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { dangerousMark } from "./focus-navigation.ts";
import { ModalDialog } from "./modal-dialog.tsx";

/** What a confirm says and does, ConfirmButton's and ConfirmDialog's alike. */
interface ConfirmWords {
  /** The question, its heading: "Delete the library?". */
  readonly question: string;
  /** What will happen, in one sentence, said before the person confirms. */
  readonly consequence: string;
  /** What the confirming button says, a verb: "Delete it", never "OK". */
  readonly confirmLabel: string;
  /**
   * Whether the action is disruptive but harmless, as restarting the server or a new link is: its
   * button plain, not in the irreversible red (ui.md rule 8). Its focus still starts on Cancel.
   */
  readonly isHarmless?: boolean;
}

/**
 * A button for what cannot be undone, or is disruptive (ui.md rule 8, D538 §4): pressed, it sends
 * nothing and opens the confirm, ConfirmDialog, whose action alone acts. Outlined in the irreversible
 * red (D278 §4) unless the action is harmless. The confirm opened from a dialog replaces that dialog,
 * which comes back as it closes (D538 §5), focus on this button.
 */
export function ConfirmButton(
  props: ConfirmWords & {
    /** What the button says. */
    readonly label: string;
    /** What a screen reader says of the button, where its label alone does not name what it acts on. */
    readonly labelForReader?: string;
    /** Whether it cannot be pressed now, as a role some people hold. */
    readonly isDisabled?: boolean;
    readonly onConfirm: () => void;
  },
): JSX.Element {
  const [isAsking, setIsAsking] = useState(false);
  return (
    <>
      <button
        type="button"
        className={props.isHarmless === true ? undefined : "irreversible-outline"}
        aria-label={props.labelForReader}
        aria-haspopup="dialog"
        disabled={props.isDisabled}
        onClick={() => {
          setIsAsking(true);
        }}
      >
        {props.label}
      </button>
      {isAsking ? (
        <ConfirmDialog
          question={props.question}
          consequence={props.consequence}
          confirmLabel={props.confirmLabel}
          {...(props.isHarmless === undefined ? {} : { isHarmless: props.isHarmless })}
          onConfirm={props.onConfirm}
          onClose={() => {
            setIsAsking(false);
          }}
        />
      ) : null}
    </>
  );
}

/**
 * The confirm, the one way the app asks before an action (ui.md rule 8, D538 §4): the Dialog's
 * confirm (modal-dialog.tsx), the question its heading, what will happen, in the irreversible red
 * unless the action is harmless, and at its foot "Cancel", first and focused as it opens, then the
 * action, filled red and marked a dangerous action, so a remote's OK pressed once too often, or a held
 * Enter, does nothing (the owner, 2026-11-22: "the confirmation defaults to not doing it"). Cancel,
 * Escape, Back or a press outside close it, sending nothing. ConfirmButton opens it; an action under
 * a "…" (more-actions.tsx) opens it itself.
 */
export function ConfirmDialog(
  props: ConfirmWords & {
    /** A choice to tick with the action, unticked as the dialog opens: the box's words. */
    readonly option?: string;
    /** The action, told whether the option was ticked. */
    readonly onConfirm: (isOptionTicked: boolean) => void;
    readonly onClose: () => void;
  },
): JSX.Element {
  const { t } = useTranslation();
  const [isOptionTicked, setIsOptionTicked] = useState(false);
  const isHarmless = props.isHarmless === true;
  return (
    <ModalDialog
      label={props.question}
      heading={props.question}
      isConfirm
      onClose={props.onClose}
      footer={
        <>
          <button type="button" onClick={props.onClose}>
            {t("dialog.cancel")}
          </button>
          <button
            type="button"
            className={isHarmless ? undefined : "irreversible"}
            {...dangerousMark}
            onClick={() => {
              props.onClose();
              props.onConfirm(isOptionTicked);
            }}
          >
            {props.confirmLabel}
          </button>
        </>
      }
    >
      <p className={isHarmless ? undefined : "irreversible-words"}>{props.consequence}</p>
      {props.option === undefined ? null : (
        <p>
          <label>
            <input
              type="checkbox"
              checked={isOptionTicked}
              onChange={(event) => {
                setIsOptionTicked(event.target.checked);
              }}
            />{" "}
            {props.option}
          </label>
        </p>
      )}
    </ModalDialog>
  );
}
