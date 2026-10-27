// SPDX-License-Identifier: MPL-2.0

import "./route-chooser.css";

import { useCallback, useId, useState, type JSX, type ReactNode } from "react";

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
 * One choice among several, each a route: a button that opens a focusable drop-down of options
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
        aria-expanded={isOpen}
        {...(props.isFieldLabelShown === false
          ? { "aria-label": `${props.fieldLabel}: ${props.current}` }
          : { "aria-describedby": fieldId })}
        onClick={() => {
          setIsOpen((wasOpen) => !wasOpen);
        }}
      >
        {props.current}
      </button>
      {isOpen ? (
        <ModalDialog
          className="modal-dialog--drop-down route-chooser__panel"
          label={props.fieldLabel}
          onClose={close}
          shouldCloseOnOutsidePress
        >
          <ul>
            {props.options.map((option) => (
              <li key={option.key}>
                <Link
                  to={option.to}
                  isCurrent={option.isCurrent}
                  isReplacing={props.isAView === true}
                  onNavigate={close}
                >
                  {option.label}
                  {option.aside}
                </Link>
              </li>
            ))}
          </ul>
        </ModalDialog>
      ) : null}
    </div>
  );
}
