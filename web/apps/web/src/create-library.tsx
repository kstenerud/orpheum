// SPDX-License-Identifier: MPL-2.0

import type { components } from "@orpheum/sdk";
import { useState, type JSX, type SyntheticEvent } from "react";
import { useTranslation } from "react-i18next";

import { ApiProblem } from "./problem.ts";
import { useServices } from "./services.tsx";
import { Loaded } from "./title.tsx";
import { useResource } from "./use-resource.ts";

/** A new library, as the server takes it. */
export type NewLibrary = components["schemas"]["CreateLibraryInputBody"];

/** The form's starting answers: read-only, told when to scan, sources as a person would expect. */
export function blankLibrary(): NewLibrary {
  return {
    name: "",
    kind: "movies",
    metadataSource: "nfo_then_tmdb",
    changeDetection: "manual",
    isWritable: false,
    node: "local",
    roots: [],
  };
}

/** A node, and the roots it configured when it is connected. */
export type NodeRoots = components["schemas"]["NodeBody"];

/** The roots chosen, with one ticked or unticked. */
export function toggled(roots: readonly string[], root: string, isChosen: boolean): string[] {
  return isChosen ? [...new Set([...roots, root])] : roots.filter((chosen) => chosen !== root);
}

/** Where the library's folders are: a connected node, and which of its configured roots. */
function WhereItIs(props: {
  readonly draft: NewLibrary;
  readonly nodes: readonly NodeRoots[];
  readonly set: SetAnswer;
}): JSX.Element {
  const { t } = useTranslation();
  const { draft, nodes, set } = props;
  if (!nodes.some((node) => node.isConnected)) {
    return <p>{t("libraries.noNodes")}</p>;
  }
  const chosen = nodes.find((node) => node.name === draft.node);
  const roots = draft.roots ?? [];
  return (
    <>
      <label>
        {t("libraries.node")}
        <select
          value={draft.node}
          onChange={(event) => {
            props.set("node", event.target.value);
            props.set("roots", []);
          }}
        >
          {nodes.map((node) => (
            <option key={node.name} value={node.name} disabled={!node.isConnected}>
              {node.isConnected ? node.name : t("libraries.nodeAway", { name: node.name })}
            </option>
          ))}
        </select>
      </label>
      <fieldset>
        <legend>{t("libraries.roots")}</legend>
        {(chosen?.roots ?? []).map((root) => (
          <label key={root}>
            <input
              type="checkbox"
              checked={roots.includes(root)}
              onChange={(event) => {
                set("roots", toggled(roots, root, event.target.checked));
              }}
            />
            <code>{root}</code>
          </label>
        ))}
      </fieldset>
    </>
  );
}

/** One question with its answers, as radio buttons, each saying what it means (D184). */
function Question<T extends string>(props: {
  readonly legend: string;
  readonly name: string;
  readonly value: T;
  readonly choices: readonly (readonly [T, string])[];
  readonly onChange: (value: T) => void;
}): JSX.Element {
  return (
    <fieldset>
      <legend>{props.legend}</legend>
      {props.choices.map(([value, label]) => (
        <label key={value}>
          <input
            type="radio"
            name={props.name}
            value={value}
            checked={props.value === value}
            onChange={() => {
              props.onChange(value);
            }}
          />
          {label}
        </label>
      ))}
    </fieldset>
  );
}

/** Changes one answer of a draft. */
type SetAnswer = <K extends keyof NewLibrary>(key: K, value: NewLibrary[K]) => void;

/** The library's name and what it holds. */
function WhatItHolds(props: { readonly draft: NewLibrary; readonly set: SetAnswer }): JSX.Element {
  const { t } = useTranslation();
  const { draft, set } = props;
  return (
    <>
      <label>
        {t("libraries.name")}
        <input
          required
          value={draft.name}
          onChange={(event) => {
            set("name", event.target.value);
          }}
        />
      </label>
      <Question
        legend={t("libraries.kind")}
        name="kind"
        value={draft.kind}
        onChange={(value) => {
          set("kind", value);
        }}
        choices={[
          ["movies", t("libraries.kindMovies")],
          ["shows", t("libraries.kindShows")],
          ["mixed", t("libraries.kindMixed")],
        ]}
      />
    </>
  );
}

