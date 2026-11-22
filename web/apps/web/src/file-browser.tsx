// SPDX-License-Identifier: MPL-2.0

import "./file-browser.css";

import { allPages, type Api } from "@orpheum/sdk";
import {
  useEffect,
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type JSX,
  type MouseEvent,
  type ReactNode,
  type RefObject,
} from "react";
import { useTranslation } from "react-i18next";

import { FolderIcon, MachineIcon, VideoFileIcon, WatchedIcon } from "./icons.tsx";
import { MoreActions, type MoreAction } from "./more-actions.tsx";
import { dataOrThrow } from "./problem.ts";
import { StatusNote, type StatusNoteTold } from "./status-note.tsx";

/** A place a library folder can be: a node, and a path on it. */
export interface Place {
  readonly nodeName: string;
  readonly path: string;
}

/** The folder above path, as far as start, where browsing began: none above start. */
export function folderAbove(path: string, start: string): string | undefined {
  if (path === start) {
    return undefined;
  }
  const cut = path.lastIndexOf("/");
  return cut <= 0 ? "/" : path.slice(0, cut);
}

/** The folder name inside path. */
export function folderInside(path: string, name: string): string {
  return path === "/" ? `/${name}` : `${path}/${name}`;
}

/** A path's last name: the folder it names, or the path itself at a disk's top. */
export function folderName(path: string): string {
  return path.split("/").findLast((part) => part !== "") ?? path;
}

/** The folders in a folder on a node, every page of them, with the videos each holds where counted. */
export interface FolderListing {
  readonly path: string;
  readonly state: "readable" | "notAnswering" | "missing" | "failed";
  readonly folders: readonly string[];
  /**
   * How many videos each folder holds, in it and below it, by its name (D539 §7); a folder the
   * node did not count in time is absent, its count unknown, never 0.
   */
  readonly videoCounts: Readonly<Record<string, number>>;
  readonly failure?: string;
}

/**
 * Asks the server for the folders in a folder on a node (D359 §2): every page of them, the first
 * page's state and path with them all (D432 §3), and every page's counts (D539 §7).
 */
export async function listFolders(api: Api, place: Place): Promise<FolderListing> {
  let first: Omit<FolderListing, "folders" | "videoCounts"> | undefined;
  const counts: Record<string, number> = {};
  const folders = await allPages(async (query) => {
    const page = dataOrThrow(
      await api.GET("/nodes/{name}/folders", {
        params: { path: { name: place.nodeName }, query: { path: place.path, ...query } },
      }),
    );
    first ??= {
      path: page.path,
      state: page.state,
      ...(page.failure === undefined ? {} : { failure: page.failure }),
    };
    Object.assign(counts, page.videoCounts ?? {});
    return { entries: page.folders ?? [], nextCursor: page.nextCursor };
  });
  if (first === undefined) {
    throw new Error("the server answered no page of folders");
  }
  return { ...first, folders, videoCounts: counts };
}

/** Names in natural order, ignoring case: "Season 2" before "Season 10" (D539 §1). */
const naturalCollator = new Intl.Collator(undefined, { numeric: true, sensitivity: "base" });

/** entries in natural order of their names (D539 §1), a copy. */
export function inNaturalOrder<T>(entries: readonly T[], nameOf: (entry: T) => string): T[] {
  return entries.toSorted((one, other) => naturalCollator.compare(nameOf(one), nameOf(other)));
}

/** One row of a file browser: a machine's place, a folder or a video file (D539 §1). */
export interface FileRow {
  /** Its key among the rows of its folder: a folder's name, a video's file id. */
  readonly key: string;
  readonly kind: "place" | "folder" | "video";
  /** Its whole name, which is its name for a screen reader too. */
  readonly name: string;
  /** The line under its name: a place's path, a video's title and year. */
  readonly under?: string | undefined;
  /** What it says at its end: "312 videos", a video's length. */
  readonly details?: string | undefined;
  /** How far into it its person is, in percent, while they have started it and not finished. */
  readonly percent?: number | undefined;
  /** Whether its person watched it, or a folder's every video. */
  readonly isWatched?: boolean | undefined;
  readonly onOpen: () => void;
  /** Its More menu's actions (Browse alone); none for no ⋯ button. */
  readonly more?: readonly MoreAction[] | undefined;
}

