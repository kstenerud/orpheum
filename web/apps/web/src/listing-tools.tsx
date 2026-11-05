// SPDX-License-Identifier: MPL-2.0

import "./listing-tools.css";

import { useCallback, useEffect, useRef, useState, type JSX, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { Icon } from "./icon.tsx";
import { ListChips, useGenres, type ChipsKind, type Genre } from "./list-chips.tsx";
import {
  clearedCriteria,
  criteriaKey,
  criteriaSummary,
  isNarrowed,
  type ListCriteria,
} from "./list-filters.ts";
import { keepsOpenAttribute, PopoverButton } from "./popover.tsx";
import { ActionChooser } from "./route-chooser.tsx";
import type { LibrarySort } from "./router.ts";
import {
  defaultOrderOf,
  sortChoiceLabel,
  sortChoicesOf,
  sortFieldLabel,
  type SortChoice,
} from "./sort-choices.ts";

/**
 * A listing page's tools, at the right of its top line beside its name (the owner, 2026-10-30): its
 * Sort and filter and, on one library's page, its view and its configure; the page gives them in
 * that order left to right.
 */
export function ListingTools(props: { readonly children: ReactNode }): JSX.Element {
  return <div className="listing-tools icon-tray">{props.children}</div>;
}

// The listing tools' icons, as their designer drew them (the owner's upload, 2026-10-30).

/** A gear: configure. */
export function GearIcon(): JSX.Element {
  return (
    <Icon className="listing-tools__icon">
      <path
        fillRule="evenodd"
        stroke="currentColor"
        strokeWidth=".8"
        strokeLinejoin="round"
        d="M9.28 4.90L9.39 1.52A10.8 10.8 0 0 1 14.61 1.52L14.72 4.90A7.6 7.6 0 0 1 16.78 6.09L19.77 4.50A10.8 10.8 0 0 1 22.38 9.02L19.51 10.81A7.6 7.6 0 0 1 19.51 13.19L22.38 14.98A10.8 10.8 0 0 1 19.77 19.50L16.78 17.91A7.6 7.6 0 0 1 14.72 19.10L14.61 22.48A10.8 10.8 0 0 1 9.39 22.48L9.28 19.10A7.6 7.6 0 0 1 7.22 17.91L4.23 19.50A10.8 10.8 0 0 1 1.62 14.98L4.49 13.19A7.6 7.6 0 0 1 4.49 10.81L1.62 9.02A10.8 10.8 0 0 1 4.23 4.50L7.22 6.09A7.6 7.6 0 0 1 9.28 4.90z M12 8.3a3.7 3.7 0 1 0 0 7.4a3.7 3.7 0 1 0 0-7.4z"
      />
    </Icon>
  );
}

/** Two arrows, one up and one down: sort, before what the list's pop-up is set to. */
export function SortIcon(): JSX.Element {
  return (
    <Icon className="listing-tools__icon">
      <path
        d="M7.5 20V4.5 M3.5 8.5 7.5 4.5 11.5 8.5 M16.5 4v15.5 M12.5 15.5 16.5 19.5 20.5 15.5"
        fill="none"
        stroke="currentColor"
        strokeWidth="2.4"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </Icon>
  );
}

/** An arrow up, a list running ascending (A to Z, oldest first), or down, descending. */
export function SortArrowIcon(props: { readonly isAscending: boolean }): JSX.Element {
  return (
    <Icon className="listing-tools__arrow">
      <path
        d={
          props.isAscending
            ? "M12 20V4.5 M7 9.5 12 4.5 17 9.5"
            : "M12 4v15.5 M7 14.5 12 19.5 17 14.5"
        }
        fill="none"
        stroke="currentColor"
        strokeWidth="2.4"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </Icon>
  );
}

/** A list's criteria as its page shows them while its pop-up is open, and how they change. */
export interface ShownCriteria {
  /** The criteria the pop-up shows: those it has changed while open, else the address's. */
  readonly shown: ListCriteria;
  /**
   * The criteria the list is asked for: those shown, but a text still being typed, which is asked for
   * once typing rests or the pop-up closes (useRestedText).
   */
  readonly listed: ListCriteria;
  readonly change: (next: ListCriteria) => void;
  /** Told as the pop-up opens and closes (PopoverButton's onOpenChange). */
  readonly onOpenChange: (isOpen: boolean) => void;
  /** The kind whose genres the chips offer; undefined for a list with no chips. */
  readonly chipsKind: ChipsKind | undefined;
  /** The chips' genres (useGenres): read once the pop-up opens or the address names a genre. */
  readonly genres: readonly Genre[];
}

/**
 * A list's criteria (ListCriteria), each change shown at once (D417; the UI audit's Q5: a setting
 * applies at once, no Apply), and kept in the address, written by write, so Back from a title opened
 * from the list returns to it as it was (the owner, 2026-11-02). Written as the pop-up closes, not at
 * each change: a step of the history while it is open lets go the entry it holds over the page, so a
 * television's Back, which is the browser's (D367 §10), would leave the page in place of closing it
 * (navigation.ts's whenNoneLaid). What it changed is shown until the address has it.
 */
export function useListCriteria(
  fromAddress: ListCriteria,
  chipsKind: ChipsKind | undefined,
  write: (criteria: ListCriteria) => void,
): ShownCriteria {
  const [held, setHeld] = useState<ListCriteria | undefined>(undefined);
  const [isOpened, setIsOpened] = useState(false);
  const [isOpen, setIsOpen] = useState(false);
  const heldRef = useRef<ListCriteria | undefined>(undefined);
  const isOpenRef = useRef(false);
  const addressKey = criteriaKey(fromAddress);
  const addressKeyRef = useRef(addressKey);
  const writeRef = useRef(write);
  useEffect(() => {
    addressKeyRef.current = addressKey;
    writeRef.current = write;
  });
  // The address has what was held, or something else moved it: it is what the list shows again.
  useEffect(() => {
    if (!isOpenRef.current) {
      heldRef.current = undefined;
      setHeld(undefined);
    }
  }, [addressKey]);
  const change = useCallback((next: ListCriteria) => {
    heldRef.current = next;
    setHeld(next);
    if (!isOpenRef.current) {
      writeRef.current(next);
    }
  }, []);
  const onOpenChange = useCallback((isOpen: boolean) => {
    isOpenRef.current = isOpen;
    setIsOpen(isOpen);
    if (isOpen) {
      setIsOpened(true);
      return;
    }
    const now = heldRef.current;
    if (now === undefined) {
      return;
    }
    if (criteriaKey(now) === addressKeyRef.current) {
      heldRef.current = undefined;
      setHeld(undefined);
      return;
    }
    writeRef.current(now);
  }, []);
  const shown = held ?? fromAddress;
  const listed = useRestedText(shown, isOpen);
  const genres = useGenres(chipsKind, isOpened || shown.filters.genre !== undefined);
  return { shown, listed, change, onOpenChange, chipsKind, genres };
}

/**
 * How long typing in a list's Sort and filter rests before the list is asked for what is typed: each
 * letter of a word is not a request (as the grouped search's field waited, before D417 took it out).
 */
export const typingRestMs = 200;

/**
 * The criteria a list is asked for, of those shown: where only the text differs from those last
 * asked for, those until typing rests typingRestMs; any other change, a chip, the sort or Clear, at
 * once, the text with it. Closing the pop-up (isTyping false) sends the text at once too: the list
 * is asked again before an arrow pressed after Escape can reach it, so focus moved onto a poster is
 * not left on a list the narrowed one then replaces (focus would fall to the page, stranding a
 * remote's person; the 11-08 batch).
 */
function useRestedText(shown: ListCriteria, isTyping: boolean): ListCriteria {
  const [listed, setListed] = useState(shown);
  const shownRef = useRef(shown);
  useEffect(() => {
    shownRef.current = shown;
  });
  const shownKey = criteriaKey(shown);
  const isTextAlone =
    isTyping &&
    listed.query.trim() !== shown.query.trim() &&
    criteriaKey({ ...listed, query: shown.query }) === shownKey;
  useEffect(() => {
    const rest = setTimeout(
      () => {
        setListed(shownRef.current);
      },
      isTextAlone ? typingRestMs : 0,
    );
    return () => {
      clearTimeout(rest);
    };
  }, [shownKey, isTextAlone]);
  return isTextAlone ? listed : shown;
}

/** A list's criteria's few words (criteriaSummary), the chosen genre named once its name is read. */
function useSummary(criteria: ShownCriteria): string {
  const { t } = useTranslation();
  const { genre } = criteria.shown.filters;
  const genreName = criteria.genres.find((each) => each.id === genre)?.name;
  return criteriaSummary(t, criteria.shown, genreName);
}

/**
 * A list's one button for what narrows and orders it (D417 §1; the owner, 2026-11-04: "It's all
 * criteria that filters what gets shown, and all of that including the search are additive to that
 * filter"): "Sort and filter" and, after it on its one line, what is set (criteriaSummary), opening a
 * pop-up (popover.tsx, from its top) of a field that narrows the list's posters by name, its chips
 * (list-chips.tsx) where it has them, its sort, and Clear, which clears every filter but the sort,
 * there while one is set. Each applies at once, the list changing under the pop-up (useListCriteria);
 * a choice or a press in it leaves it open (keepsOpenAttribute), for the next.
 */
export function ListCriteriaButton(props: {
  readonly criteria: ShownCriteria;
  /** The sorts the list offers, its own first (D366 §1). */
  readonly sorts: readonly LibrarySort[];
}): JSX.Element {
  const { t } = useTranslation();
  const summary = useSummary(props.criteria);
  return (
    <PopoverButton
      label={t("criteria.open")}
      opener={
        <>
          <SortIcon />
          <span className="list-criteria__open">{t("criteria.open")}</span>
          <span className="list-criteria__summary">{summary}</span>
        </>
      }
      openerClassName="list-criteria__button"
      openerNaming={{ "aria-label": `${t("criteria.open")}: ${summary}`, title: summary }}
      isFromItsTop
      onOpenChange={props.criteria.onOpenChange}
    >
      <CriteriaControls criteria={props.criteria} sorts={props.sorts} />
    </PopoverButton>
  );
}

/** What a list's Sort and filter's pop-up holds, one column (ListCriteriaButton). */
function CriteriaControls(props: {
  readonly criteria: ShownCriteria;
  readonly sorts: readonly LibrarySort[];
}): JSX.Element {
  const { t } = useTranslation();
  const { shown, change, chipsKind, genres } = props.criteria;
  const fieldRef = useRef<HTMLInputElement>(null);
  return (
    <div className="list-criteria" data-focus-container="" {...{ [keepsOpenAttribute]: "" }}>
      {/* A text field, not a search one: Escape in a search field with text clears it, in
          Chromium and Firefox, and does not close the pop-up (idiosyncrasies.md). */}
      <input
        ref={fieldRef}
        type="text"
        enterKeyHint="search"
        className="list-criteria__name"
        aria-label={t("criteria.name")}
        placeholder={t("criteria.name")}
        value={shown.query}
        onChange={(event) => {
          change({ ...shown, query: event.target.value });
        }}
      />
      {chipsKind === undefined ? null : (
        <ListChips
          genres={genres}
          filters={shown.filters}
          onChange={(filters) => {
            change({ ...shown, filters });
          }}
        />
      )}
      <SortChooser
        sorts={props.sorts}
        choice={shown.choice}
        onChoose={(choice) => {
          change({ ...shown, choice });
        }}
      />
      {isNarrowed(shown) ? (
        <button
          type="button"
          className="list-criteria__clear"
          onClick={() => {
            change(clearedCriteria(shown));
            // Clear goes as it clears: focus to the field, the pop-up's first control.
            fieldRef.current?.focus();
          }}
        >
          {t("criteria.clear")}
        </button>
      ) : null}
    </div>
  );
}

/** A choice's key among the sort's: its sort, and the way it runs when not the sort's own. */
function choiceValue(choice: SortChoice): string {
  return choice.order === defaultOrderOf(choice.sort)
    ? choice.sort
    : `${choice.sort}:${choice.order}`;
}

/** The order to list in: each sort the list offers, either way (D366 §2), the page keeping it. */
function SortChooser(props: {
  readonly sorts: readonly LibrarySort[];
  readonly choice: SortChoice;
  readonly onChoose: (choice: SortChoice) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const chosen = choiceValue(props.choice);
  return (
    <ActionChooser
      fieldLabel={t("browse.sort")}
      current={sortChoiceLabel(t, props.choice)}
      options={sortChoicesOf(props.sorts).map((choice) => ({
        key: choiceValue(choice),
        label: sortFieldLabel(t, choice.sort),
        mark: <SortArrowIcon isAscending={choice.order === "ascending"} />,
        spoken: sortChoiceLabel(t, choice),
        isCurrent: choiceValue(choice) === chosen,
        onChoose: () => {
          props.onChoose(choice);
        },
      }))}
    />
  );
}
