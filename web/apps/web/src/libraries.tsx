// SPDX-License-Identifier: MPL-2.0

import "./libraries.css";

import type { Api, components } from "@orpheum/sdk";
import type { TFunction } from "i18next";
import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { arrivalMark } from "./focus-navigation.ts";
import { RootWarning } from "./root-warning.tsx";
import { useRequest } from "./use-request.ts";
import { idempotent } from "./idempotent.ts";
import { useBackgroundChangesPoller } from "./polling.ts";
import { dataOrThrow, problemText } from "./problem.ts";
import { filingName } from "./files.tsx";
import { Link } from "./link.tsx";
import { useServices } from "./services.tsx";
import { Loaded } from "./loaded.tsx";
import { useResource } from "./use-resource.ts";

/** A library, as an admin sees it. */
export type Library = components["schemas"]["LibraryBody"];

/** What a library holds, in a word or two, by its kind. */
const kindKeys = {
  film: "libraries.summaryMovies",
  series: "libraries.summaryShows",
  other: "libraries.summaryOther",
} as const;

/**
 * A library's rescans as they run (D357 §5): scanning first, then how many of the films and series
 * listed were matched again.
 */
export function RescanProgress(props: {
  readonly rescan: NonNullable<Library["rescan"]>;
}): JSX.Element {
  const { t } = useTranslation();
  const { rescan } = props;
  return (
    <output>
      {rescan.isScanning && rescan.itemCount === 0
        ? t("libraries.rescanScanning")
        : t("libraries.rescanMatching", { matched: rescan.matchedCount, count: rescan.itemCount })}
    </output>
  );
}

/** Whether a library is rescanned whole: one of films or of series, never other (D357 §1). */
export function isRescannable(library: Pick<Library, "kind">): boolean {
  return library.kind === "film" || library.kind === "series";
}

/** A library's files counted by filing, when the admin sees them. */
export function FilingCounts(props: { readonly library: Library }): JSX.Element | null {
  const { t } = useTranslation();
  const filingCounts = Object.entries(props.library.fileCountsByFiling ?? {});
  if (filingCounts.length === 0) {
    return null;
  }
  return (
    <p>
      {filingCounts
        .map(([filing, count]) => t("libraries.filing", { filing: filingName(t, filing), count }))
        .join(" · ")}
    </p>
  );
}

/** How a library notices changes, in a few words (D184). */
function changeDetectionText(t: TFunction, library: Library): string {
  switch (library.changeDetection) {
    case "watch":
      return t("libraries.summaryWatched");
    case "interval":
      return t("libraries.summaryInterval", { hours: library.checkEveryHours ?? 0 });
    case "manual":
      return t("libraries.summaryManual");
  }
}

/**
 * A library at a glance (D361 §1): what it holds, how many folders it reads, how it notices changes,
 * and whether a scan of it runs now.
 */
export function LibrarySummary(props: { readonly library: Library }): JSX.Element {
  const { t } = useTranslation();
  const { library } = props;
  const roots = library.roots ?? [];
  const parts = [
    t(kindKeys[library.kind]),
    t("libraries.summaryFolders", { count: roots.length }),
    changeDetectionText(t, library),
    ...(roots.some((root) => root.scan !== undefined) ? [t("libraries.summaryScanning")] : []),
  ];
  return <p className="libraries__summary">{parts.join(" · ")}</p>;
}

/**
 * One library in the list (D361 §1): its name, what it is at a glance, its folders each with the
 * warning sign of one with a problem, its files by filing, a rescan running, and the way to its own
 * page to manage it.
 */
export function LibraryRow(props: { readonly library: Library }): JSX.Element {
  const { t } = useTranslation();
  const { library } = props;
  const headingId = `library-${String(library.id)}`;
  return (
    <li className="libraries__library">
      <article aria-labelledby={headingId}>
        <h2 id={headingId}>{library.name}</h2>
        <LibrarySummary library={library} />
        <ul className="libraries__folders">
          {(library.roots ?? []).map((root) => (
            <li key={`${root.nodeName}:${root.rootPath}`}>
              <RootWarning root={root} />
              <code>{t("libraries.root", { node: root.nodeName, path: root.rootPath })}</code>
            </li>
          ))}
        </ul>
        <FilingCounts library={library} />
        {library.rescan === undefined ? null : <RescanProgress rescan={library.rescan} />}
        <p>
          <Link to={{ screen: "library", libraryId: library.id }} className="libraries__manage">
            {t("libraries.manage")}
          </Link>
        </p>
      </article>
    </li>
  );
}

/**
 * "Rescan everything": every library of films or of series scanned, then each film and series in it
 * matched again from its files, in the background (D357 §1); beside it, its full rescan, every file
 * read again first (D392 §1). Each says how many started.
 */
export function RescanEverything(props: { readonly onStarted: () => void }): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [note, setNote] = useState("");
  const rescanning = useRequest((cause) => problemText(t, cause));
  const rescan = (isFull: boolean): void => {
    setNote("");
    rescanning.runRequest(async (idempotencyKey) => {
      const answer = dataOrThrow(
        await api.POST("/libraries/rescan", {
          ...(isFull ? { params: { query: { full: true } } } : {}),
          ...idempotent(idempotencyKey),
        }),
      );
      setNote(t("libraries.rescanEverythingStarted", { count: answer.startedCount }));
      props.onStarted();
    });
  };
  return (
    <>
      <button
        type="button"
        title={t("libraries.rescanHelp")}
        disabled={rescanning.isBusy}
        onClick={() => {
          rescan(false);
        }}
      >
        {t("libraries.rescanEverything")}
      </button>{" "}
      <button
        type="button"
        title={t("libraries.fullRescanHelp")}
        disabled={rescanning.isBusy}
        onClick={() => {
          rescan(true);
        }}
      >
        {t("libraries.fullRescanEverything")}
      </button>
      {note === "" ? null : <output>{note}</output>}
      {rescanning.problem === undefined ? null : <output>{rescanning.problem}</output>}
    </>
  );
}

/** The libraries the person reaches; an admin sees each one's roots and files too. */
export async function listLibraries(api: Api): Promise<Library[]> {
  return dataOrThrow(await api.GET("/libraries")).libraries ?? [];
}

/**
 * The admin's Libraries page (D361 §1): creating a library, on a page of its own, and rescanning
 * every one (D357), then each library at a glance, each leading to its own page to manage it, kept
 * up to date as scans and rescans run.
 */
export function Libraries(): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [libraries, reload] = useResource(
    "libraries",
    () => listLibraries(api),
    useBackgroundChangesPoller(),
  );
  return (
    <section className="libraries" aria-labelledby="libraries-heading">
      <h1 id="libraries-heading">{t("libraries.heading")}</h1>
      <p className="libraries__actions">
        <Link to={{ screen: "new-library" }} className="libraries__create">
          {t("libraries.createHeading")}
        </Link>{" "}
        <RescanEverything onStarted={reload} />
      </p>
      <Loaded
        resource={libraries}
        render={(value) =>
          value.length === 0 ? (
            <p>{t("libraries.none")}</p>
          ) : (
            <ul className="libraries__list" {...arrivalMark}>
              {value.map((library) => (
                <LibraryRow key={library.id} library={library} />
              ))}
            </ul>
          )
        }
      />
    </section>
  );
}
