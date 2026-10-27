// SPDX-License-Identifier: MPL-2.0

import "./library-browse.css";

import type { components } from "@orpheum/sdk";
import type { TFunction } from "i18next";
import { useCallback, useEffect, useId, useRef, useState, type JSX, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import {
  AlphabetRail,
  useAlphabetRailLetters,
  useLetterJump,
  useLetterJumpState,
  type LetterJump,
} from "./alphabet-rail.tsx";
import { PosterGrid, type Entry } from "./browse.tsx";
import { EndOfList } from "./end-of-list.tsx";
import { Link } from "./link.tsx";
import { Loaded } from "./loaded.tsx";
import { useItemsInView } from "./items-in-view.ts";
import { ModalDialog } from "./modal-dialog.tsx";
import type { Navigation } from "./navigation.ts";
import { useBackgroundChangesPoller } from "./polling.ts";
import { dataOrThrow } from "./problem.ts";
import type { LibrarySort, Route, SortOrder } from "./router.ts";
import { QueryResults } from "./search.tsx";
import { useServices } from "./services.tsx";
import {
  isByName,
  isSameChoice,
  orderOf,
  orderQuery,
  sortChoiceLabel,
  sortChoicesOf,
  type SortChoice,
} from "./sort-choices.ts";
import { usePages, type Page } from "./use-pages.ts";
import { useResource, type Resource } from "./use-resource.ts";

/** A library, as the server sends it. */
type LibraryBody = components["schemas"]["LibraryBody"];

/** A subfolder of an other library's folder, as the server sends it. */
type SubfolderBody = components["schemas"]["SubfolderBody"];

/** Which titles a list asks for: a library's own kind, or every kind for All libraries. */
type TitlesKind = "movie" | "series" | "other" | "any";

/** How many posters a page loads, as Films and Series do (browse.tsx). */
const entriesPerPage = 60;

/** The kind of titles a library's own page lists (D349 §9.2); every kind before its library is read. */
function titlesKindOf(kind: LibraryBody["kind"] | undefined): TitlesKind {
  if (kind === "movies") {
    return "movie";
  }
  if (kind === "shows") {
    return "series";
  }
  if (kind === "other") {
    return "other";
  }
  return "any";
}

/**
 * The sorts a kind offers, the first its default (D349 §9.1: an other library's by recorded date);
 * films and series by release date too, not every kind together (D366 §1).
 */
function sortOptionsFor(kind: TitlesKind): readonly [LibrarySort, ...LibrarySort[]] {
  switch (kind) {
    case "other":
      return ["recorded", "name", "file_name", "added"];
    case "movie":
    case "series":
      return ["name", "released", "added"];
    case "any":
      return ["name", "added"];
  }
}

/** The libraries the person reaches, in their locale's alphabetical order (D349 §9.1). */
function sortedLibraries(loaded: readonly LibraryBody[]): LibraryBody[] {
  return [...loaded].sort((first, second) => first.name.localeCompare(second.name));
}

/** What the page shows, read from its route (app.tsx), with what changed when a person chooses. */
interface BrowseState {
  readonly library: number | "all" | undefined;
  readonly view: "folders" | undefined;
  readonly folder: string | undefined;
  readonly sort: LibrarySort | undefined;
  /** The way the sort runs; undefined for its own (D366 §2). */
  readonly order?: SortOrder | undefined;
  readonly query: string;
}

/** The route for state, with overrides: a key given as undefined clears it, left out keeps it. */
function routeFor(state: BrowseState, overrides: Partial<BrowseState>): Route {
  const next = { ...state, ...overrides };
  return {
    screen: "browse",
    ...(next.library === undefined ? {} : { library: next.library }),
    ...(next.view === undefined ? {} : { view: next.view }),
    ...(next.folder === undefined ? {} : { folder: next.folder }),
    ...(next.sort === undefined ? {} : { sort: next.sort }),
    ...(next.order === undefined ? {} : { order: next.order }),
    ...(next.query === "" ? {} : { query: next.query }),
  };
}

/** What a library (or All libraries) chosen settles for the page: its kind, sort and view. */
interface ResolvedBrowse {
  readonly kind: TitlesKind;
  readonly sort: LibrarySort;
  /** The way the sort runs: the address's, or the sort's own. */
  readonly order: SortOrder;
  readonly sortOptions: readonly LibrarySort[];
  readonly showsFolders: boolean;
  readonly view: "all" | "folders";
  readonly libraryId: number | undefined;
  readonly currentLabel: string;
}

/** Settles what the chosen library (found in sorted, or "all") means for the page (D349 §9.1, §9.2). */
function chosenLibraryOf(
  state: BrowseState,
  sorted: readonly LibraryBody[],
): LibraryBody | undefined {
  if (state.library === "all" || state.library === undefined) {
    return undefined;
  }
  return sorted.find((library) => library.id === state.library);
}

/** The sort to list in: the one chosen, when the resolved kind offers it, else its default. */
function resolvedSortOf(
  state: BrowseState,
  sortOptions: readonly [LibrarySort, ...LibrarySort[]],
): LibrarySort {
  return state.sort !== undefined && sortOptions.includes(state.sort) ? state.sort : sortOptions[0];
}

function resolveBrowse(
  state: BrowseState,
  sorted: readonly LibraryBody[],
  t: TFunction,
): ResolvedBrowse {
  const chosen = chosenLibraryOf(state, sorted);
  const kind: TitlesKind = state.library === "all" ? "any" : titlesKindOf(chosen?.kind);
  const sortOptions = sortOptionsFor(kind);
  const showsFolders = kind === "other";
  const sort = resolvedSortOf(state, sortOptions);
  return {
    kind,
    sort,
    order: orderOf(sort, sort === state.sort ? state.order : undefined),
    sortOptions,
    showsFolders,
    view: showsFolders && state.view === "folders" ? "folders" : "all",
    libraryId: state.library === "all" ? undefined : state.library,
    currentLabel:
      chosen?.name ?? (state.library === "all" ? t("browse.allLibraries") : t("browse.library")),
  };
}

/**
 * Picks the first library, by name, the moment the page is visited with none in its address (D349
 * §9.1 point 1); the address then holds it, so Back and a reload keep it. Nothing is asked for, or
 * shown, under a kind guessed before the library's own is known: the page waits for the libraries
 * to load (Loaded, in LibraryBrowse) before it resolves a kind, sort or view from any of them.
 */
function useAutoSelectFirstLibrary(
  state: BrowseState,
  libraries: Resource<readonly LibraryBody[]>,
  navigation: Navigation,
): void {
  useEffect(() => {
    if (state.library !== undefined || libraries.state !== "loaded") {
      return;
    }
    const first = sortedLibraries(libraries.value)[0];
    if (first !== undefined) {
      navigation.replace(routeFor(state, { library: first.id }));
    }
  }, [state, libraries, navigation]);
}

/**
 * The Libraries page (D349): a library chosen (or All libraries), its view, its sort and a search
 * field, in its address so Back and a reload return to where it was. A film library lists its
 * films, a series library its series, All libraries everything; an other library
 * lists its videos by recorded date, name, file name or newest added, with a view of All or
 * Folders; no genre.
 */
export function LibraryBrowse(props: BrowseState): JSX.Element {
  const { api, navigation } = useServices();
  const [libraries] = useResource(
    "browse-libraries",
    async () => dataOrThrow(await api.GET("/libraries")).libraries ?? [],
    useBackgroundChangesPoller(),
  );
  useAutoSelectFirstLibrary(props, libraries, navigation);
  return (
    <Loaded
      resource={libraries}
      render={(loaded) => <LibraryBrowseResolved state={props} libraries={loaded} />}
    />
  );
}

/** The page once the libraries the person reaches are known: sorted, or that there are none yet. */
function LibraryBrowseResolved(props: {
  readonly state: BrowseState;
  readonly libraries: readonly LibraryBody[];
}): JSX.Element {
  const { t } = useTranslation();
  const sorted = sortedLibraries(props.libraries);
  if (sorted.length === 0) {
    return (
      <section className="browse" aria-labelledby="browse-heading">
        <h1 id="browse-heading">{t("browse.libraries")}</h1>
        <p className="browse__empty">{t("browse.noLibrariesAtAll")}</p>
      </section>
    );
  }
  const resolved = resolveBrowse(props.state, sorted, t);
  return <LibraryBrowseLayout state={props.state} libraryOptions={sorted} resolved={resolved} />;
}

/** Clears a letter jump when key changes: a list remounted under another library or sort starts
 * with none, not the last letter pressed in another one (alphabet-rail.tsx's own concern, shared
 * by Films, Series and this page alike). */
function useResetJumpOnChange(key: string, clear: () => void): void {
  const lastKeyRef = useRef(key);
  useEffect(() => {
    if (lastKeyRef.current !== key) {
      lastKeyRef.current = key;
      clear();
    }
  }, [key, clear]);
}

/** The page once a library (or All libraries) is settled: its bar, its rail, and what it lists. */
function LibraryBrowseLayout(props: {
  readonly state: BrowseState;
  readonly libraryOptions: readonly LibraryBody[];
  readonly resolved: ResolvedBrowse;
}): JSX.Element {
  const { navigation } = useServices();
  const { state, resolved } = props;
  const [typedQuery, setTypedQuery] = useState(state.query);
  const [jump, onLetter, clearJump] = useLetterJumpState();
  const listRef = useRef<HTMLDivElement>(null);
  useResetJumpOnChange(`${String(state.library)}/${resolved.sort}/${resolved.order}`, clearJump);
  const showsRail = state.query === "" && resolved.view === "all" && isByName(resolved.sort);
  return (
    <section className="browse browse__layout" aria-labelledby="browse-heading">
      <div ref={listRef}>
        <LibraryBrowseBar
          state={state}
          libraryOptions={props.libraryOptions}
          resolved={resolved}
          typedQuery={typedQuery}
          onTypedQueryChange={setTypedQuery}
          onSubmitSearch={() => {
            navigation.go(routeFor(state, { query: typedQuery.trim() }));
          }}
        />
        <LibraryBrowseBody
          state={state}
          resolved={resolved}
          jump={showsRail ? jump : undefined}
          listRef={listRef}
        />
      </div>
      {showsRail ? (
        <LibraryLetters
          kind={resolved.kind}
          library={resolved.libraryId}
          sort={resolved.sort === "file_name" ? "file_name" : "name"}
          isZToA={resolved.order === "descending"}
          onLetter={onLetter}
        />
      ) : null}
    </section>
  );
}

/** One choice among several, each a route: the library chooser's, the view's and the sort's. */
interface Choice {
  readonly key: string;
  readonly label: string;
  readonly to: Route;
  readonly isCurrent: boolean;
}

/** The library chooser's choices: every one the person reaches, by name, then All libraries. */
function libraryChoicesOf(
  state: BrowseState,
  libraries: readonly LibraryBody[],
  t: TFunction,
): Choice[] {
  return [
    ...libraries.map((library) => ({
      key: String(library.id),
      label: library.name,
      to: routeFor(state, {
        library: library.id,
        view: undefined,
        folder: undefined,
        sort: undefined,
        order: undefined,
      }),
      isCurrent: state.library === library.id,
    })),
    {
      key: "all",
      label: t("browse.allLibraries"),
      to: routeFor(state, {
        library: "all",
        view: undefined,
        folder: undefined,
        sort: undefined,
        order: undefined,
      }),
      isCurrent: state.library === "all",
    },
  ];
}

/**
 * The sort chooser's choices: each sort the resolved library (or All libraries) offers, its own way
 * and the other (D366 §2); the address names the way only when it is not the sort's own.
 */
function sortChoicesOfPage(state: BrowseState, resolved: ResolvedBrowse, t: TFunction): Choice[] {
  return sortChoicesOf(resolved.sortOptions).map((choice) => ({
    key: `${choice.sort}/${choice.order}`,
    label: sortChoiceLabel(t, choice),
    to: routeFor(state, { sort: choice.sort, order: orderQuery(choice).order }),
    isCurrent: isSameChoice(choice, resolved),
  }));
}

/** The view chooser's choices: All or Folders, an other library's alone (D349 §9.1). */
function viewChoicesOf(state: BrowseState, resolved: ResolvedBrowse, t: TFunction): Choice[] {
  return [
    {
      key: "all",
      label: t("browse.viewAll"),
      to: routeFor(state, { view: undefined, folder: undefined }),
      isCurrent: resolved.view === "all",
    },
    {
      key: "folders",
      label: t("browse.viewFolders"),
      to: routeFor(state, { view: "folders", folder: undefined }),
      isCurrent: resolved.view === "folders",
    },
  ];
}

/** The page's top row: the library chooser, the view it offers, the sort, and a search field. */
function LibraryBrowseBar(props: {
  readonly state: BrowseState;
  readonly libraryOptions: readonly LibraryBody[];
  readonly resolved: ResolvedBrowse;
  readonly typedQuery: string;
  readonly onTypedQueryChange: (query: string) => void;
  readonly onSubmitSearch: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { state, resolved } = props;
  return (
    <div className="browse__bar" data-focus-container="">
      <h1 id="browse-heading">{t("browse.libraries")}</h1>
      <RouteChooser
        fieldLabel={t("browse.library")}
        isFieldLabelShown={false}
        current={resolved.currentLabel}
        options={libraryChoicesOf(state, props.libraryOptions, t)}
      />
      {resolved.showsFolders ? (
        <RouteChooser
          fieldLabel={t("browse.view")}
          current={resolved.view === "folders" ? t("browse.viewFolders") : t("browse.viewAll")}
          options={viewChoicesOf(state, resolved, t)}
        />
      ) : null}
      <RouteChooser
        fieldLabel={t("browse.sort")}
        current={sortChoiceLabel(t, resolved)}
        options={sortChoicesOfPage(state, resolved, t)}
      />
      <form
        role="search"
        onSubmit={(event) => {
          event.preventDefault();
          props.onSubmitSearch();
        }}
      >
        <label>
          <input
            type="search"
            aria-label={t("browse.searchLabel")}
            placeholder={t("browse.searchSubmit")}
            value={props.typedQuery}
            onChange={(event) => {
              props.onTypedQueryChange(event.target.value);
            }}
          />
        </label>
        <button type="submit">{t("browse.searchSubmit")}</button>
      </form>
    </div>
  );
}

/** What the page lists under its bar: search results, a folder, or the library's grid. */
function LibraryBrowseBody(props: {
  readonly state: BrowseState;
  readonly resolved: ResolvedBrowse;
  readonly jump: LetterJump | undefined;
  readonly listRef: RefObject<HTMLElement | null>;
}): JSX.Element {
  const { state, resolved } = props;
  if (state.query !== "") {
    return <QueryResults key={state.query} query={state.query} library={resolved.libraryId} />;
  }
  if (resolved.view === "folders" && resolved.libraryId !== undefined) {
    return (
      <FolderView
        key={`${String(resolved.libraryId)}/${resolved.sort}/${resolved.order}/${state.folder ?? ""}`}
        libraryId={resolved.libraryId}
        folder={state.folder}
        choice={resolved}
        folderRoute={(folder) => routeFor(state, { folder })}
      />
    );
  }
  return (
    <LibraryGrid
      key={`${resolved.kind}/${String(resolved.libraryId)}/${resolved.sort}/${resolved.order}`}
      kind={resolved.kind}
      library={resolved.libraryId}
      choice={resolved}
      jump={props.jump}
      listRef={props.listRef}
    />
  );
}

/**
 * One choice among several, each a route: a button that opens a focusable drop-down of options
 * (modal-dialog.tsx: "the sort's select will need the same treatment" as a native select does not
 * work reliably on a television's browser, library-browsing.md §5.5), one list for the library, the
 * view and the sort alike.
 */
function RouteChooser(props: {
  readonly fieldLabel: string;
  /**
   * Whether the field's name shows beside it; a screen reader hears it either way. The library
   * chooser's does not: what it chooses is plain from its choices (the owner, 2026-10-27).
   */
  readonly isFieldLabelShown?: boolean;
  readonly current: string;
  readonly options: readonly Choice[];
}): JSX.Element {
  const [isOpen, setIsOpen] = useState(false);
  const fieldId = useId();
  const close = useCallback(() => {
    setIsOpen(false);
  }, []);
  return (
    <div className="library-browse__chooser">
      {props.isFieldLabelShown === false ? null : (
        <span className="library-browse__chooser-field" id={fieldId}>
          {props.fieldLabel}
        </span>
      )}
      <button
        type="button"
        className="library-browse__chooser-button"
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
          className="modal-dialog--drop-down library-browse__chooser-panel"
          label={props.fieldLabel}
          onClose={close}
          shouldCloseOnOutsidePress
        >
          <ul>
            {props.options.map((option) => (
              <li key={option.key}>
                <Link to={option.to} isCurrent={option.isCurrent} onNavigate={close}>
                  {option.label}
                </Link>
              </li>
            ))}
          </ul>
        </ModalDialog>
      ) : null}
    </div>
  );
}

