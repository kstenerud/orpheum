// SPDX-License-Identifier: MPL-2.0

import type { Api, components } from "@orpheum/sdk";
import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { BusyGroup } from "./busy-group.tsx";
import { useRequest } from "./use-request.ts";
import { idempotent } from "./idempotent.ts";
import { useBackgroundChanges } from "./polling.ts";
import { dataOrThrow, problemText, throwIfRefused } from "./problem.ts";
import { filingName } from "./files.tsx";
import { Link } from "./link.tsx";
import { useServices } from "./services.tsx";
import { Loaded } from "./title.tsx";
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
  readonly onAnswer: (root: Root, answer: RootAnswer) => void;
  /** Where it can move to, and saying it did (D241 §6). */
  readonly places?: readonly Place[];
  readonly onRepoint?: (root: Root, to: Place) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { root } = props;
  return (
    <li>
      <code>{t("libraries.root", { node: root.nodeName, path: root.rootPath })}</code>{" "}
      {root.lastScanOutcome === "" ? null : <span>{t(outcomeKeys[root.lastScanOutcome])}</span>}
      {root.scan === undefined ? null : <ScanProgress scan={root.scan} />}
      {root.withheldReason === undefined ? null : <p>{root.withheldReason}</p>}
      {root.lastScanOutcome === "withheld" ? (
        <button
          type="button"
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
 * One library: its name, its roots, its files by filing, asking it to scan — again once the scan
 * running ends, while one does (D236 §3) — and deleting it, confirmed first (D231 §7).
 */
export function LibraryCard(props: {
  readonly library: Library;
  readonly note: string;
  readonly onScan: () => void;
  readonly onAnswer: (root: Root, answer: RootAnswer) => void;
  readonly onDelete: () => void;
  readonly places?: readonly Place[];
  readonly onRepoint?: (root: Root, to: Place) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { library } = props;
  const filingCounts = Object.entries(library.fileCountsByFiling ?? {});
  return (
    <article aria-labelledby={`library-${String(library.id)}`}>
      <h2 id={`library-${String(library.id)}`}>{library.name}</h2>
      <ul>
        {(library.roots ?? []).map((root) => (
          <RootLine
            key={`${root.nodeName}:${root.rootPath}`}
            root={root}
            onAnswer={props.onAnswer}
            places={props.places ?? []}
            {...(props.onRepoint === undefined ? {} : { onRepoint: props.onRepoint })}
          />
        ))}
      </ul>
      {filingCounts.length === 0 ? null : (
        <p>
          {filingCounts
            .map(([filing, count]) =>
              t("libraries.filing", {
                filing: filingName(t, filing),
                count,
              }),
            )
            .join(" · ")}
        </p>
      )}
      <p>
        <Link to={{ screen: "files", libraryId: library.id }}>{t("libraries.files")}</Link>{" "}
        <button type="button" onClick={props.onScan}>
          {(library.roots ?? []).some((root) => root.scan !== undefined)
            ? t("libraries.scanAgainAfter")
            : t("libraries.scan")}
        </button>{" "}
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

function useLibraryActions(reload: () => void): {
  readonly notesByLibraryId: ReadonlyMap<number, string>;
  readonly scan: (libraryId: number) => void;
  readonly answerAboutRoot: (libraryId: number, root: Root, answer: RootAnswer) => void;
  readonly deleteLibrary: (libraryId: number) => void;
  readonly repoint: (libraryId: number, root: Root, to: Place) => void;
  /** Whether an action runs, which every library's buttons wait for (D244 §4). */
  readonly isActing: boolean;
} {
  const { t } = useTranslation();
  const { api } = useServices();
  const { notesByLibraryId, setNote, act, isActing } = useLibraryNotes();
  return {
    notesByLibraryId,
    isActing,
    scan: (libraryId) => {
      act(libraryId, async (idempotencyKey) => {
        const askedRootCount = await scanLibrary(api, libraryId, idempotencyKey);
        setNote(libraryId, t("libraries.scanning", { count: askedRootCount }));
      });
    },
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

/** The libraries, each with its roots and files, and a form to create another. */
export function Libraries(): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [libraries, reload] = useResource(
    "libraries",
    async () => {
      const librariesBody = dataOrThrow(await api.GET("/libraries"));
      return librariesBody.libraries ?? [];
    },
    useBackgroundChanges(),
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
    />
  );
  return (
    <section aria-labelledby="libraries-heading">
      <h1 id="libraries-heading">{t("libraries.heading")}</h1>
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
