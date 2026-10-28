// SPDX-License-Identifier: MPL-2.0

import "./route-chooser.css";

import { useCallback, useId, useState, type JSX, type ReactNode } from "react";

import { Icon } from "./icon.tsx";
import { Link } from "./link.tsx";
import { ModalDialog } from "./modal-dialog.tsx";
import type { Route } from "./router.ts";

/** One choice among several, each a route: a library, a view, a sort, a season. */
export interface Choice {
  readonly key: string;
  readonly label: string;
  readonly to: Route;
  readonly isCurrent: boolean;
  /** What shows after its label, as a season's watched check (D370 §3). */
  readonly aside?: ReactNode;
}

/**
 * One choice among several, each a route: a button, marked by a chevron as opening a list (the
 * owner, 2026-10-28: it looked "no different from a regular button"), that opens a focusable
 * drop-down of options
 * (modal-dialog.tsx: a native select does not work reliably on a television's browser,
 * library-browsing.md §5.5), one list for the Libraries page's library, view and sort and a series'
 * season alike. A choice of a view of the page, a sort or a season, replaces the history's entry,
 * not adding one (D367 §9); a choice of another page, a library, adds one.
 */
export function RouteChooser(props: {
  readonly fieldLabel: string;
  /**
   * Whether the field's name shows beside it; a screen reader hears it either way. The library
   * chooser's does not: what it chooses is plain from its choices (the owner, 2026-10-27).
   */
  readonly isFieldLabelShown?: boolean;
  readonly current: string;
  readonly options: readonly Choice[];
  /** Whether a choice is a view of the page, which replaces its entry in the history. */
  readonly isAView?: boolean;
}): JSX.Element {
  const [isOpen, setIsOpen] = useState(false);
  const fieldId = useId();
  const close = useCallback(() => {
    setIsOpen(false);
  }, []);
  return (
    <div className="route-chooser">
      {props.isFieldLabelShown === false ? null : (
        <span className="route-chooser__field" id={fieldId}>
          {props.fieldLabel}
        </span>
      )}
      <button
        type="button"
        className="route-chooser__button"
        aria-haspopup="dialog"
        aria-expanded={isOpen}
        {...(props.isFieldLabelShown === false
          ? { "aria-label": `${props.fieldLabel}: ${props.current}` }
          : { "aria-describedby": fieldId })}
        onClick={() => {
          setIsOpen((wasOpen) => !wasOpen);
        }}
      >
        {props.current}
        <ChevronDownIcon />
      </button>
      {isOpen ? (
        <ChoicesList
          label={props.fieldLabel}
          options={props.options}
          isAView={props.isAView === true}
          onClose={close}
        />
      ) : null}
    </div>
  );
}

/** A chooser's open list: its options, each a link, in a drop-down a press outside closes. */
function ChoicesList(props: {
  readonly label: string;
  readonly options: readonly Choice[];
  readonly isAView: boolean;
  readonly onClose: () => void;
}): JSX.Element {
  return (
    <ModalDialog
      className="modal-dialog--drop-down route-chooser__panel"
      label={props.label}
      onClose={props.onClose}
      shouldCloseOnOutsidePress
    >
      <ul>
        {props.options.map((option) => (
          <li key={option.key}>
            <Link
              to={option.to}
              isCurrent={option.isCurrent}
              isReplacing={props.isAView}
              onNavigate={props.onClose}
            >
              {option.label}
              {option.aside}
            </Link>
          </li>
        ))}
      </ul>
    </ModalDialog>
  );
}

/** A chooser's mark: a chevron pointing down, to the list it opens. */
export function ChevronDownIcon(): JSX.Element {
  return (
    <Icon className="route-chooser__chevron">
      <path d="M5.6 8.3 12 14.7l6.4-6.4 1.4 1.4-7.8 7.8-7.8-7.8z" />
    </Icon>
  );
}