/**
 * The alphabet rail's letters for a library's list, only those some title in it begins with (D349
 * §9.1), through alphabet-rail.tsx's one mechanism, shared with Films and Series.
 */
function LibraryLetters(props: {
  readonly kind: TitlesKind;
  readonly library: number | undefined;
  readonly sort: "name" | "file_name";
  readonly isZToA: boolean;
  readonly onLetter: (letter: string) => void;
}): JSX.Element {
  const letters = useAlphabetRailLetters(props.kind, props.library, props.sort);
  return <AlphabetRail onLetter={props.onLetter} letters={letters} isZToA={props.isZToA} />;
}

/** An entry's id, by which a list shows an entry read twice once. */
const entryId = (entry: Entry): number => entry.id;

/**
 * One library's (or All libraries') list, in one order, read a page at a time, moved to a letter
 * as the alphabet rail asks (D349 §9.1). kind and library say what GET /titles asks for; All
 * libraries' grid reads each entry's own kind (browse.tsx's PosterGrid).
 */
function LibraryGrid(props: {
  readonly kind: TitlesKind;
  readonly library: number | undefined;
  readonly choice: SortChoice;
  readonly jump: LetterJump | undefined;
  readonly listRef: RefObject<HTMLElement | null>;
}): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const { kind, library } = props;
  const { sort, order } = props.choice;
  const loadPage = useCallback(
    async (cursor: string): Promise<Page<Entry>> => {
      const query = {
        kind,
        sort,
        ...orderQuery({ sort, order }),
        limit: entriesPerPage,
        ...(library === undefined ? {} : { library }),
        ...(cursor === "" ? {} : { cursor }),
      };
      const titlesPage = dataOrThrow(await api.GET("/titles", { params: { query } }));
      return { items: titlesPage.titles ?? [], nextCursor: titlesPage.nextCursor ?? "" };
    },
    [api, kind, library, sort, order],
  );
  const pages = usePages(loadPage, useBackgroundChangesPoller(), entryId);
  const watchItemAt = useItemsInView(pages.showRange);
  useLetterJump({
    kind,
    sort: sort === "file_name" ? "file_name" : "name",
    isZToA: order === "descending",
    library,
    jump: isByName(sort) ? props.jump : undefined,
    pages,
    listRef: props.listRef,
  });
  return (
    <Loaded
      resource={pages.loadedSoFar}
      render={(page) =>
        page.items.length === 0 ? (
          <p className="browse__empty">{t("browse.empty")}</p>
        ) : (
          <>
            <PosterGrid entries={page.items} watchItemAt={watchItemAt} />
            {page.nextCursor === "" ? null : (
              <EndOfList onMore={pages.loadMore} isLoadingMore={pages.isLoadingMore} />
            )}
          </>
        )
      }
    />
  );
}

