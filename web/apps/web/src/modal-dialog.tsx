// SPDX-License-Identifier: MPL-2.0

import "./modal-dialog.css";

import {
  use,
  useId,
  useLayoutEffect,
  useRef,
  type JSX,
  type ReactNode,
  type RefObject,
} from "react";
import { useTranslation } from "react-i18next";

import { choiceArrivalIn } from "./focus-navigation.ts";
import { useLatest } from "./latest.ts";
import { useModalLayer } from "./modal-layer.tsx";
import { GrownFromContext, GrownParts, useGrownOutOf } from "./popover.tsx";

/** What a modal dialog is told. */
interface ModalDialogProps {
  /** Its name for a screen reader: its heading, or its message where it has none. */
  readonly label: string;
  readonly onClose: () => void;
  /** "alertdialog" for a notice that asks only to be read. */
  readonly role?: "dialog" | "alertdialog";
  /**
   * Its heading, kept at its top while its body scrolls under it (D538 §2), where it has one drawn
   * apart from what it holds.
   */
  readonly heading?: string;
  /**
   * Its buttons, kept at its foot while its body scrolls (D538 §2): the secondary first, then the
   * main one, which carries arrivalMark (focus-navigation.ts), where focus arrives (D538 §4).
   */
  readonly footer?: ReactNode;
  /**
   * Whether it is a confirm, at most 24rem wide at any width, read out as an alert with what it says
   * (D538 §4): a question before what cannot be undone (ConfirmDialog).
   */
  readonly isConfirm?: boolean;
  /**
   * Whether it is wide, 40rem where the screen has room, not a notice's 26rem: a path and its
   * folders (the folder browser), or what a setting is for, need room.
   */
  readonly isWide?: boolean;
  /**
   * Whether it is large, 48rem wide and 85% of the screen's height where it has room, its height
   * fixed so moving between folders never resizes it (D539 §6): the file browser's Choose.
   */
  readonly isLarge?: boolean;
  /**
   * Offered Back and Escape first, whether it took them: a file browser going up a folder, the
   * dialog closing only at its top (D539 §4).
   */
  readonly onBack?: () => boolean;
  /** What its top edge says, where it says anything: "warning", a notice in the warning yellow. */
  readonly mark?: "warning";
  readonly children: ReactNode;
}

/**
 * The dialogs on the screen, the one shown last: a dialog opened from a dialog replaces it, never
 * stacks on it (D538 §5), and closing it brings the one under it back.
 */
const shownDialogs: HTMLDialogElement[] = [];

/** The attribute marking a dialog another has replaced: still open, not shown (modal-dialog.css). */
const replacedAttribute = "data-replaced";

/**
 * Hides the dialog under the one in ref as it opens, and shows it again as this one closes (D538
 * §5). Hidden, not closed: what this one holds may be drawn inside the one under it, and a closed
 * dialog draws nothing. Called before useModalLayer, so the one under it is shown again before
 * focus is given back into it.
 */
function useReplacesTheOneUnder(ref: RefObject<HTMLDialogElement | null>): void {
  useLayoutEffect(() => {
    const dialog = ref.current;
    if (dialog === null) {
      return undefined;
    }
    shownDialogs.at(-1)?.setAttribute(replacedAttribute, "");
    shownDialogs.push(dialog);
    return () => {
      const at = shownDialogs.indexOf(dialog);
      shownDialogs.splice(at, 1);
      if (at === shownDialogs.length) {
        shownDialogs.at(-1)?.removeAttribute(replacedAttribute);
      }
    };
  }, [ref]);
}

/**
 * Where a confirm's focus arrives (D538 §4): its least harmful choice at its foot, Cancel before the
 * dangerous action, never a box to tick in its body.
 */
function confirmArrivalIn(root: HTMLElement): HTMLElement | undefined {
  return choiceArrivalIn(root.querySelector<HTMLElement>(".modal-dialog__foot") ?? root);
}