/** The three questions, each answer saying what it means (D184). */
function ThreeQuestions(props: {
  readonly draft: NewLibrary;
  readonly set: SetAnswer;
}): JSX.Element {
  const { t } = useTranslation();
  const { draft, set } = props;
  return (
    <>
      <Question
        legend={t("libraries.source")}
        name="source"
        value={draft.metadataSource}
        onChange={(value) => {
          set("metadataSource", value);
        }}
        choices={[
          ["nfo_then_tmdb", t("libraries.sourceNfoThenTmdb")],
          ["tmdb", t("libraries.sourceTmdb")],
          ["nfo", t("libraries.sourceNfo")],
        ]}
      />
      <Question
        legend={t("libraries.changes")}
        name="changes"
        value={draft.changeDetection}
        onChange={(value) => {
          set("changeDetection", value);
        }}
        choices={[
          ["watch", t("libraries.changesWatch")],
          ["interval", t("libraries.changesInterval")],
          ["manual", t("libraries.changesManual")],
        ]}
      />
      {draft.changeDetection === "interval" ? (
        <label>
          {t("libraries.checkEvery")}
          <input
            type="number"
            min={1}
            required
            value={draft.checkEveryH ?? 24}
            onChange={(event) => {
              set("checkEveryH", Number(event.target.value));
            }}
          />
        </label>
      ) : null}
      <Writing draft={draft} set={set} />
    </>
  );
}

/** Whether the server may write beside the media: read-only unless granted. */
function Writing(props: { readonly draft: NewLibrary; readonly set: SetAnswer }): JSX.Element {
  const { t } = useTranslation();
  return (
    <>
      <label>
        <input
          type="checkbox"
          checked={props.draft.isWritable}
          onChange={(event) => {
            props.set("isWritable", event.target.checked);
          }}
        />
        {t("libraries.writable")}
      </label>
      <p>{t("libraries.writableHelp")}</p>
    </>
  );
}

/** Creating a library asks three things, each stating its effect (D184): sources, changes, writing. */
export function CreateLibraryForm(props: {
  readonly draft: NewLibrary;
  readonly nodes: readonly NodeRoots[];
  readonly problem: string;
  readonly onDraft: (draft: NewLibrary) => void;
  readonly onSubmit: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { draft } = props;
  const set: SetAnswer = (key, value) => {
    props.onDraft({ ...draft, [key]: value });
  };
  const submit = (event: SyntheticEvent): void => {
    event.preventDefault();
    props.onSubmit();
  };
  return (
    <form onSubmit={submit} aria-labelledby="create-library-heading">
      <h2 id="create-library-heading">{t("libraries.createHeading")}</h2>
      <WhatItHolds draft={draft} set={set} />
      <ThreeQuestions draft={draft} set={set} />
      <WhereItIs draft={draft} nodes={props.nodes} set={set} />
      {props.problem === "" ? null : (
        <p role="alert">{t("common.failed", { message: props.problem })}</p>
      )}
      <button type="submit">{t("libraries.create")}</button>
    </form>
  );
}

/** The create-library form, sent to the server; a created library is scanned at once. */
export function CreateLibrary(props: { readonly onCreated: () => void }): JSX.Element {
  const { api } = useServices();
  const [draft, setDraft] = useState(blankLibrary);
  const [problem, setProblem] = useState("");
  const [nodes] = useResource("nodes", async () => {
    const { data, error } = await api.GET("/nodes");
    if (data === undefined) {
      throw new ApiProblem(error.error);
    }
    return data.nodes ?? [];
  });
  const submit = (): void => {
    setProblem("");
    void api.POST("/libraries", { body: draft }).then(({ error }) => {
      if (error === undefined) {
        setDraft(blankLibrary());
        props.onCreated();
      } else {
        setProblem(error.error.message);
      }
    });
  };
  return (
    <Loaded
      resource={nodes}
      render={(value) => (
        <CreateLibraryForm
          draft={draft}
          nodes={value}
          problem={problem}
          onDraft={setDraft}
          onSubmit={submit}
        />
      )}
    />
  );
}
