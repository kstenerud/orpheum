// SPDX-License-Identifier: MPL-2.0

import type { components } from "@orpheum/sdk";
import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { dataOrThrow, problemText, throwIfRefused } from "./problem.ts";
import { filingName } from "./files.tsx";
import { Link } from "./link.tsx";
import { useServices } from "./services.tsx";
import { Loaded } from "./title.tsx";
import { useResource } from "./use-resource.ts";
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
 * An admin's answer about a root (D208): its withheld removals are real, or the disk missing from
 * one of the mount points inside it is gone for good.
 */
export type RootAnswer =
  { readonly kind: "approveRemovals" } | { readonly kind: "diskGone"; readonly mountPath: string };

/**
 * Where an answer goes and what it sends: removals name the root's own path; a disk gone names
 * the missing mount point's, one of those the root lists, which is what the server looks up.
 */
export function rootAnswerRequest(
  root: Root,
  answer: RootAnswer,
): {
  readonly endpoint: "/libraries/{id}/removals" | "/libraries/{id}/mounts/gone";
  readonly body: { readonly nodeName: string; readonly path: string };
} {
  return answer.kind === "approveRemovals"
    ? {
        endpoint: "/libraries/{id}/removals",
        body: { nodeName: root.nodeName, path: root.rootPath },
      }
    : {
        endpoint: "/libraries/{id}/mounts/gone",
        body: { nodeName: root.nodeName, path: answer.mountPath },
      };
}

/**
 * One root: where it is, how its last scan went, the admin's answer when its removals wait, and
 * for each mount point inside it whose disk is missing, the answer that it is gone (D208).
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
    </li>
  );
}

/** One library: its name, its roots, its files by filing, and asking it to scan. */
export function LibraryCard(props: {
  readonly library: Library;
  readonly note: string;
  readonly onScan: () => void;
  readonly onAnswer: (root: Root, answer: RootAnswer) => void;
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
          {t("libraries.scan")}
        </button>
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
  };
}

/** The libraries, each with its roots and files, and a form to create another. */
export function Libraries(): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [libraries, reload] = useResource("libraries", async () => {
    const librariesBody = dataOrThrow(await api.GET("/libraries"));
    return librariesBody.libraries ?? [];
  });
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
