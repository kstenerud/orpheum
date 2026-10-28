// SPDX-License-Identifier: MPL-2.0

import "./route-chooser.css";

import { useId, type JSX, type ReactNode } from "react";

import { DropDown } from "./drop-down.tsx";
import { Icon } from "./icon.tsx";
import { Link } from "./link.tsx";
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
 * One choice among several that changes a page's view in place, kept by the page, not its address:
 * the Films and Series pages' sort, a library's files' filing.
 */
export interface ActionChoice {
  readonly key: string;
  readonly label: string;
  readonly isCurrent: boolean;
  readonly onChoose: () => void;
}

/** What a chooser of either kind is told beside its places. */
interface ChooserTold {
  readonly fieldLabel: string;
  /**
   * Whether the field's name shows beside it; a screen reader hears it either way. The library
   * chooser's does not: what it chooses is plain from its choices (the owner, 2026-10-27).
   */
  readonly isFieldLabelShown?: boolean;
  readonly current: string;
}

/**
 * A chooser's field name and its drop-down (drop-down.tsx) of places, its button showing the
 * current choice and a chevron, for both kinds of chooser.
 */
function Chooser(props: ChooserTold & { readonly children: ReactNode }): JSX.Element {
  const fieldId = useId();
  return (
    <div className="route-chooser">
      {props.isFieldLabelShown === false ? null : (
        <span className="route-chooser__field" id={fieldId}>
          {props.fieldLabel}
        </span>
      )}
      <DropDown
        label={props.fieldLabel}
        opener={
          <>
            {props.current}
            <ChevronDownIcon />
          </>
        }
        openerClassName="route-chooser__button"
        listClassName="route-chooser__panel"
        openerNaming={
          props.isFieldLabelShown === false
            ? { "aria-label": `${props.fieldLabel}: ${props.current}` }
            : { "aria-describedby": fieldId }
        }
      >
        {props.children}
      </DropDown>
    </div>
  );
}

/**
 * One choice among several, each a route: a drop-down (drop-down.tsx; a native select does not work
 * reliably on a television's browser, library-browsing.md §5.5) whose button shows the current
 * choice, marked by a chevron as opening a list (the owner, 2026-10-28: it looked "no different from
 * a regular button"), one list for the Libraries page's library, view and sort and a series' season
 * alike. A choice of a view of the page, a sort or a season, replaces the history's entry, not adding
 * one (D367 §9); a choice of another page, a library, adds one.
 */
export function RouteChooser(
  props: ChooserTold & {
    readonly options: readonly Choice[];
    /** Whether a choice is a view of the page, which replaces its entry in the history. */
    readonly isAView?: boolean;
  },
): JSX.Element {
  return (
    <Chooser {...props}>
      {props.options.map((option) => (
        <li key={option.key}>
          <Link to={option.to} isCurrent={option.isCurrent} isReplacing={props.isAView === true}>
            {option.label}
            {option.aside}
          </Link>
        </li>
      ))}
    </Chooser>
  );
}

/**
 * One choice among several that the page keeps, not its address, as RouteChooser's list and button:
 * the Films and Series pages' sort, a native select there until the owner found it (2026-10-28: "The
 * sort chooser on the films page was missed in the revamp"), and a library's files' filing.
 */
export function ActionChooser(
  props: ChooserTold & { readonly options: readonly ActionChoice[] },
): JSX.Element {
  return (
    <Chooser {...props}>
      {props.options.map((option) => (
        <li key={option.key}>
          <button
            type="button"
            aria-current={option.isCurrent ? "true" : undefined}
            onClick={option.onChoose}
          >
            {option.label}
          </button>
        </li>
      ))}
    </Chooser>
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