/** A group of rows: Choose's places of one machine, under its name; else the folder's rows alone. */
export interface FileGroup {
  readonly key: string;
  readonly heading?: string | undefined;
  readonly rows: readonly FileRow[];
  /** What it says in place of its rows: a machine that is not answering. */
  readonly told?: StatusNoteTold | undefined;
}

/** One step of the path bar: the place first, the folder it is in last. */
export interface Crumb {
  readonly key: string;
  readonly name: string;
  readonly onOpen: () => void;
}

/** What a file browser shows under its path bar: rows while they load, the rows, or a state. */
export type FileBrowserShown =
  | { readonly kind: "loading" }
  | {
      readonly kind: "listed";
      readonly groups: readonly FileGroup[];
      /** What it says where there are no rows: "No folders in Films". */
      readonly empty: StatusNoteTold;
    }
  | { readonly kind: "told"; readonly told: StatusNoteTold };

/** How long focus rests on a row before the side panel shows it (D539 §5). */
export const sidePanelRestMs = 150;

/** How many skeleton rows stand in for a folder while it loads. */
const skeletonRows = 8;

/**
 * The file browser (D539): a path bar over a list of rows, folders first, then videos, each in
 * natural order. Two uses compose it: Choose, a dialog that picks a folder on a machine
 * (file-browser-choose.tsx), and Browse, a library's folders and videos (library-folders.tsx), with
 * a side panel beside the list on a wide screen telling the row focus rested on. A state (loading,
 * empty, a machine not answering, a folder gone) takes the list's place, the path bar staying, so
 * there is always a way up. Opening a folder focuses its first row, and going up the folder it came
 * from (arrivalKey), where focus was in it or nowhere.
 */
