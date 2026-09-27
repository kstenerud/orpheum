// SPDX-License-Identifier: MPL-2.0

import type { components } from "@orpheum/sdk";
import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { ApiProblem } from "./problem.ts";
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
  readonly onApprove: (root: Root) => void;
  readonly onGone: (root: Root) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { root } = props;
  return (
    <li>
      <code>{t("libraries.root", { node: root.node, path: root.path })}</code>{" "}
      {root.outcome === "" ? null : <span>{t(outcomeKeys[root.outcome])}</span>}
      {root.withheldReason === undefined ? null : <p>{root.withheldReason}</p>}
      {root.outcome === "withheld" ? (
        <>
          <button
            type="button"
            onClick={() => {
              props.onApprove(root);
            }}
          >
            {t("libraries.approve")}
          </button>{" "}
          <button
            type="button"
            onClick={() => {
              props.onGone(root);
            }}
          >
            {t("libraries.gone")}
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
  readonly onApprove: (root: Root) => void;
  readonly onGone: (root: Root) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { library } = props;
  const filings = Object.entries(library.filings ?? {});
  return (
    <article aria-labelledby={`library-${String(library.id)}`}>
      <h2 id={`library-${String(library.id)}`}>{library.name}</h2>
      <ul>
        {(library.roots ?? []).map((root) => (
          <RootLine
            key={`${root.node}:${root.path}`}
            root={root}
            onApprove={props.onApprove}
            onGone={props.onGone}
          />
        ))}
      </ul>
      {filings.length === 0 ? null : (
        <p>
          {filings
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
  readonly notes: ReadonlyMap<number, string>;
  readonly scan: (id: number) => void;
  readonly answer: (
    id: number,
    root: Root,
    path: "/libraries/{id}/removals" | "/libraries/{id}/mounts/gone",
  ) => void;
} {
  const { t } = useTranslation();
  const { api } = useServices();
  const [notes, setNotes] = useState<ReadonlyMap<number, string>>(() => new Map());
  const noted = (id: number, note: string): void => {
    setNotes((previous) => new Map(previous).set(id, note));
  };
  return {
    notes,
    scan: (id) => {
      void api
        .POST("/libraries/{id}/scan", { params: { path: { id } } })
        .then(({ data, error }) => {
          noted(
            id,
            data === undefined
              ? t("common.failed", { message: error.error.message })
              : t("libraries.scanning", { count: data.rootsAsked }),
          );
        });
    },
    answer: (id, root, path) => {
      void api
        .POST(path, { params: { path: { id } }, body: { node: root.node, path: root.path } })
        .then(({ error }) => {
          if (error === undefined) {
            reload();
          } else {
            noted(id, t("common.failed", { message: error.error.message }));
          }
        });
    },
  };
}

/** The libraries, each with its roots and files, and a form to create another. */
export function Libraries(): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [libraries, reload] = useResource("libraries", async () => {
    const { data, error } = await api.GET("/libraries");
    if (data === undefined) {
      throw new ApiProblem(error.error);
    }
    return data.libraries ?? [];
  });
  const actions = useLibraryActions(reload);
  const card = (library: Library): JSX.Element => (
    <LibraryCard
      key={library.id}
      library={library}
      note={actions.notes.get(library.id) ?? ""}
      onScan={() => {
        actions.scan(library.id);
      }}
      onApprove={(root) => {
        actions.answer(library.id, root, "/libraries/{id}/removals");
      }}
      onGone={(root) => {
        actions.answer(library.id, root, "/libraries/{id}/mounts/gone");
      }}
    />
  );
  return (
    <section aria-labelledby="libraries-heading">
      <h1 id="libraries-heading">{t("libraries.heading")}</h1>
      <Loaded
        resource={libraries}
        render={(value) =>
          value.length === 0 ? <p>{t("libraries.none")}</p> : <>{value.map(card)}</>
        }
      />
      <CreateLibrary onCreated={reload} />
    </section>
  );
}
