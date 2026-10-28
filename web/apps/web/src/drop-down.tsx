// SPDX-License-Identifier: MPL-2.0

import "./drop-down.css";

import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type JSX,
  type ReactNode,
  type RefObject,
} from "react";
import { useTranslation } from "react-i18next";

import { IconButton } from "./icon-button.tsx";
import { useCommands } from "./input-layer.tsx";
import { ModalDialog } from "./modal-dialog.tsx";
import { CloseIcon } from "./player-icons.tsx";

/** What names the opening button for a screen reader and a pointer's hover, where its content does not. */
export interface OpenerNaming {
  readonly "aria-label"?: string;
  readonly "aria-describedby"?: string;
  readonly title?: string;
}

/**
 * Brings a focused place of the list wholly into its view, where the list scrolls within itself:
 * Firefox left a place partly below its end in view (the browser test, 2026-10-28), which a remote's
 * focus could then not show whole.
 */
function useFocusedPlaceShown(ref: RefObject<HTMLElement | null>): void {
  useEffect(() => {
    const list = ref.current;
    if (list === null) {
      return undefined;
    }
    const show = (event: FocusEvent): void => {
      if (event.target instanceof HTMLElement) {
        event.target.scrollIntoView({ block: "nearest", inline: "nearest" });
      }
    };
    list.addEventListener("focusin", show);
    return () => {
      list.removeEventListener("focusin", show);
    };
  }, [ref]);
}

/**
 * Closes the list when one of its places, a link or a button, is pressed: after what it does, as a
 * choice's route followed (link.tsx), heard at the document, past the page's root. Heard on the list
 * itself, the close ran first, between the browser's listeners, and the list was gone before React
 * heard the press at its root, so a link's own handling never ran and the browser followed it (the
 * browser test, 2026-10-28).
 */
function useChoiceCloses(ref: RefObject<HTMLElement | null>, close: () => void): void {
  useEffect(() => {
    const chosen = (event: MouseEvent): void => {
      const list = ref.current;
      const { target } = event;
      if (
        list !== null &&
        target instanceof Element &&
        list.contains(target) &&
        target.closest("a, button") !== null
      ) {
        close();
      }
    };
    document.addEventListener("click", chosen);
    return () => {
      document.removeEventListener("click", chosen);
    };
  }, [ref, close]);
}

/**
 * The one drop-down: a button that opens a list over the page, a chooser's (route-chooser.tsx) and
 * the banner's menu (app-shell.tsx) alike, the owner, 2026-10-28: "the menu should share code with
 * it, the only real difference being that there's no extra visual indicator that it's a dropdown",
 * and "the drop-down menu should be modal". The list is a modal dialog (modal-dialog.tsx): the page
 * behind it takes no press and focus stays in it while it is open. It closes when one of its places
 * is pressed; at a press outside it, which presses nothing else; at Escape and at Back, a key's or
 * the browser's own history step (Silk's on a Fire TV, D367 §10); at Left or Right, the list being one
 * column whose Up and Down move within it (the owner, 2026-10-28: "moving left/right while in a
 * chooser or menu closes it"), the press closing it alone; and by its own Close after its places,
 * which a remote's arrows reach. Focus goes back to the button (D361 §3).
 */
export function DropDown(props: {
  /** The list's name for a screen reader. */
  readonly label: string;
  /** What the button shows: a chooser's choice and its chevron, the menu's icon. */
  readonly opener: ReactNode;
  readonly openerClassName: string;
  readonly openerNaming: OpenerNaming;
  /** The class of the list's dialog, beside the drop-down's own. */
  readonly listClassName?: string;
  /** Its places, each a list item. */
  readonly children: ReactNode;
}): JSX.Element {
  const [isOpen, setIsOpen] = useState(false);
  const close = useCallback(() => {
    setIsOpen(false);
  }, []);
  return (
    <>
      <button
        type="button"
        className={props.openerClassName}
        aria-haspopup="dialog"
        aria-expanded={isOpen}
        {...props.openerNaming}
        onClick={() => {
          setIsOpen((wasOpen) => !wasOpen);
        }}
      >
        {props.opener}
      </button>
      {isOpen ? (
        <DropDownList label={props.label} className={props.listClassName} onClose={close}>
          {props.children}
        </DropDownList>
      ) : null}
    </>
  );
}

/** A drop-down's open list: its Close at the top-right, over its places. */
function DropDownList(props: {
  readonly label: string;
  readonly className: string | undefined;
  readonly onClose: () => void;
  readonly children: ReactNode;
}): JSX.Element {
  const { t } = useTranslation();
  const ref = useRef<HTMLDivElement>(null);
  const { onClose } = props;
  useChoiceCloses(ref, onClose);
  useFocusedPlaceShown(ref);
  useCommands(ref, (detail, event) => {
    if (detail.command === "left" || detail.command === "right") {
      event.preventDefault();
      onClose();
    }
  });
  const classes = ["modal-dialog--drop-down", "drop-down__list", props.className]
    .filter(Boolean)
    .join(" ");
  return (
    <ModalDialog
      className={classes}
      label={props.label}
      onClose={onClose}
      shouldCloseOnOutsidePress
    >
      {/* Its Close, a cross at the top-right that stays put, its places scrolling under it (the
          owner, 2026-10-28: "a static close button in the top-right corner that the scrollable
          portion scrolls under"). */}
      <IconButton
        label={t("dropDown.close")}
        icon={<CloseIcon />}
        onClick={onClose}
        className="drop-down__close"
      />
      <div ref={ref} className="drop-down__places">
        <ul>{props.children}</ul>
      </div>
    </ModalDialog>
  );
}