/**
 * A modal dialog, the one way the app shows one (the owner, 2026-10-11: "these dialogs should be
 * modal ... darken the rest"): the modal layer (modal-layer.tsx: the browser's own modal, Escape,
 * Back, a press outside, focus given back) in the middle of a darkened screen, a sheet from the
 * screen's bottom on a phone, the arrows moving among its own controls alone (focus-navigation.ts).
 * It has no cross (D538 §2): Escape, Back, a press outside and its own buttons close it. Given a
 * heading and a footer, those stay in view while its body scrolls. One opened from it replaces it
 * (D538 §5). It takes no class from where it is drawn: its looks are named options (plans/toolkit.md).
 * Focus arrives, as it opens, on its first field to type in; else on its marked arrival, its main
 * action; else its first choice not marked a dangerous action (dangerousMark), or, where every choice
 * is, on none: on the dialog itself (D421, D538 §4). A pop-up is not one (popover.tsx, D408 §2).
 */
export function ModalDialog(props: ModalDialogProps): JSX.Element {
  const ref = useRef<HTMLDialogElement>(null);
  const bodyId = useId();
  // The latest of what it is told, read as it closes: a page passes a new onClose at each render,
  // and opening the dialog again at each would reset its focus.
  const toldRef = useLatest({
    onClose: props.onClose,
    shouldFocusFirstField: true,
    arrivalOf: props.isConfirm === true ? confirmArrivalIn : undefined,
    onBack: props.onBack,
  });
  useReplacesTheOneUnder(ref);
  useModalLayer(ref, toldRef);
  const grownFrom = use(GrownFromContext);
  useGrownOutOf(ref, grownFrom, props.onClose);
  const isConfirm = props.isConfirm === true;
  const drawn = hasPartsIn(props) ? (
    <DialogParts heading={props.heading} footer={props.footer} bodyId={bodyId}>
      {props.children}
    </DialogParts>
  ) : (
    props.children
  );
  return (
    // tabIndex -1: a dialog taller than the window scrolls, and a box that scrolls is a Tab stop of its
    // own in Chromium and Firefox, reached as nothing (the Cast and Watch Together dialogs, 2026-11-04).
    <dialog
      ref={ref}
      className={classesOf(props, grownFrom !== undefined)}
      role={isConfirm ? "alertdialog" : props.role}
      aria-label={props.label}
      aria-describedby={isConfirm ? bodyId : undefined}
      tabIndex={-1}
    >
      <GrownFromContext value={undefined}>
        {grownFrom === undefined ? (
          drawn
        ) : (
          <>
            <GrownParts tab={grownFrom.tab} panelClass="modal-dialog__panel">
              {drawn}
            </GrownParts>
            <div className="popover__room" aria-hidden="true" />
          </>
        )}
      </GrownFromContext>
    </dialog>
  );
}

/** Whether a dialog is drawn in parts, its heading and its foot apart from its body. */
function hasPartsIn(props: ModalDialogProps): boolean {
  return props.heading !== undefined || props.footer !== undefined;
}

/** A dialog's classes: its looks, as named options. */
function classesOf(props: ModalDialogProps, isGrown: boolean): string {
  return [
    "modal-dialog",
    isGrown ? "modal-dialog--grown" : "",
    hasPartsIn(props) ? "modal-dialog--parts" : "",
    props.isConfirm === true ? "modal-dialog--confirm" : "",
    props.isWide === true ? "modal-dialog--wide" : "",
    props.isLarge === true ? "modal-dialog--large" : "",
    props.mark === "warning" ? "modal-dialog--warning" : "",
  ]
    .filter((name) => name !== "")
    .join(" ");
}

/** A dialog's heading, its body, which scrolls between them, and its foot (D538 §2). */
function DialogParts(props: {
  readonly heading: string | undefined;
  readonly footer: ReactNode;
  readonly bodyId: string;
  readonly children: ReactNode;
}): JSX.Element {
  return (
    <>
      {props.heading === undefined ? null : (
        <header className="modal-dialog__head">
          <h2>{props.heading}</h2>
        </header>
      )}
      <div id={props.bodyId} className="modal-dialog__body" tabIndex={-1}>
        {props.children}
      </div>
      {props.footer === undefined ? null : (
        <footer className="modal-dialog__foot">{props.footer}</footer>
      )}
    </>
  );
}

/**
 * The button at the foot of a dialog that only tells or shows, with nothing to do (D538 §2: no
 * cross): "Done", which closes it (ui.md rule 6).
 */
export function DialogClose(props: { readonly onClose: () => void }): JSX.Element {
  const { t } = useTranslation();
  return (
    <button type="button" onClick={props.onClose}>
      {t("dialog.done")}
    </button>
  );
}
