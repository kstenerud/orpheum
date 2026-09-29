// SPDX-License-Identifier: MPL-2.0

import type { components } from "@orpheum/sdk";
import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { dataOrThrow, problemText, throwIfRefused } from "./problem.ts";
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

/** One root: where it is, how its last scan went, and the admin's two answers when it waits (D208). */
export function RootLine(props: {
  readonly root: Root;
  readonly onApproveRemovals: (root: Root) => void;
  readonly onMarkGone: (root: Root) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { root } = props;
  return (
    <li>
      <code>{t("libraries.root", { node: root.nodeName, path: root.rootPath })}</code>{" "}
      {root.lastScanOutcome === "" ? null : <span>{t(outcomeKeys[root.lastScanOutcome])}</span>}
      {root.withheldReason === undefined ? null : <p>{root.withheldReason}</p>}
      {root.lastScanOutcome === "withheld" ? (
        <>
          <button
            type="button"
            onClick={() => {
              props.onApproveRemovals(root);
            }}
          >
            {t("libraries.approveRemovals")}
          </button>{" "}
          <button
            type="button"
            onClick={() => {
              props.onMarkGone(root);
            }}
          >
            {t("libraries.diskGone")}
          </button>
        </>
      ) : null}
    </li>
  );
}

/** One library: its name, its roots, its files by filing, and asking it to scan. */
export function LibraryCard(props: {
  readonly library: Library;
  readonly note: string;
  readonly onScan: () => void;
  readonly onApproveRemovals: (root: Root) => void;
  readonly onMarkGone: (root: Root) => void;
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
            onApproveRemovals={props.onApproveRemovals}
            onMarkGone={props.onMarkGone}
          />
        ))}
      </ul>
      {filingCounts.length === 0 ? null : (
        <p>
          {filingCounts
            .map(([filing, count]) =>
              t("libraries.filing", {
                filing: t(`filing.${filing}`, { defaultValue: filing }),
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
  readonly answerWithheldRoot: (
    libraryId: number,
    root: Root,
    endpoint: "/libraries/{id}/removals" | "/libraries/{id}/mounts/gone",
  ) => void;
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
    answerWithheldRoot: (libraryId, root, endpoint) => {
      api
        .POST(endpoint, {
          params: { path: { id: libraryId } },
          body: { nodeName: root.nodeName, path: root.rootPath },
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
      onApproveRemovals={(root) => {
        actions.answerWithheldRoot(library.id, root, "/libraries/{id}/removals");
      }}
      onMarkGone={(root) => {
        actions.answerWithheldRoot(library.id, root, "/libraries/{id}/mounts/gone");
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
