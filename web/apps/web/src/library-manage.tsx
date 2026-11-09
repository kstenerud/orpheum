// SPDX-License-Identifier: MPL-2.0

import type { Api, components } from "@orpheum/sdk";
import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { AddRoot } from "./add-root.tsx";
import { BusyGroup } from "./busy-group.tsx";
import { ConfirmButton, ConfirmDialog } from "./confirm-button.tsx";
import { arrivalMark } from "./focus-navigation.ts";
import { idempotent } from "./idempotent.ts";
import {
  FilingCounts,
  isRescannable,
  LibrarySummary,
  listLibraries,
  RescanProgress,
  type Library,
} from "./libraries.tsx";
import { Link } from "./link.tsx";
import { Loaded } from "./loaded.tsx";
import { useBackgroundChangesPoller } from "./polling.ts";
import { dataOrThrow, problemText, throwIfRefused } from "./problem.ts";
import { MoreActions, type MoreAction } from "./more-actions.tsx";
import { RepointDialog, type Place } from "./repoint-root.tsx";
import { RootWarning } from "./root-warning.tsx";
import { SaveNote, useSaveState, type SaveState } from "./save-note.tsx";
import { useServices } from "./services.tsx";
import { UpTrail } from "./up-trail.tsx";
import { adminLibrariesCrumb } from "./upward.ts";
import { useRequest } from "./use-request.ts";
import { useResource } from "./use-resource.ts";

type Root = components["schemas"]["RootBody"];

const outcomeKeys = {
  complete: "libraries.outcomeComplete",
  unreadable: "libraries.outcomeUnreadable",
  withheld: "libraries.outcomeWithheld",
} as const;

/** Why a node does not watch a root, as its status said (D272 §9). */
const watchCauseKeys = {
  networkFilesystem: "libraries.watchCauseNetworkFilesystem",
  watchLimit: "libraries.watchCauseWatchLimit",
  unavailable: "libraries.watchCauseUnavailable",
  unreadable: "libraries.watchCauseUnreadable",
} as const;

/**
 * An admin's answer about a root (D208): its withheld removals are real, the disk missing from
 * one of the mount points inside it is gone for good, or the root itself is, and is retired
 * (D231 §7).
 */
export type RootAnswer =
  | { readonly kind: "approveRemovals" }
  | { readonly kind: "diskGone"; readonly mountPath: string }
  | { readonly kind: "retire" };

/**
 * Where an answer goes and what it sends: removals and a retire name the root's own path; a disk
 * gone names the missing mount point's, one of those the root lists, which is what the server
 * looks up.
 */
export function rootAnswerRequest(
  root: Root,
  answer: RootAnswer,
): {
  readonly endpoint:
    | "/libraries/{id}/roots/removals"
    | "/libraries/{id}/roots/mount-gone"
    | "/libraries/{id}/roots/retire";
  readonly body: { readonly nodeName: string; readonly path: string };
} {
  switch (answer.kind) {
    case "approveRemovals":
      return {
        endpoint: "/libraries/{id}/roots/removals",
        body: { nodeName: root.nodeName, path: root.rootPath },
      };
    case "diskGone":
      return {
        endpoint: "/libraries/{id}/roots/mount-gone",
        body: { nodeName: root.nodeName, path: answer.mountPath },
      };
    case "retire":
      return {
        endpoint: "/libraries/{id}/roots/retire",
        body: { nodeName: root.nodeName, path: root.rootPath },
      };
  }
}

/**
 * One root: where it is, how its last scan went, the admin's answer when its removals wait, for
 * each mount point inside it whose disk is missing, the answer that it is gone (D208), and
 * retiring it for good, confirmed first (D231 §7).
 */
