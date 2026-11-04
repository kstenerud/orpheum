// SPDX-License-Identifier: MPL-2.0

import "./library-browse.css";

import type { components } from "@orpheum/sdk";
import type { TFunction } from "i18next";
import { useCallback, useEffect, useRef, type JSX, type RefObject } from "react";
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
import { useListLoadsToFocusNoted } from "./input-layer.tsx";
import { useItemsInView } from "./items-in-view.ts";
import type { Navigation } from "./navigation.ts";
import { useBackgroundChangesPoller } from "./polling.ts";
import { UpTrail } from "./up-trail.tsx";
import type { Upward } from "./upward.ts";
import { dataOrThrow } from "./problem.ts";
import { itemRoute, routeHref, type LibrarySort, type Route, type SortOrder } from "./router.ts";
import { LibraryConfigure } from "./library-configure.tsx";
import { useLibrariesReached, sortedLibraries, type LibraryBody } from "./libraries-reached.ts";
import { RouteChooser, type Choice } from "./route-chooser.tsx";
import { ListingTools, SearchButton, SortArrowIcon, SortIcon } from "./listing-tools.tsx";
import { useServices } from "./services.tsx";
import {
  isByName,
  isSameChoice,
  orderOf,
  orderQuery,
  sortChoiceLabel,
  sortChoicesOf,
  sortFieldLabel,
  type SortChoice,
} from "./sort-choices.ts";
import { usePages, type Page } from "./use-pages.ts";
import { useResource, type Resource } from "./use-resource.ts";

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
  const { navigation } = useServices();
  const libraries = useLibrariesReached();
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
  const [jump, onLetter, clearJump] = useLetterJumpState();
  const listRef = useRef<HTMLDivElement>(null);
  useResetJumpOnChange(`${String(state.library)}/${resolved.sort}/${resolved.order}`, clearJump);
  const showsRail = (state.query !== "" || resolved.view === "all") && isByName(resolved.sort);
  return (
    <section className="browse browse__layout" aria-labelledby="browse-heading">
      <div ref={listRef}>
        <LibraryBrowseBar
          state={state}
          libraryOptions={props.libraryOptions}
          resolved={resolved}
          onSearch={(query) => {
            navigation.replace(routeFor(state, { query }));
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
          search={state.query}
          library={resolved.libraryId}
          sort={resolved.sort === "file_name" ? "file_name" : "name"}
          isZToA={resolved.order === "descending"}
          onLetter={onLetter}
        />
      ) : null}
    </section>
  );
}

/**
 * The sort chooser's choices: each sort the resolved library (or All libraries) offers, its own way
 * and the other (D366 §2); the address names the way only when it is not the sort's own.
 */
