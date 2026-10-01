// SPDX-License-Identifier: MPL-2.0

import type { components } from "@orpheum/sdk";
import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { submitWith } from "./form-row.tsx";
import { dataOrThrow, fieldProblemText, throwIfRefused } from "./problem.ts";
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
    nodeName: "local",
    rootPaths: [],
  };
}

/** A node, and the roots it configured when it is connected. */
export type LibraryNode = components["schemas"]["NodeBody"];

/** A server setting, as /config lists it. */
type Setting = components["schemas"]["SettingBody"];

/**
 * The hours between checks a new library checked on an interval starts with: the server's
 * library_check_every_hours, so the form holds no default of its own (D200).
 */
export function checkEveryHoursToStart(settings: readonly Setting[]): number {
  const value = settings.find((setting) => setting.key === "library_check_every_hours")?.value;
  if (typeof value !== "number") {
    throw new Error("the server did not say library_check_every_hours");
  }
  return value;
}

/**
 * The draft with how changes are noticed chosen: an interval starts at hoursToStart unless hours
 * were already given, and any other choice takes none, since the server refuses hours without an
 * interval.
 */
export function withChangeDetection(
  draft: NewLibrary,
  changeDetection: NewLibrary["changeDetection"],
  hoursToStart: number,
): NewLibrary {
  const { checkEveryHours, ...rest } = draft;
  return changeDetection === "interval"
    ? { ...rest, changeDetection, checkEveryHours: checkEveryHours ?? hoursToStart }
    : { ...rest, changeDetection };
}

/** The roots chosen, with one ticked or unticked. */
export function withRootChosen(
  roots: readonly string[],
  root: string,
  isChosen: boolean,
): string[] {
  return isChosen ? [...new Set([...roots, root])] : roots.filter((chosen) => chosen !== root);
}

/** Whether the draft has a folder to read: the server refuses a library without one. */
export function hasARoot(draft: NewLibrary): boolean {
  return (draft.rootPaths ?? []).length > 0;
}