export function RootLine(props: {
  readonly root: Root;
  /** Whether its library is watched, so whether its node's watching it is worth saying. */
  readonly isWatched?: boolean;
  readonly onAnswer: (root: Root, answer: RootAnswer) => void;
  /** Where it can move to, and saying it did (D241 §6). */
  readonly places?: readonly Place[];
  readonly onRepoint?: (root: Root, to: Place) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { root } = props;
  return (
    <li className="library__folder">
      <p>
        <RootWarning root={root} />
        <code>{t("libraries.root", { node: root.nodeName, path: root.rootPath })}</code>{" "}
        {root.lastScanOutcome === "" ? null : <span>{t(outcomeKeys[root.lastScanOutcome])}</span>}
      </p>
      {root.scan === undefined ? null : <ScanProgress scan={root.scan} />}
      {props.isWatched === true && root.watch !== undefined ? (
        <RootWatch watch={root.watch} />
      ) : null}
      <RootAnswers root={root} onAnswer={props.onAnswer} />
      <RootRepairs
        root={root}
        places={props.places ?? []}
        onRetire={() => {
          props.onAnswer(root, { kind: "retire" });
        }}
        onRepoint={(to) => props.onRepoint?.(root, to)}
      />
    </li>
  );
}

/**
 * A folder's two repairs, rarely used, under its own "…" (ui-audit-2026-11-04.md §3, A library):
 * retiring it for good, and moving it where it now is, where there is a place to move it to. Each
 * opens a dialog whose least harmful choice, keeping it, is focused as it opens.
 */
function RootRepairs(props: {
  readonly root: Root;
  readonly places: readonly Place[];
  readonly onRetire: () => void;
  readonly onRepoint: (to: Place) => void;
}): JSX.Element | null {
  const { t } = useTranslation();
  const from = { nodeName: props.root.nodeName, path: props.root.rootPath };
  const retire: MoreAction = {
    key: "retire",
    label: t("libraries.retireRoot"),
    dialog: (close) => (
      <ConfirmDialog
        heading={t("libraries.retireRootHeading")}
        consequence={t("libraries.retireRootConsequence")}
        confirmLabel={t("libraries.retireRootConfirm")}
        onConfirm={props.onRetire}
        onClose={close}
      />
    ),
  };
  const repoint: MoreAction = {
    key: "repoint",
    label: t("libraries.repoint"),
    dialog: (close) => (
      <RepointDialog
        from={from}
        places={props.places}
        onRepoint={props.onRepoint}
        onClose={close}
      />
    ),
  };
  return (
    <MoreActions
      label={t("libraries.folderMore", { node: from.nodeName, path: from.path })}
      actions={props.places.length === 0 ? [retire] : [retire, repoint]}
    />
  );
}

/**
 * The answers a root waits for (D208): that its withheld removals are real, and for each mount point
 * inside it whose disk is missing, that the disk is gone for good.
 */
function RootAnswers(props: {
  readonly root: Root;
  readonly onAnswer: (root: Root, answer: RootAnswer) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { root } = props;
  return (
    <>
      {root.withheldReason === undefined ? null : <p>{root.withheldReason}</p>}
      {root.lastScanOutcome === "withheld" ? (
        <button
          type="button"
          className="irreversible"
          onClick={() => {
            props.onAnswer(root, { kind: "approveRemovals" });
          }}
        >
          {t("libraries.approveRemovals")}
        </button>
      ) : null}
      {(root.missingMountPaths ?? []).map((mountPath) => (
        <p key={mountPath}>
          {t("libraries.diskMissing")} <code>{mountPath}</code>{" "}
          <button
            type="button"
            onClick={() => {
              props.onAnswer(root, { kind: "diskGone", mountPath });
            }}
          >
            {t("libraries.diskGone")}
          </button>
        </p>
      ))}
    </>
  );
}

/**
 * Whether a watched library's root is watched by its node (D272 §9): if not, why, and that its
 * periodic scan finds what changes meanwhile.
 */
function RootWatch(props: { readonly watch: NonNullable<Root["watch"]> }): JSX.Element {
  const { t } = useTranslation();
  const { watch } = props;
  if (watch.isWatching || watch.cause === "") {
    return <span>{t("libraries.watching")}</span>;
  }
  return (
    <p>
      {t(watchCauseKeys[watch.cause])} {t("libraries.watchFallback")}
      {watch.reason === undefined ? null : <small> {watch.reason}</small>}
    </p>
  );
}

/**
 * A root's scan in progress (D236 §2): listing the root, then the files it found and how many of
 * the new or changed ones are read; and that one more runs after it, when one was asked for (§3).
 */
function ScanProgress(props: { readonly scan: NonNullable<Root["scan"]> }): JSX.Element {
  const { t } = useTranslation();
  const { scan } = props;
  return (
    <output>
      {scan.isListed
        ? t("libraries.scanReading", {
            count: scan.listedCount,
            read: scan.examinedCount,
            total: scan.toExamineCount,
          })
        : t("libraries.scanListing")}
      {scan.isAnotherScanQueued ? ` ${t("libraries.scanThenAgain")}` : null}
    </output>
  );
}

/** The ways a library notices changes (D184). */
const changeDetections = ["watch", "interval", "manual"] as const;

/** How a library notices changes, as the API takes it (D184). */
export type ChangeDetection = Pick<Library, "changeDetection" | "checkEveryHours">;

/**
 * How a library notices changes, chosen again after it was made (the owner, 2026-10-09): watching,
 * checking every so many hours, or when asked; saved at once (the owner, 2026-11-04, Q5), a choice
 * as it is made and the hours on Enter or on leaving their field, an interval only once it has its
 * hours, and saying Saved, or the refusal, beside it.
 */
export function ChangeDetectionChoice(props: {
  readonly library: Library;
  readonly onSave: (choice: ChangeDetection) => Promise<void>;
}): JSX.Element {
  const { t } = useTranslation();
  const { library } = props;
  const [detection, setDetection] = useState(library.changeDetection);
  const [hours, setHours] = useState<number | undefined>(library.checkEveryHours);
  const { state, send } = useSentOnce(library, props.onSave);
  const isInterval = detection === "interval";
  const withHours = (chosen: Library["changeDetection"]): ChangeDetection =>
    chosen === "interval" && hours !== undefined
      ? { changeDetection: chosen, checkEveryHours: hours }
      : { changeDetection: chosen };
  return (
    <form
      onSubmit={(event) => {
        event.preventDefault();
        send(withHours(detection));
      }}
    >
      <label>
        {t("libraries.changeDetection")}{" "}
        <select
          value={detection}
          onChange={(event) => {
            const chosen = changeDetections.find((each) => each === event.target.value);
            if (chosen !== undefined) {
              setDetection(chosen);
              send(withHours(chosen));
            }
          }}
        >
          <option value="watch">{t("libraries.changesWatch")}</option>
          <option value="interval">{t("libraries.changesInterval")}</option>
          <option value="manual">{t("libraries.changesManual")}</option>
        </select>
      </label>{" "}
      {isInterval ? (
        <label>
          {t("libraries.checkEvery")}{" "}
          <input
            type="number"
            min={1}
            required
            value={hours ?? ""}
            onChange={(event) => {
              setHours(event.target.value === "" ? undefined : Number(event.target.value));
            }}
            onBlur={() => {
              send(withHours(detection));
            }}
          />
        </label>
      ) : null}{" "}
      <SaveNote state={state} />
    </form>
  );
}

/**
 * Sending a way of noticing changes as it is chosen, not the one last sent again, nor an interval
 * still without its hours; a refused one may be sent again.
 */
function useSentOnce(
  library: Library,
  onSave: (choice: ChangeDetection) => Promise<void>,
): { readonly state: SaveState; readonly send: (choice: ChangeDetection) => void } {
  const { state, save } = useSaveState();
  const [sent, setSent] = useState(() => choiceKey(library));
  const send = (choice: ChangeDetection): void => {
    if (choice.changeDetection === "interval" && choice.checkEveryHours === undefined) {
      return;
    }
    if (choiceKey(choice) === sent) {
      return;
    }
    setSent(choiceKey(choice));
    void save(async () => {
      await onSave(choice);
    }).then((isSaved) => {
      if (!isSaved) {
        setSent("");
      }
    });
  };
  return { state, send };
}

/** A way of noticing changes as one string, to tell whether it is the one last sent. */
function choiceKey(choice: ChangeDetection): string {
  return `${choice.changeDetection}:${String(choice.checkEveryHours)}`;
}

/**
 * Managing one library, its own page's content (D361 §2): its name and what it is, its folders with
 * their state and the answers about them, adding one, how it notices changes, its files by filing,
 * its scan — again once the scan running ends, while one does (D236 §3) — its rescan (D357), its note,
 * and deleting it, confirmed first (D231 §7).
 */
export function LibraryManagement(props: {
  readonly library: Library;
  readonly note: string;
  readonly onScan: () => void;
  /** Its rescan (D357), or its full rescan (D392), offered for a library of films or of series. */
  readonly onRescan?: (isFull: boolean) => void;
  readonly onAnswer: (root: Root, answer: RootAnswer) => void;
  readonly onDelete: () => void;
  readonly places?: readonly Place[];
  readonly onRepoint?: (root: Root, to: Place) => void;
  /** Adding a folder to it, one of a node's configured roots or a folder inside one (D359 §3). */
  readonly onAddRoot?: (to: Place) => void;
  readonly onChangeDetection?: (choice: ChangeDetection) => Promise<void>;
}): JSX.Element {
  const { t } = useTranslation();
  const { library } = props;
  return (
    <article className="library" aria-labelledby="library-heading">
      <h1 id="library-heading">{library.name}</h1>
      <LibrarySummary library={library} />
      <LibraryButtons
        library={library}
        onScan={props.onScan}
        {...(props.onRescan === undefined ? {} : { onRescan: props.onRescan })}
      />
      {props.note === "" ? null : <output>{props.note}</output>}
      <h2>{t("libraries.foldersHeading")}</h2>
      <ul className="library__folders">
        {(library.roots ?? []).map((root) => (
          <RootLine
            key={`${root.nodeName}:${root.rootPath}`}
            root={root}
            isWatched={props.library.changeDetection === "watch"}
            onAnswer={props.onAnswer}
            places={props.places ?? []}
            {...(props.onRepoint === undefined ? {} : { onRepoint: props.onRepoint })}
          />
        ))}
      </ul>
      {props.onAddRoot === undefined ? null : (
        <AddRoot places={props.places ?? []} onAdd={props.onAddRoot} />
      )}
      <h2>{t("libraries.scansHeading")}</h2>
      <ChangeDetectionChoice
        library={library}
        onSave={async (choice) => {
          await props.onChangeDetection?.(choice);
        }}
      />
      <FilingCounts library={library} />
      {library.rescan === undefined ? null : <RescanProgress rescan={library.rescan} />}
      <h2>{t("libraries.deleteHeading")}</h2>
      <ConfirmButton
        label={t("libraries.delete")}
        consequence={t("libraries.deleteConsequence")}
        confirmLabel={t("libraries.deleteConfirm")}
        onConfirm={props.onDelete}
      />
    </article>
  );
}

/**
 * A library's scan, first on its page and where focus arrives (ui-audit-2026-11-04.md §3, A
 * library), for a library of films or of series its rescan (D357) and its full rescan beside it
 * (D392), then its files.
 */
function LibraryButtons(props: {
  readonly library: Library;
  readonly onScan: () => void;
  readonly onRescan?: (isFull: boolean) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { library } = props;
  return (
    <p>
      <button type="button" {...arrivalMark} onClick={props.onScan}>
        {(library.roots ?? []).some((root) => root.scan !== undefined)
          ? t("libraries.scanAgainAfter")
          : t("libraries.scan")}
      </button>
      {props.onRescan !== undefined && isRescannable(library) ? (
        <>
          {" "}
          <button
            type="button"
            title={t("libraries.rescanHelp")}
            onClick={() => {
              props.onRescan?.(false);
            }}
          >
            {t("libraries.rescan")}
          </button>{" "}
          <button
            type="button"
            title={t("libraries.fullRescanHelp")}
            onClick={() => {
              props.onRescan?.(true);
            }}
          >
            {t("libraries.fullRescan")}
          </button>
        </>
      ) : null}{" "}
      <Link to={{ screen: "files", libraryId: library.id }}>{t("libraries.files")}</Link>
    </p>
  );
}

/** What an admin does with a library on its page, one request at a time (D244 §4), and its note. */
interface LibraryActions {
  readonly note: string;
  readonly isActing: boolean;
  readonly scan: () => void;
  readonly rescan: (isFull: boolean) => void;
  readonly answerAboutRoot: (root: Root, answer: RootAnswer) => void;
  readonly deleteLibrary: () => void;
  readonly repoint: (root: Root, to: Place) => void;
  readonly addRoot: (to: Place) => void;
  readonly setChangeDetection: (choice: ChangeDetection) => Promise<void>;
}

/**
 * The library's note, and its actions run one at a time: the note says what an action asked for,
 * or what went wrong with it.
 */
function useLibraryNote(): {
  readonly note: string;
  readonly setNote: (note: string) => void;
  readonly act: (action: (idempotencyKey: string) => Promise<void>) => void;
  readonly isActing: boolean;
} {
  const { t } = useTranslation();
  const [note, setNote] = useState("");
  const acting = useRequest(() => "");
  const act = (action: (idempotencyKey: string) => Promise<void>): void => {
    acting.runRequest(async (idempotencyKey) => {
      try {
        await action(idempotencyKey);
      } catch (cause: unknown) {
        setNote(problemText(t, cause));
      }
    });
  };
  return { note, setNote, act, isActing: acting.isBusy };
}

/** The library's scan and its rescan (D357) or full rescan (D392), each saying what it asked for in the note. */
function useScanActions(
  libraryId: number,
  noted: Pick<ReturnType<typeof useLibraryNote>, "act" | "setNote"> & {
    readonly reload: () => void;
  },
): Pick<LibraryActions, "scan" | "rescan"> {
  const { t } = useTranslation();
  const { api } = useServices();
  const { act, setNote, reload } = noted;
  return {
    scan: () => {
      act(async (idempotencyKey) => {
        const askedRootCount = await scanLibrary(api, libraryId, idempotencyKey);
        setNote(t("libraries.scanning", { count: askedRootCount }));
      });
    },
    rescan: (isFull) => {
      act(async (idempotencyKey) => {
        const answer = dataOrThrow(
          await api.POST("/libraries/{id}/rescan", {
            params: { path: { id: libraryId }, ...(isFull ? { query: { full: true } } : {}) },
            ...idempotent(idempotencyKey),
          }),
        );
        const started = isFull ? t("libraries.fullRescanStarted") : t("libraries.rescanStarted");
        setNote(answer.isStarted ? started : t("libraries.rescanRunning"));
        reload();
      });
    },
  };
}

/** The library's folders: the answers about one, moving one and adding one. */
function useFolderActions(
  libraryId: number,
  noted: Pick<ReturnType<typeof useLibraryNote>, "act" | "setNote"> & {
    readonly reload: () => void;
  },
): Pick<LibraryActions, "answerAboutRoot" | "repoint" | "addRoot"> {
  const { t } = useTranslation();
  const { api } = useServices();
  const { act, setNote, reload } = noted;
  return {
    answerAboutRoot: (root, answer) => {
      act(async (idempotencyKey) => {
        await sendRootAnswer(
          api,
          { libraryId, request: rootAnswerRequest(root, answer) },
          idempotencyKey,
        );
        reload();
      });
    },
    repoint: (root, to) => {
      act(async (idempotencyKey) => {
        const movedFileCount = await repointRoot(api, { root, to }, idempotencyKey);
        setNote(t("libraries.repointed", { count: movedFileCount }));
        reload();
      });
    },
    addRoot: (to) => {
      act(async (idempotencyKey) => {
        await addLibraryRoot(api, { libraryId, to }, idempotencyKey);
        setNote(t("libraries.rootAdded", { path: to.path }));
        reload();
      });
    },
  };
}

/**
 * What the admin does with the library on its page. Deleting it goes back to the list, where it is
 * no longer listed (D361 §2).
 */
function useLibraryActions(libraryId: number, reload: () => void): LibraryActions {
  const { api, navigation } = useServices();
  const { note, setNote, act, isActing } = useLibraryNote();
  return {
    note,
    isActing,
    ...useScanActions(libraryId, { act, setNote, reload }),
    ...useFolderActions(libraryId, { act, setNote, reload }),
    setChangeDetection: async (choice) => {
      throwIfRefused(
        await api.PUT("/libraries/{id}/change-detection", {
          params: { path: { id: libraryId } },
          body: choice,
        }),
      );
      reload();
    },
    deleteLibrary: () => {
      act(async () => {
        throwIfRefused(
          await api.DELETE("/libraries/{id}", { params: { path: { id: libraryId } } }),
        );
        navigation.go({ screen: "libraries" });
      });
    },
  };
}

/** Asks for a scan of a library's roots, and says how many it asked for. */
async function scanLibrary(api: Api, libraryId: number, idempotencyKey: string): Promise<number> {
  const scanAnswer = dataOrThrow(
    await api.POST("/libraries/{id}/scan", {
      params: { path: { id: libraryId } },
      ...idempotent(idempotencyKey),
    }),
  );
  return scanAnswer.askedRootCount;
}

/** Sends the admin's answer about one of a library's roots. */
async function sendRootAnswer(
  api: Api,
  answer: { readonly libraryId: number; readonly request: ReturnType<typeof rootAnswerRequest> },
  idempotencyKey: string,
): Promise<void> {
  throwIfRefused(
    await api.POST(answer.request.endpoint, {
      params: { path: { id: answer.libraryId } },
      body: answer.request.body,
      ...idempotent(idempotencyKey),
    }),
  );
}

/** Says a library's folder moved to another place, and how many files went with it (D429 §2). */
async function repointRoot(
  api: Api,
  move: { readonly root: Root; readonly to: Place },
  idempotencyKey: string,
): Promise<number> {
  const { root, to } = move;
  const moved = dataOrThrow(
    await api.POST("/repoints", {
      body: {
        nodeName: root.nodeName,
        path: root.rootPath,
        newNodeName: to.nodeName,
        newPath: to.path,
      },
      ...idempotent(idempotencyKey),
    }),
  );
  return moved.livePlaceCount;
}

/** Adds a folder to a library, which the server scans (D359 §3). */
async function addLibraryRoot(
  api: Api,
  adding: { readonly libraryId: number; readonly to: Place },
  idempotencyKey: string,
): Promise<void> {
  throwIfRefused(
    await api.POST("/libraries/{id}/roots", {
      params: { path: { id: adding.libraryId } },
      body: { nodeName: adding.to.nodeName, path: adding.to.path },
      ...idempotent(idempotencyKey),
    }),
  );
}

/**
 * Every place a root can move to, or a folder be added from: each connected node's configured roots,
 * and the folders browsed to inside them (D359).
 */
function placesOf(answer: components["schemas"]["NodesOutputBody"]): Place[] {
  return (answer.nodes ?? [])
    .filter((node) => node.isConnected)
    .flatMap((node) => (node.rootPaths ?? []).map((path) => ({ nodeName: node.name, path })));
}

/**
 * One library's own page, where the admin manages it (D361 §2), at `/libraries/<id>`: its trail up
 * to the list (D367 §8), and the library, read with the list and kept up to date as it is; an id no library has
 * says so.
 */
export function LibraryPage(props: { readonly libraryId: number }): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [libraries, reload] = useResource(
    "libraries",
    () => listLibraries(api),
    useBackgroundChangesPoller(),
  );
  const actions = useLibraryActions(props.libraryId, reload);
  const [places] = useResource("library-places", async () =>
    placesOf(dataOrThrow(await api.GET("/nodes"))),
  );
  const placeList = places.state === "loaded" ? places.value : [];
  const render = (library: Library): JSX.Element => (
    <LibraryManagement
      library={library}
      note={actions.note}
      onScan={actions.scan}
      onRescan={actions.rescan}
      onAnswer={actions.answerAboutRoot}
      onDelete={actions.deleteLibrary}
      places={placeList}
      onRepoint={actions.repoint}
      onAddRoot={actions.addRoot}
      onChangeDetection={actions.setChangeDetection}
    />
  );
  return (
    <section className="libraries">
      <UpTrail
        upward={{
          crumbs: [adminLibrariesCrumb(t)],
          here:
            libraries.state === "loaded"
              ? (libraries.value.find((each) => each.id === props.libraryId)?.name ?? "")
              : "",
        }}
      />
      <BusyGroup isBusy={actions.isActing}>
        <Loaded
          resource={libraries}
          render={(list) => {
            const library = list.find((each) => each.id === props.libraryId);
            return library === undefined ? <p>{t("libraries.notFound")}</p> : render(library);
          }}
        />
      </BusyGroup>
    </section>
  );
}