/** A folder's path with its last segment dropped, undefined at the library's top. */
function parentFolderOf(folder: string): string | undefined {
  const parts = folder.split("/").filter((part) => part !== "");
  parts.pop();
  return parts.length === 0 ? undefined : parts.join("/");
}

/** A folder's subfolders, each leading into its own (D349 §9.1): its name and how many videos it
 * holds, in the folders inside it too. */
function Subfolders(props: {
  readonly subfolders: readonly SubfolderBody[];
  readonly folderRoute: (folder: string | undefined) => Route;
}): JSX.Element {
  return (
    <ul className="library-browse__subfolders" data-focus-container="">
      {props.subfolders.map((subfolder) => (
        <li key={subfolder.path}>
          <Link to={props.folderRoute(subfolder.path)}>
            <span className="library-browse__subfolder-name">{subfolder.name}</span>
            <span className="library-browse__subfolder-count">{subfolder.videoCount}</span>
          </Link>
        </li>
      ))}
    </ul>
  );
}

/**
 * One folder of an other library (D349 §9.1, §9.2): the folders in it, each leading into its own,
 * and its own videos, in the sort the page chose. Only a view (library-browsing.md §4.1): no
 * pagination, as the scanner's folders are read whole.
 */
function FolderView(props: {
  readonly libraryId: number;
  readonly folder: string | undefined;
  readonly choice: SortChoice;
  readonly folderRoute: (folder: string | undefined) => Route;
}): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const { libraryId, folder, choice } = props;
  // An other library's folder has no release date to sort by; its sorts never offer one.
  const sort = choice.sort === "released" ? "recorded" : choice.sort;
  const [result] = useResource(
    `browse-folder/${String(libraryId)}/${folder ?? ""}/${sort}/${choice.order}`,
    async () =>
      dataOrThrow(
        await api.GET("/libraries/{id}/folders", {
          params: {
            path: { id: libraryId },
            query: {
              sort,
              ...orderQuery(choice),
              ...(folder === undefined ? {} : { folder }),
            },
          },
        }),
      ),
    useBackgroundChangesPoller(),
  );
  return (
    <Loaded
      resource={result}
      render={(data) => {
        const subfolders: readonly SubfolderBody[] = data.subfolders ?? [];
        const titles: readonly Entry[] = data.titles ?? [];
        const isEmpty = subfolders.length === 0 && titles.length === 0;
        return (
          <>
            {data.folder === "" ? null : (
              <p className="library-browse__folder-up">
                <Link to={props.folderRoute(parentFolderOf(data.folder))}>
                  {t("browse.folderUp")}
                </Link>
              </p>
            )}
            {isEmpty ? <p className="browse__empty">{t("browse.folderEmpty")}</p> : null}
            {subfolders.length === 0 ? null : (
              <Subfolders subfolders={subfolders} folderRoute={props.folderRoute} />
            )}
            {titles.length === 0 ? null : <PosterGrid entries={titles} />}
          </>
        );
      }}
    />
  );
}
