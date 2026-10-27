// SPDX-License-Identifier: MPL-2.0

import type { Api, components } from "@orpheum/sdk";
import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { RootWarning } from "./root-warning.tsx";
import { BusyGroup } from "./busy-group.tsx";
import { useRequest } from "./use-request.ts";
import { idempotent } from "./idempotent.ts";
import { useBackgroundChangesPoller } from "./polling.ts";
import { dataOrThrow, problemText, throwIfRefused } from "./problem.ts";
import { filingName } from "./files.tsx";
import { Link } from "./link.tsx";
import { useServices } from "./services.tsx";
import { Loaded } from "./loaded.tsx";
import { useResource } from "./use-resource.ts";
import { ConfirmButton } from "./confirm-button.tsx";
import { CreateLibrary } from "./create-library.tsx";
import { RepointRoot, type Place } from "./repoint-root.tsx";

/** A library, as an admin sees it. */
export type Library = components["schemas"]["LibraryBody"];
type Root = components["schemas"]["RootBody"];

const outcomeKeys = {
  complete: "libraries.outcomeComplete",
  unreadable: "libraries.outcomeUnreadable",
  withheld: "libraries.outcomeWithheld",
} as const;

/** Why a node does not watch a root, as its status said (D272 §9). */
const watchCauseKeys = {
  "network-filesystem": "libraries.watchCauseNetworkFilesystem",
  "watch-limit": "libraries.watchCauseWatchLimit",
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
    "/libraries/{id}/removals" | "/libraries/{id}/mounts/gone" | "/libraries/{id}/roots/retire";
  readonly body: { readonly nodeName: string; readonly path: string };
} {
  switch (answer.kind) {
    case "approveRemovals":
      return {
        endpoint: "/libraries/{id}/removals",
        body: { nodeName: root.nodeName, path: root.rootPath },
      };
    case "diskGone":
      return {
        endpoint: "/libraries/{id}/mounts/gone",
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
    <li>
      <RootWarning root={root} />
      <code>{t("libraries.root", { node: root.nodeName, path: root.rootPath })}</code>{" "}
      {root.lastScanOutcome === "" ? null : <span>{t(outcomeKeys[root.lastScanOutcome])}</span>}
      {root.scan === undefined ? null : <ScanProgress scan={root.scan} />}
      {props.isWatched === true && root.watch !== undefined ? (
        <RootWatch watch={root.watch} />
      ) : null}
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
      <ConfirmButton
        label={t("libraries.retireRoot")}
        consequence={t("libraries.retireRootConsequence")}
        confirmLabel={t("libraries.retireRootConfirm")}
        onConfirm={() => {
          props.onAnswer(root, { kind: "retire" });
        }}
      />{" "}
      <RepointRoot
        from={{ nodeName: root.nodeName, path: root.rootPath }}
        places={props.places ?? []}
        onRepoint={(to) => props.onRepoint?.(root, to)}
      />
    </li>
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

/**
 * A library's rescans as they run (D357 §5): scanning first, then how many of the films and series
 * listed were matched again.
 */
function RescanProgress(props: { readonly rescan: NonNullable<Library["rescan"]> }): JSX.Element {
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

/** Whether a library is rescanned whole: one of films or of series, never mixed or other (D357 §1). */
export function isRescannable(library: Pick<Library, "kind">): boolean {
  return library.kind === "movies" || library.kind === "shows";
}

/** The ways a library notices changes (D184). */
const changeDetections = ["watch", "interval", "manual"] as const;

/** How a library notices changes, as the API takes it (D184). */
export type ChangeDetection = Pick<Library, "changeDetection" | "checkEveryHours">;

/**
 * How a library notices changes, chosen again after it was made (the owner, 2026-10-09): watching,
 * checking every so many hours, or when asked; saved when the admin says so, hours only with an
 * interval, which the admin gives.
 */
export function ChangeDetectionChoice(props: {
  readonly library: Library;
  readonly onSave: (choice: ChangeDetection) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { library } = props;
  const [detection, setDetection] = useState(library.changeDetection);
  const [hours, setHours] = useState<number | undefined>(library.checkEveryHours);
  const isInterval = detection === "interval";
  const isUnchanged =
    detection === library.changeDetection && (!isInterval || hours === library.checkEveryHours);
  return (
    <form
      onSubmit={(event) => {
        event.preventDefault();
        props.onSave(
          isInterval && hours !== undefined
            ? { changeDetection: detection, checkEveryHours: hours }
            : { changeDetection: detection },
        );
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
          />
        </label>
      ) : null}{" "}
      <button type="submit" disabled={isUnchanged}>
        {t("libraries.changeDetectionSave")}
      </button>
    </form>
  );
}

/** A library's files counted by filing, when the admin sees them. */
function FilingCounts(props: { readonly library: Library }): JSX.Element | null {
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

/**
 * One library: its name, its roots, its files by filing, asking it to scan — again once the scan
 * running ends, while one does (D236 §3) — and deleting it, confirmed first (D231 §7).
 */
export function LibraryCard(props: {
  readonly library: Library;
  readonly note: string;
  readonly onScan: () => void;
  /** Its rescan (D357), offered for a library of films or of series. */
  readonly onRescan?: () => void;
  readonly onAnswer: (root: Root, answer: RootAnswer) => void;
  readonly onDelete: () => void;
  readonly places?: readonly Place[];
  readonly onRepoint?: (root: Root, to: Place) => void;
  readonly onChangeDetection?: (choice: ChangeDetection) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { library } = props;
  return (
    <article aria-labelledby={`library-${String(library.id)}`}>
      <h2 id={`library-${String(library.id)}`}>{library.name}</h2>
      <ul>
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
      <ChangeDetectionChoice
        key={`${library.changeDetection}:${String(library.checkEveryHours)}`}
        library={library}
        onSave={(choice) => props.onChangeDetection?.(choice)}
      />
      <FilingCounts library={library} />
      {library.rescan === undefined ? null : <RescanProgress rescan={library.rescan} />}
      <p>
        <Link to={{ screen: "files", libraryId: library.id }}>{t("libraries.files")}</Link>{" "}
        <button type="button" onClick={props.onScan}>
          {(library.roots ?? []).some((root) => root.scan !== undefined)
            ? t("libraries.scanAgainAfter")
            : t("libraries.scan")}
        </button>{" "}
        {props.onRescan !== undefined && isRescannable(library) ? (
          <>
            <button type="button" title={t("libraries.rescanHelp")} onClick={props.onRescan}>
              {t("libraries.rescan")}
            </button>{" "}
          </>
        ) : null}
        <ConfirmButton
          label={t("libraries.delete")}
          consequence={t("libraries.deleteConsequence")}
          confirmLabel={t("libraries.deleteConfirm")}
          onConfirm={props.onDelete}
        />
      </p>
      {props.note === "" ? null : <output>{props.note}</output>}
    </article>
  );
}

/** What an admin does with libraries, and what each last said. */
/**
 * Each library's note, and its actions run one at a time: a library's own note says what went
 * wrong with it, and the request only makes every library's buttons wait (D244 §4).
 */
function useLibraryNotes(): {
  readonly notesByLibraryId: ReadonlyMap<number, string>;
  readonly setNote: (libraryId: number, note: string) => void;
  readonly act: (libraryId: number, action: (idempotencyKey: string) => Promise<void>) => void;
  readonly isActing: boolean;
} {
  const { t } = useTranslation();
  const [notesByLibraryId, setNotesByLibraryId] = useState<ReadonlyMap<number, string>>(
    () => new Map(),
  );
  const setNote = (libraryId: number, note: string): void => {
    setNotesByLibraryId((previous) => new Map(previous).set(libraryId, note));
  };
  const acting = useRequest(() => "");
  const act = (libraryId: number, action: (idempotencyKey: string) => Promise<void>): void => {
    acting.runRequest(async (idempotencyKey) => {
      try {
        await action(idempotencyKey);
      } catch (cause: unknown) {
        setNote(libraryId, problemText(t, cause));
      }
    });
  };
  return { notesByLibraryId, setNote, act, isActing: acting.isBusy };
}

/**
 * A library's scan, and its rescan (D357), each run as a library's action, saying what it asked
 * for in the library's note.
 */
function useScanActions(
  notes: Pick<ReturnType<typeof useLibraryNotes>, "act" | "setNote"> & {
    readonly reload: () => void;
  },
): {
  readonly scan: (libraryId: number) => void;
  readonly rescan: (libraryId: number) => void;
} {
  const { t } = useTranslation();
  const { api } = useServices();
  const { act, setNote, reload } = notes;
  return {
    scan: (libraryId) => {
      act(libraryId, async (idempotencyKey) => {
        const askedRootCount = await scanLibrary(api, libraryId, idempotencyKey);
        setNote(libraryId, t("libraries.scanning", { count: askedRootCount }));
      });
    },
    rescan: (libraryId) => {
      act(libraryId, async (idempotencyKey) => {
        const answer = dataOrThrow(
          await api.POST("/libraries/{id}/rescan", {
            params: { path: { id: libraryId } },
            ...idempotent(idempotencyKey),
          }),
        );
        setNote(
          libraryId,
          answer.isStarted ? t("libraries.rescanStarted") : t("libraries.rescanRunning"),
        );
        reload();
      });
    },
  };
}

function useLibraryActions(reload: () => void): {
  readonly notesByLibraryId: ReadonlyMap<number, string>;
  readonly scan: (libraryId: number) => void;
  readonly rescan: (libraryId: number) => void;
  readonly answerAboutRoot: (libraryId: number, root: Root, answer: RootAnswer) => void;
  readonly deleteLibrary: (libraryId: number) => void;
  readonly repoint: (libraryId: number, root: Root, to: Place) => void;
  readonly setChangeDetection: (libraryId: number, choice: ChangeDetection) => void;
  /** Whether an action runs, which every library's buttons wait for (D244 §4). */
  readonly isActing: boolean;
} {
  const { t } = useTranslation();
  const { api } = useServices();
  const { notesByLibraryId, setNote, act, isActing } = useLibraryNotes();
  return {
    notesByLibraryId,
    isActing,
    ...useScanActions({ act, setNote, reload }),
    answerAboutRoot: (libraryId, root, answer) => {
      act(libraryId, async (idempotencyKey) => {
        await sendRootAnswer(
          api,
          { libraryId, request: rootAnswerRequest(root, answer) },
          idempotencyKey,
        );
        reload();
      });
    },
    repoint: (libraryId, root, to) => {
      act(libraryId, async (idempotencyKey) => {
        const movedFileCount = await repointRoot(api, { libraryId, root, to }, idempotencyKey);
        setNote(libraryId, t("libraries.repointed", { count: movedFileCount }));
        reload();
      });
    },
    setChangeDetection: (libraryId, choice) => {
      act(libraryId, async () => {
        throwIfRefused(
          await api.PUT("/libraries/{id}/change-detection", {
            params: { path: { id: libraryId } },
            body: choice,
          }),
        );
        setNote(libraryId, t("libraries.changeDetectionSaved"));
        reload();
      });
    },
    deleteLibrary: (libraryId) => {
      act(libraryId, async () => {
        throwIfRefused(
          await api.DELETE("/libraries/{id}", { params: { path: { id: libraryId } } }),
        );
        reload();
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

/** Says a library's root moved to another place, and how many files went with it (D241 §6). */
async function repointRoot(
  api: Api,
  move: { readonly libraryId: number; readonly root: Root; readonly to: Place },
  idempotencyKey: string,
): Promise<number> {
  const { libraryId, root, to } = move;
  const moved = dataOrThrow(
    await api.POST("/libraries/{id}/roots/repoint", {
      params: { path: { id: libraryId } },
      body: {
        nodeName: root.nodeName,
        path: root.rootPath,
        newNodeName: to.nodeName,
        newPath: to.path,
      },
      ...idempotent(idempotencyKey),
    }),
  );
  return moved.movedFileCount;
}

/** Every place a root can move to: each connected node's configured roots. */
function placesOf(answer: components["schemas"]["NodesOutputBody"]): Place[] {
  return (answer.nodes ?? [])
    .filter((node) => node.isConnected)
    .flatMap((node) => (node.rootPaths ?? []).map((path) => ({ nodeName: node.name, path })));
}

/**
 * "Rescan everything": every library of films or of series scanned, then each film and series in it
 * matched again from its files, in the background (D357 §1); it says how many started.
 */
export function RescanEverything(props: { readonly onStarted: () => void }): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [note, setNote] = useState("");
  const rescanning = useRequest((cause) => problemText(t, cause));
  return (
    <p>
      <button
        type="button"
        title={t("libraries.rescanHelp")}
        disabled={rescanning.isBusy}
        onClick={() => {
          setNote("");
          rescanning.runRequest(async (idempotencyKey) => {
            const answer = dataOrThrow(
              await api.POST("/libraries/rescan", { ...idempotent(idempotencyKey) }),
            );
            setNote(t("libraries.rescanEverythingStarted", { count: answer.startedCount }));
            props.onStarted();
          });
        }}
      >
        {t("libraries.rescanEverything")}
      </button>
      {note === "" ? null : <output>{note}</output>}
      {rescanning.problem === undefined ? null : <output>{rescanning.problem}</output>}
    </p>
  );
}

/** The libraries the person reaches; an admin sees each one's roots and files too. */
export async function listLibraries(api: Api): Promise<Library[]> {
  return dataOrThrow(await api.GET("/libraries")).libraries ?? [];
}

/** The libraries, each with its roots and files, and a form to create another. */
export function Libraries(): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [libraries, reload] = useResource(
    "libraries",
    () => listLibraries(api),
    useBackgroundChangesPoller(),
  );
  const actions = useLibraryActions(reload);
  const [places] = useResource("library-places", async () =>
    placesOf(dataOrThrow(await api.GET("/nodes"))),
  );
  const placeList = places.state === "loaded" ? places.value : [];
  const renderCard = (library: Library): JSX.Element => (
    <LibraryCard
      key={library.id}
      library={library}
      note={actions.notesByLibraryId.get(library.id) ?? ""}
      onScan={() => {
        actions.scan(library.id);
      }}
      onRescan={() => {
        actions.rescan(library.id);
      }}
      onAnswer={(root, answer) => {
        actions.answerAboutRoot(library.id, root, answer);
      }}
      onDelete={() => {
        actions.deleteLibrary(library.id);
      }}
      places={placeList}
      onRepoint={(root, to) => {
        actions.repoint(library.id, root, to);
      }}
      onChangeDetection={(choice) => {
        actions.setChangeDetection(library.id, choice);
      }}
    />
  );
  return (
    <section aria-labelledby="libraries-heading">
      <h1 id="libraries-heading">{t("libraries.heading")}</h1>
      <RescanEverything onStarted={reload} />
      <BusyGroup isBusy={actions.isActing}>
        <Loaded
          resource={libraries}
          render={(value) =>
            value.length === 0 ? <p>{t("libraries.none")}</p> : <>{value.map(renderCard)}</>
          }
        />
      </BusyGroup>
      <CreateLibrary onCreated={reload} />
    </section>
  );
}
