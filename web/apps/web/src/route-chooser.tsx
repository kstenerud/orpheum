// SPDX-License-Identifier: MPL-2.0

import "./route-chooser.css";

import { useId, type JSX, type ReactNode } from "react";

import { DropDown } from "./drop-down.tsx";
import { ChevronDownIcon } from "./icons.tsx";
import { ListOption } from "./list-box.tsx";
import type { Route } from "./router.ts";

/** One choice among several, each a route: a view, a sort. */
export interface Choice {
  readonly key: string;
  readonly label: string;
  /** A mark drawn before the label: a sort's arrow. */
  readonly mark?: ReactNode;
  /** The choice as a screen reader says it, where the mark says part of it: "Name, A–Z". */
  readonly spoken?: string;
  readonly to: Route;
  readonly isCurrent: boolean;
}

/**
 * One choice among several that changes a page's view in place, kept by the page, not its address:
 * the Films and Series pages' sort, a library's files' filing.
 */
export interface ActionChoice {
  readonly key: string;
  readonly label: string;
  /** A mark drawn before the label: a sort's arrow. */
  readonly mark?: ReactNode;
  /** The choice as a screen reader says it, where the mark says part of it. */
  readonly spoken?: string;
  readonly isCurrent: boolean;
  readonly onChoose: () => void;
}

/** What a chooser's button shows: its current choice's label, else its first's, else nothing. */
export function currentLabelOf(options: readonly { label: string; isCurrent: boolean }[]): string {
  return (options.find((option) => option.isCurrent) ?? options[0])?.label ?? "";
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
  /**
   * An icon its button shows alone, in place of the current choice: a listing's sort (the owner,
   * 2026-10-30: "put the whole sort thing behind an icon"), its choice seen in the list it opens.
   */
  readonly openerIcon?: ReactNode;
  /** Whether its icon is drawn over a film, as the player's overlay's 💬 and 🎧 are (D407 §8). */
  readonly isOverVideo?: boolean;
  /** Told each time its list opens or closes (DropDown's onOpenChange). */
  readonly onOpenChange?: ((isOpen: boolean) => void) | undefined;
}

/**
 * A chooser's field name and its drop-down (drop-down.tsx) of places, its button showing the
 * current choice and a chevron, for both kinds of chooser.
 */
function Chooser(props: ChooserTold & { readonly children: ReactNode }): JSX.Element {
  const fieldId = useId();
  const isIcon = props.openerIcon !== undefined;
  return (
    <div className="route-chooser">
      {props.isFieldLabelShown === false || isIcon ? null : (
        <span className="route-chooser__field" id={fieldId}>
          {props.fieldLabel}
        </span>
      )}
      <DropDown
        label={props.fieldLabel}
        opener={
          isIcon ? (
            props.openerIcon
          ) : (
            <>
              {props.current}
              <ChevronDownIcon />
            </>
          )
        }
        openerClassName={openerClassOf(isIcon, props.isOverVideo === true)}
        onOpenChange={props.onOpenChange}
        openerNaming={
          props.isFieldLabelShown === false || isIcon
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
 * a regular button"), one list for a library's page's view and sort alike (a series' seasons are
 * a row of posters, D379; another library is the banner's Libraries menu's). A choice of a view of
 * the page, a sort, replaces the history's entry, not adding one (D367 §9); a choice of another page
 * adds one.
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
        <ListOption
          key={option.key}
          to={option.to}
          isCurrent={option.isCurrent}
          isReplacing={props.isAView === true}
          spoken={option.spoken}
        >
          {option.mark}
          {option.label}
        </ListOption>
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
        <ListOption
          key={option.key}
          isCurrent={option.isCurrent}
          spoken={option.spoken}
          onChoose={option.onChoose}
        >
          {option.mark}
          {option.label}
        </ListOption>
      ))}
    </Chooser>
  );
}

/** A chooser's button's classes: an icon's, over a film or not, or the current choice's. */
function openerClassOf(isIcon: boolean, isOverVideo: boolean): string {
  if (!isIcon) {
    return "route-chooser__button";
  }
  return isOverVideo
    ? "icon-button icon-button--over-video route-chooser__button"
    : "icon-button route-chooser__button";
}