/** Where the library's folders are: a connected node, and which of its configured roots. */
function WhereItIs(props: {
  readonly draft: NewLibrary;
  readonly nodes: readonly LibraryNode[];
  readonly setAnswer: SetAnswer;
}): JSX.Element {
  const { t } = useTranslation();
  const { draft, nodes, setAnswer } = props;
  if (!nodes.some((node) => node.isConnected)) {
    return <p>{t("libraries.noNodes")}</p>;
  }
  const chosenNode = nodes.find((node) => node.name === draft.nodeName);
  const roots = draft.rootPaths ?? [];
  return (
    <>
      <label>
        {t("libraries.node")}
        <select
          value={draft.nodeName}
          onChange={(event) => {
            props.setAnswer("nodeName", event.target.value);
            props.setAnswer("rootPaths", []);
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
        <p>{t("libraries.rootsHelp")}</p>
        {(chosenNode?.rootPaths ?? []).map((root) => (
          <label key={root}>
            <input
              type="checkbox"
              checked={roots.includes(root)}
              onChange={(event) => {
                setAnswer("rootPaths", withRootChosen(roots, root, event.target.checked));
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
function WhatItHolds(props: {
  readonly draft: NewLibrary;
  readonly setAnswer: SetAnswer;
}): JSX.Element {
  const { t } = useTranslation();
  const { draft, setAnswer } = props;
  return (
    <>
      <label>
        {t("libraries.name")}
        <input
          required
          value={draft.name}
          onChange={(event) => {
            setAnswer("name", event.target.value);
          }}
        />
      </label>
      <Question
        legend={t("libraries.kind")}
        name="kind"
        value={draft.kind}
        onChange={(value) => {
          setAnswer("kind", value);
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
  readonly checkEveryHoursToStart: number;
  readonly setAnswer: SetAnswer;
  readonly onDraftChange: (draft: NewLibrary) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { draft, setAnswer } = props;
  return (
    <>
      <Question
        legend={t("libraries.metadataSource")}
        name="source"
        value={draft.metadataSource}
        onChange={(value) => {
          setAnswer("metadataSource", value);
        }}
        choices={[
          ["nfo_then_tmdb", t("libraries.sourceNfoThenTmdb")],
          ["tmdb", t("libraries.sourceTmdb")],
          ["nfo", t("libraries.sourceNfo")],
        ]}
      />
      <Question
        legend={t("libraries.changeDetection")}
        name="changes"
        value={draft.changeDetection}
        onChange={(value) => {
          props.onDraftChange(withChangeDetection(draft, value, props.checkEveryHoursToStart));
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
            value={draft.checkEveryHours ?? props.checkEveryHoursToStart}
            onChange={(event) => {
              setAnswer("checkEveryHours", Number(event.target.value));
            }}
          />
        </label>
      ) : null}
      <WritableChoice draft={draft} setAnswer={setAnswer} />
    </>
  );
}

/** Whether the server may write beside the media: read-only unless granted. */
function WritableChoice(props: {
  readonly draft: NewLibrary;
  readonly setAnswer: SetAnswer;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <>
      <label>
        <input
          type="checkbox"
          checked={props.draft.isWritable}
          onChange={(event) => {
            props.setAnswer("isWritable", event.target.checked);
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
  readonly nodes: readonly LibraryNode[];
  /** The server's library_check_every_hours, which an interval starts at. */
  readonly checkEveryHoursToStart: number;
  readonly problem: string;
  readonly onDraftChange: (draft: NewLibrary) => void;
  readonly onSubmit: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { draft } = props;
  const setAnswer: SetAnswer = (key, value) => {
    props.onDraftChange({ ...draft, [key]: value });
  };
  return (
    <form onSubmit={submitWith(props.onSubmit)} aria-labelledby="create-library-heading">
      <h2 id="create-library-heading">{t("libraries.createHeading")}</h2>
      <WhatItHolds draft={draft} setAnswer={setAnswer} />
      <ThreeQuestions
        draft={draft}
        checkEveryHoursToStart={props.checkEveryHoursToStart}
        setAnswer={setAnswer}
        onDraftChange={props.onDraftChange}
      />
      <WhereItIs draft={draft} nodes={props.nodes} setAnswer={setAnswer} />
      {props.problem === "" ? null : <p role="alert">{props.problem}</p>}
      <button type="submit" disabled={!hasARoot(draft)}>
        {t("libraries.create")}
      </button>
    </form>
  );
}

/** The create-library form, sent to the server; a created library is scanned at once. */
export function CreateLibrary(props: { readonly onCreated: () => void }): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [draft, setDraft] = useState(blankLibrary);
  const [problem, setProblem] = useState("");
  const [choices] = useResource("create-library", async () => {
    const [nodesAnswer, configAnswer] = await Promise.all([api.GET("/nodes"), api.GET("/config")]);
    return {
      nodes: dataOrThrow(nodesAnswer).nodes ?? [],
      checkEveryHoursToStart: checkEveryHoursToStart(dataOrThrow(configAnswer).settings ?? []),
    };
  });
  const submit = (): void => {
    setProblem("");
    api
      .POST("/libraries", { body: draft })
      .then(throwIfRefused)
      .then(
        () => {
          setDraft(blankLibrary());
          props.onCreated();
        },
        (cause: unknown) => {
          setProblem(
            fieldProblemText(t, cause, {
              fields: {
                "body.name": t("libraries.name"),
                "body.nodeName": t("libraries.node"),
                "body.rootPaths": t("libraries.roots"),
                "body.checkEveryHours": t("libraries.checkEvery"),
              },
            }),
          );
        },
      );
  };
  return (
    <Loaded
      resource={choices}
      render={(value) => (
        <CreateLibraryForm
          draft={draft}
          nodes={value.nodes}
          checkEveryHoursToStart={value.checkEveryHoursToStart}
          problem={problem}
          onDraftChange={setDraft}
          onSubmit={submit}
        />
      )}
    />
  );
}
