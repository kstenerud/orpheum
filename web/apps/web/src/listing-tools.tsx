// SPDX-License-Identifier: MPL-2.0

import "./listing-tools.css";

import { useState, type JSX, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { Icon } from "./icon.tsx";
import { IconButton } from "./icon-button.tsx";
import { ModalDialog } from "./modal-dialog.tsx";
import { keepRecentSearch } from "./recent-searches.ts";
import { SearchFound, type GenreList } from "./search-found.tsx";

/**
 * A listing page's tools, at the right of its top line beside its name (the owner, 2026-10-30): read
 * right to left, its sort, its search and, on one library's page, its configure, each an icon, as
 * close together as the banner's tray's; the page gives them in that order left to right.
 */
export function ListingTools(props: { readonly children: ReactNode }): JSX.Element {
  return <div className="listing-tools icon-tray">{props.children}</div>;
}

// The listing tools' icons, as their designer drew them (the owner's upload, 2026-10-30).

/** A magnifying glass: search. */
export function SearchIcon(): JSX.Element {
  return (
    <Icon className="listing-tools__icon">
      <path
        fillRule="evenodd"
        d="M10 2.5a7.5 7.5 0 1 0 0 15a7.5 7.5 0 1 0 0-15z M10 5a5 5 0 1 0 0 10a5 5 0 1 0 0-10z"
      />
      <path
        d="M15.6 15.6 20.6 20.6"
        fill="none"
        stroke="currentColor"
        strokeWidth="3.4"
        strokeLinecap="round"
      />
    </Icon>
  );
}

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

/** Two arrows, one up and one down: sort. */
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

/**
 * The magnifying glass, which asks for a search in a dialog of its own, in the middle of the screen
 * (the owner, 2026-10-30: back to a modal dialog, "a larger text box and a button beside it"; later
 * any other search criteria). Searching for nothing clears the search.
 */
export function SearchButton(props: {
  /** The search the page shows now, "" for none. */
  readonly query: string;
  readonly onSearch: (query: string) => void;
  /** The list a genre the search finds opens: Films, or Series from Series' own page. */
  readonly genreList: GenreList;
}): JSX.Element {
  const { t } = useTranslation();
  const [isOpen, setIsOpen] = useState(false);
  const close = (): void => {
    setIsOpen(false);
  };
  return (
    <>
      <IconButton
        label={t("browse.searchOpen")}
        icon={<SearchIcon />}
        isExpanded={isOpen}
        mark={props.query === "" ? undefined : "applied"}
        onClick={() => {
          setIsOpen(true);
        }}
      />
      {isOpen ? (
        <ModalDialog
          label={t("browse.searchOpen")}
          onClose={close}
          shouldFocusFirstField
          className="listing-search"
        >
          <SearchForm
            query={props.query}
            genreList={props.genreList}
            onSearch={(query) => {
              close();
              props.onSearch(query);
            }}
            onOpen={close}
          />
        </ModalDialog>
      ) : null}
    </>
  );
}

/**
 * The search's dialog: its heading on the line its Close is on, as every dialog's (Configure's), and
 * under it the field, Search beside it, and Show all, which drops every search criterion and lists
 * everything again (the owner, 2026-10-30). Search filters the page's list by what is typed; under
 * the field, at each letter, is what the whole server's search finds, in groups, and while it is
 * empty the device's recent searches (search-by-more §4.2, §4.5). A search submitted, or one a
 * result is opened from, is kept as the newest recent one.
 */
function SearchForm(props: {
  readonly query: string;
  readonly genreList: GenreList;
  readonly onSearch: (query: string) => void;
  /** Told as a result is opened, the page changing under the dialog. */
  readonly onOpen: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const [typed, setTyped] = useState(props.query);
  return (
    <>
      <h2 className="listing-search__heading">{t("browse.searchOpen")}</h2>
      <form
        role="search"
        className="listing-search__form"
        onSubmit={(event) => {
          event.preventDefault();
          keepRecentSearch(typed);
          props.onSearch(typed.trim());
        }}
      >
        <input
          type="search"
          aria-label={t("browse.searchLabel")}
          placeholder={t("browse.searchSubmit")}
          value={typed}
          onChange={(event) => {
            setTyped(event.target.value);
          }}
        />
        <button type="submit">{t("browse.searchSubmit")}</button>
      </form>
      <SearchFound
        typed={typed}
        genreList={props.genreList}
        onRecent={setTyped}
        onOpen={() => {
          keepRecentSearch(typed);
          props.onOpen();
        }}
      />
      <div className="listing-search__actions">
        <button
          type="button"
          onClick={() => {
            props.onSearch("");
          }}
        >
          {t("browse.showAll")}
        </button>
      </div>
    </>
  );
}