function sortChoicesOfPage(state: BrowseState, resolved: ResolvedBrowse, t: TFunction): Choice[] {
  return sortChoicesOf(resolved.sortOptions).map((choice) => ({
    key: `${choice.sort}/${choice.order}`,
    label: sortFieldLabel(t, choice.sort),
    mark: <SortArrowIcon isAscending={choice.order === "ascending"} />,
    spoken: sortChoiceLabel(t, choice),
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

/**
 * The page's top row: the library's name at the left, and at the right (the owner, 2026-10-30), read
 * right to left, its sort, its search and, on one library's page, its configure, with its view
 * before them where it offers one. Another library is the banner's Libraries menu's to choose
 * (app-shell.tsx; the owner, 2026-10-28), so the page has no library chooser of its own.
 */
function LibraryBrowseBar(props: {
  readonly state: BrowseState;
  readonly libraryOptions: readonly LibraryBody[];
  readonly resolved: ResolvedBrowse;
  readonly onSearch: (query: string) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { state, resolved } = props;
  const chosen = props.libraryOptions.find((library) => library.id === resolved.libraryId);
  return (
    <div className="browse__bar" data-focus-container="">
      <h1 id="browse-heading">{resolved.currentLabel}</h1>
      <ListingTools>
        {resolved.showsFolders ? (
          <RouteChooser
            fieldLabel={t("browse.view")}
            current={resolved.view === "folders" ? t("browse.viewFolders") : t("browse.viewAll")}
            options={viewChoicesOf(state, resolved, t)}
            isAView
          />
        ) : null}
        {chosen === undefined ? null : <LibraryConfigure library={chosen} />}
        <SearchButton
          query={state.query}
          onSearch={props.onSearch}
          genreList={resolved.kind === "series" ? "series" : "films"}
        />
        <RouteChooser
          fieldLabel={t("browse.sort")}
          openerIcon={<SortIcon />}
          current={sortChoiceLabel(t, resolved)}
          options={sortChoicesOfPage(state, resolved, t)}
          isAView
        />
      </ListingTools>
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
  if (resolved.view === "folders" && state.query === "" && resolved.libraryId !== undefined) {
    return (
      <>
        <UpTrail
          upward={folderUpward(
            { id: resolved.libraryId, name: resolved.currentLabel },
            state.folder,
            (folder) => routeFor(state, { folder }),
          )}
        />
        <FolderView
          key={`${String(resolved.libraryId)}/${resolved.sort}/${resolved.order}/${state.folder ?? ""}`}
          libraryId={resolved.libraryId}
          folder={state.folder}
          choice={resolved}
          folderRoute={(folder) => routeFor(state, { folder })}
        />
      </>
    );
  }
  return (
    <LibraryGrid
      key={`${resolved.kind}/${String(resolved.libraryId)}/${resolved.sort}/${resolved.order}/${state.query}`}
      kind={resolved.kind}
      library={resolved.libraryId}
      search={state.query}
      choice={resolved}
      jump={props.jump}
      listRef={props.listRef}
    />
  );
}

/**
 * The alphabet rail's letters for a library's list, only those some title in it begins with (D349
 * §9.1), through alphabet-rail.tsx's one mechanism, shared with Films and Series.
 */
function LibraryLetters(props: {
  readonly kind: TitlesKind;
  readonly search: string;
  readonly library: number | undefined;
  readonly sort: "name" | "file_name";
  readonly isZToA: boolean;
  readonly onLetter: (letter: string) => void;
}): JSX.Element {
  const letters = useAlphabetRailLetters(props);
  return <AlphabetRail onLetter={props.onLetter} letters={letters} isZToA={props.isZToA} />;
}

/** An entry's id, by which a list shows an entry read twice once. */
const entryId = (entry: Entry): number => entry.id;

/** A library list's page as GET /titles is asked for it: its criteria, its search one of them. */
function titlesQuery(
  list: {
    readonly kind: TitlesKind;
    readonly library: number | undefined;
    readonly sort: LibrarySort;
    readonly order: SortOrder;
    readonly search: string;
  },
  cursor: string,
) {
  const { kind, library, sort, order, search } = list;
  return {
    kind,
    sort,
    ...orderQuery({ sort, order }),
    limit: entriesPerPage,
    ...(library === undefined ? {} : { library }),
    ...(search === "" ? {} : { query: search }),
    ...(cursor === "" ? {} : { cursor }),
  };
}

/** The address an entry's poster leads to, by the entry's own kind, as PosterGrid draws it. */
const entryHref = (entry: Entry): string => routeHref(itemRoute(entry.kind, entry.id));

/**
 * One library's (or All libraries') list, in one order, read a page at a time, moved to a letter
 * as the alphabet rail asks (D349 §9.1). kind and library say what GET /titles asks for; All
 * libraries' grid reads each entry's own kind (browse.tsx's PosterGrid).
 */
function LibraryGrid(props: {
  readonly kind: TitlesKind;
  readonly library: number | undefined;
  /** The list's search, "" for none (GET /titles' query). */
  readonly search: string;
  readonly choice: SortChoice;
  readonly jump: LetterJump | undefined;
  readonly listRef: RefObject<HTMLElement | null>;
}): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const { kind, library, search } = props;
  const { sort, order } = props.choice;
  const loadPage = useCallback(
    async (cursor: string): Promise<Page<Entry>> => {
      const query = titlesQuery({ kind, library, sort, order, search }, cursor);
      const titlesPage = dataOrThrow(await api.GET("/titles", { params: { query } }));
      return { items: titlesPage.titles ?? [], nextCursor: titlesPage.nextCursor ?? "" };
    },
    [api, kind, library, sort, order, search],
  );
  const pages = usePages(loadPage, useBackgroundChangesPoller(), entryId);
  const watchItemAt = useItemsInView(pages.showRange);
  useListLoadsToFocusNoted(pages, entryHref);
  useLetterJump({
    kind,
    sort: sort === "file_name" ? "file_name" : "name",
    isZToA: order === "descending",
    library,
    search,
    jump: isByName(sort) ? props.jump : undefined,
    pages,
    listRef: props.listRef,
  });
  return (
    <Loaded
      resource={pages.loadedSoFar}
      render={(page) =>
        page.items.length === 0 ? (
          <p className="browse__empty">
            {search === "" ? t("browse.empty") : t("search.none", { query: search })}
          </p>
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

/**
 * Where an other library's folder sits (D367 §2): the library's folders at its top, then each folder
 * above it, the one directly above its parent; the folder's own name last. The library's top is a
 * top page.
 */
export function folderUpward(
  library: { readonly id: number; readonly name: string },
  folder: string | undefined,
  folderRoute: (folder: string | undefined) => Route,
): Upward {
  const { id: libraryId, name: libraryName } = library;
  const parts = (folder ?? "").split("/").filter((part) => part !== "");
  const here = parts.at(-1);
  if (here === undefined) {
    return { crumbs: [], here: libraryName };
  }
  const isFolder =
    (path: string | undefined) =>
    (route: Route): boolean =>
      route.screen === "browse" &&
      route.library === libraryId &&
      route.view === "folders" &&
      route.folder === path;
  const above = parts.slice(0, -1).map((name, index) => {
    const path = parts.slice(0, index + 1).join("/");
    return { label: name, to: folderRoute(path), isIt: isFolder(path) };
  });
  return {
    crumbs: [
      { label: libraryName, to: folderRoute(undefined), isIt: isFolder(undefined) },
      ...above,
    ],
    here,
  };
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
