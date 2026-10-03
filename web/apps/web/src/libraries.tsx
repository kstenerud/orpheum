// SPDX-License-Identifier: MPL-2.0

import type { components } from "@orpheum/sdk";
import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { useBackgroundChanges } from "./polling.ts";
import { dataOrThrow, problemText, throwIfRefused } from "./problem.ts";
import { filingName } from "./files.tsx";
import { Link } from "./link.tsx";
import { useServices } from "./services.tsx";
import { Loaded } from "./title.tsx";
import { useResource } from "./use-resource.ts";
import { ConfirmButton } from "./confirm-button.tsx";
import { CreateLibrary } from "./create-library.tsx";

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
            total: scan.examineCount,
          })
        : t("libraries.scanListing")}
      {scan.isScanAgain ? ` ${t("libraries.scanThenAgain")}` : null}
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
function useLibraryActions(reload: () => void): {
  readonly notesByLibraryId: ReadonlyMap<number, string>;
  readonly scan: (libraryId: number) => void;
  readonly answerAboutRoot: (libraryId: number, root: Root, answer: RootAnswer) => void;
  readonly deleteLibrary: (libraryId: number) => void;
} {
  const { t } = useTranslation();
  const { api } = useServices();
  const [notesByLibraryId, setNotesByLibraryId] = useState<ReadonlyMap<number, string>>(
    () => new Map(),
  );
  const setNote = (libraryId: number, note: string): void => {
    setNotesByLibraryId((previous) => new Map(previous).set(libraryId, note));
  };
  return {
    notesByLibraryId,
    scan: (libraryId) => {
      api
        .POST("/libraries/{id}/scan", { params: { path: { id: libraryId } } })
        .then(dataOrThrow)
        .then(
          (scanAnswer) => {
            setNote(libraryId, t("libraries.scanning", { count: scanAnswer.askedRootCount }));
          },
          (cause: unknown) => {
            setNote(libraryId, problemText(t, cause));
          },
        );
    },
    answerAboutRoot: (libraryId, root, answer) => {
      const request = rootAnswerRequest(root, answer);
      api
        .POST(request.endpoint, {
          params: { path: { id: libraryId } },
          body: request.body,
        })
        .then(throwIfRefused)
        .then(reload, (cause: unknown) => {
          setNote(libraryId, problemText(t, cause));
        });
    },
    deleteLibrary: (libraryId) => {
      api
        .DELETE("/libraries/{id}", { params: { path: { id: libraryId } } })
        .then(throwIfRefused)
        .then(reload, (cause: unknown) => {
          setNote(libraryId, problemText(t, cause));
        });
    },
  };
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
    />
  );
  return (
    <section aria-labelledby="libraries-heading">
      <h1 id="libraries-heading">{t("libraries.heading")}</h1>
      <Loaded
        resource={libraries}
        render={(value) =>
          value.length === 0 ? <p>{t("libraries.none")}</p> : <>{value.map(renderCard)}</>
        }
      />
      <CreateLibrary onCreated={reload} />
    </section>
  );
}