export function FileBrowser(props: {
  readonly use: "choose" | "browse";
  /** The place, then each folder down to the one shown, which is last. */
  readonly crumbs: readonly Crumb[];
  /** The list's name for a screen reader. */
  readonly listLabel: string;
  readonly shown: FileBrowserShown;
  /** The row focus arrives on: the folder it came up from; else the first. */
  readonly arrivalKey?: string | undefined;
  /** What the path bar ends with: Choose's Go to…. */
  readonly barEnd?: ReactNode;
  /** In place of the crumbs: Go to…'s field while it is open. */
  readonly barInstead?: ReactNode;
  /** The side panel's content for the row focus rested on (Browse, wide); none for no panel. */
  readonly side?: ((row: FileRow | undefined) => ReactNode) | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  const rootRef = useRef<HTMLDivElement>(null);
  const [rested, setRested] = useState<FileRow | undefined>(undefined);
  const where = props.crumbs.map((crumb) => crumb.key).join("/");
  useArrival(rootRef, `${where}/${props.shown.kind}`, props.arrivalKey);
  const onRest = useRest(setRested);
  const rows = props.shown.kind === "listed" ? props.shown.groups.flatMap((g) => g.rows) : [];
  return (
    <div
      ref={rootRef}
      className={`file-browser file-browser--${props.use}${props.side === undefined ? "" : " file-browser--with-side"}`}
    >
      <div className="file-browser__bar">
        {props.barInstead ?? <PathBar crumbs={props.crumbs} />}
        {props.barInstead === undefined ? props.barEnd : null}
      </div>
      <div className="file-browser__body">
        <div className="file-browser__shown">
          <Shown shown={props.shown} listLabel={props.listLabel} onRest={onRest} />
        </div>
        {props.side === undefined ? null : (
          <aside className="file-browser__side" aria-hidden="true">
            {props.side(rows.find((row) => row.key === rested?.key))}
          </aside>
        )}
      </div>
      {props.shown.kind === "listed" ? (
        <output className="file-browser__told" aria-live="polite">
          {t("fileBrowser.opened", {
            folder: props.crumbs.at(-1)?.name ?? "",
            count: rows.length,
          })}
        </output>
      ) : null}
    </div>
  );
}

/**
 * Focuses a folder's arrival row once its rows are drawn, each time what is shown changes: the row
 * with arrivalKey, else the first, else a state's first button, else (a folder with nothing in it
 * and nothing to press) the dialog's main button or the path bar's first, so focus never lands on
 * nothing (D539 §4); only where focus is inside the browser, in the dialog around it, or nowhere,
 * so it never takes focus from elsewhere on a page.
 */
function useArrival(
  rootRef: RefObject<HTMLDivElement | null>,
  shownKey: string,
  arrivalKey: string | undefined,
): void {
  const arrivalRef = useRef(arrivalKey);
  useLayoutEffect(() => {
    arrivalRef.current = arrivalKey;
  });
  useLayoutEffect(() => {
    const root = rootRef.current;
    if (root !== null && isFocusOurs(root)) {
      arrivalIn(root, arrivalRef.current)?.focus();
    }
  }, [rootRef, shownKey]);
}

/** Whether focus is inside the browser, in the dialog around it, or nowhere. */
function isFocusOurs(root: HTMLElement): boolean {
  const focused = document.activeElement;
  const around = root.closest("dialog") ?? root;
  return (
    focused === null ||
    focused === document.body ||
    around.contains(focused) ||
    !focused.isConnected
  );
}

/** Where focus arrives in the browser at root: see useArrival. */
function arrivalIn(root: HTMLElement, arrivalKey: string | undefined): HTMLElement | null {
  const around = root.closest("dialog") ?? root;
  const rows = [...root.querySelectorAll<HTMLElement>(".file-browser__row")];
  return (
    rows.find((row) => row.dataset["rowKey"] === arrivalKey) ??
    rows[0] ??
    root.querySelector<HTMLElement>(".file-browser__shown button") ??
    around.querySelector<HTMLElement>("button[type='submit']") ??
    root.querySelector<HTMLElement>(".file-browser__bar button")
  );
}

/** A row focus rested on for sidePanelRestMs, told to onRested; a move before then starts again. */
function useRest(onRested: (row: FileRow | undefined) => void): (row: FileRow) => void {
  const timerRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  useEffect(
    () => () => {
      clearTimeout(timerRef.current);
    },
    [],
  );
  return (row) => {
    clearTimeout(timerRef.current);
    timerRef.current = setTimeout(() => {
      onRested(row);
    }, sidePanelRestMs);
  };
}

/**
 * The path bar (D539 §1): on a wide browser its crumbs, each a button but the folder it is in; on
 * a narrow one a Back to the folder above, "‹ Films", and the folder's name. Both are drawn, the
 * browser's own width choosing one (file-browser.css).
 */
function PathBar(props: { readonly crumbs: readonly Crumb[] }): JSX.Element {
  const { t } = useTranslation();
  const here = props.crumbs.at(-1);
  const above = props.crumbs.at(-2);
  return (
    <>
      <nav className="file-browser__crumbs" aria-label={t("fileBrowser.path")}>
        <ol>
          {props.crumbs.map((crumb, index) =>
            index === props.crumbs.length - 1 ? (
              <li key={crumb.key} aria-current="page" className="file-browser__here">
                {crumb.name}
              </li>
            ) : (
              <li key={crumb.key}>
                <button type="button" className="file-browser__crumb" onClick={crumb.onOpen}>
                  {crumb.name}
                </button>
                <span className="file-browser__chevron" aria-hidden="true">
                  ›
                </span>
              </li>
            ),
          )}
        </ol>
      </nav>
      <div className="file-browser__narrow-bar">
        {above === undefined ? null : (
          <button
            type="button"
            className="file-browser__back"
            aria-label={t("fileBrowser.up")}
            onClick={above.onOpen}
          >
            <span aria-hidden="true">‹ {above.name}</span>
          </button>
        )}
        <span className="file-browser__narrow-here">{here?.name}</span>
      </div>
    </>
  );
}

/** What the browser shows under its bar: skeleton rows, its groups of rows, or a state. */
function Shown(props: {
  readonly shown: FileBrowserShown;
  readonly listLabel: string;
  readonly onRest: (row: FileRow) => void;
}): JSX.Element {
  const { shown } = props;
  if (shown.kind === "loading") {
    return (
      <ul className="file-browser__list file-browser__list--loading" aria-busy="true">
        {Array.from({ length: skeletonRows }, (_, index) => (
          <li key={index} className="file-browser__skeleton" />
        ))}
      </ul>
    );
  }
  if (shown.kind === "told") {
    return <StatusNote told={shown.told} />;
  }
  if (shown.groups.every((group) => group.rows.length === 0 && group.told === undefined)) {
    return <StatusNote told={shown.empty} />;
  }
  return (
    <div className="file-browser__groups" data-focus-container="">
      {shown.groups.map((group) => (
        <section
          key={group.key}
          className="file-browser__group"
          aria-label={group.heading ?? props.listLabel}
        >
          {group.heading === undefined ? null : (
            <h3 className="file-browser__group-heading">{group.heading}</h3>
          )}
          {group.told === undefined ? null : <StatusNote told={group.told} />}
          {group.rows.length === 0 ? null : (
            <ul className="file-browser__list">
              {group.rows.map((row) => (
                <Row key={row.key} row={row} onRest={props.onRest} />
              ))}
            </ul>
          )}
        </section>
      ))}
    </div>
  );
}

/** A row's icon by its kind: a machine for a place, a folder, a video file. */
function RowIcon(props: { readonly kind: FileRow["kind"] }): JSX.Element {
  if (props.kind === "place") {
    return <MachineIcon />;
  }
  return props.kind === "folder" ? <FolderIcon /> : <VideoFileIcon />;
}

/** Opens the ⋯ of a row a right click or a long press was on, where it has one. */
function openMoreOf(event: MouseEvent<HTMLButtonElement>): void {
  const more =
    event.currentTarget.parentElement?.querySelector<HTMLButtonElement>(".more-actions__button");
  if (more !== null && more !== undefined) {
    event.preventDefault();
    more.click();
  }
}

/** What a row says after its name: its details, how much of it is watched, or that it all is. */
function RowDetails(props: { readonly row: FileRow; readonly id: string }): JSX.Element {
  const { t } = useTranslation();
  const { row } = props;
  return (
    <span id={props.id} className="file-browser__details">
      {row.details === undefined ? null : <span>{row.details}</span>}
      {row.percent === undefined || row.isWatched === true ? null : (
        <span className="file-browser__progress">
          <progress value={row.percent} max={100} aria-hidden="true" />
          <span className="file-browser__said">
            {t("fileBrowser.percentWatched", { percent: row.percent })}
          </span>
        </span>
      )}
      {row.isWatched === true ? (
        <span className="file-browser__watched">
          <WatchedIcon />
          <span className="file-browser__said">
            {row.kind === "folder" ? t("fileBrowser.allWatched") : t("fileBrowser.watched")}
          </span>
        </span>
      ) : null}
    </span>
  );
}

/**
 * One row (D539 §1): one press, its whole name its name for a screen reader, what it says under and
 * after its name read after it; a video's ⋯ beside it, which a right click or a long press opens too.
 */
function Row(props: {
  readonly row: FileRow;
  readonly onRest: (row: FileRow) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { row } = props;
  const underId = useId();
  const aboutId = useId();
  const hasMore = row.more !== undefined && row.more.length > 0;
  return (
    <li className="file-browser__entry">
      <button
        type="button"
        className="file-browser__row"
        data-row-key={row.key}
        aria-label={row.name}
        aria-describedby={row.under === undefined ? aboutId : `${underId} ${aboutId}`}
        onClick={row.onOpen}
        onFocus={() => {
          props.onRest(row);
        }}
        onContextMenu={openMoreOf}
      >
        <RowIcon kind={row.kind} />
        <span className="file-browser__text">
          <span className="file-browser__name">{row.name}</span>
          {row.under === undefined ? null : (
            <span id={underId} className="file-browser__under">
              {row.under}
            </span>
          )}
        </span>
        <RowDetails row={row} id={aboutId} />
      </button>
      {hasMore ? (
        <MoreActions
          actions={row.more ?? []}
          label={t("fileBrowser.more", { name: row.name })}
          className="file-browser__more"
        />
      ) : null}
    </li>
  );
}
