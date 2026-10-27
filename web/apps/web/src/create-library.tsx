// SPDX-License-Identifier: MPL-2.0

import type { components } from "@orpheum/sdk";
import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { BackButton } from "./back-button.tsx";
import { FolderBrowser } from "./folder-picker.tsx";
import { useRequest } from "./use-request.ts";
import { idempotent } from "./idempotent.ts";
import { submitWith } from "./form-row.tsx";
import { dataOrThrow, fieldProblemText } from "./problem.ts";
import { useServices } from "./services.tsx";
import { Loaded } from "./loaded.tsx";
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
    // No machine yet: onAConnectedNode puts the first connected one here once they are known.
    nodeName: "",
    rootPaths: [],
  };
}

/** A node, and the roots it configured when it is connected. */
export type LibraryNode = components["schemas"]["NodeBody"];

/** A server setting, as /config lists it. */
type Setting = components["schemas"]["SettingBody"];

/**
 * The hours between checks a new library checked on an interval starts with: the server's
 * new_library_default_scan_every_hours, so the form holds no default of its own (D200).
 */
export function checkEveryHoursToStart(settings: readonly Setting[]): number {
  const value = settings.find(
    (setting) => setting.key === "new_library_default_scan_every_hours",
  )?.value;
  if (typeof value !== "number") {
    throw new Error("the server did not say new_library_default_scan_every_hours");
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

/**
 * The draft on a machine that is connected: its own when that one is, else the first connected one,
 * with no folders ticked. The form started on the dev loop's machine, "local", so on a server whose
 * machine had another name it listed no folders, while the list showed that machine as chosen
 * (staging, 2026-10-06).
 */
export function onAConnectedNode(draft: NewLibrary, nodes: readonly LibraryNode[]): NewLibrary {
  if (nodes.some((node) => node.isConnected && node.name === draft.nodeName)) {
    return draft;
  }
  const first = nodes.find((node) => node.isConnected);
  if (first === undefined) {
    return draft;
  }
  // Folders ticked on another machine are not this one's; with none chosen yet, there are none.
  return {
    ...draft,
    nodeName: first.name,
    rootPaths: draft.nodeName === "" ? draft.rootPaths : [],
  };
}

/**
 * The library the server is asked to create: the draft, but an other library is not asked where its
 * information comes from, as it takes NFO files only and is never looked up online (D349).
 */
export function libraryToCreate(draft: NewLibrary): NewLibrary {
  if (draft.kind !== "other") {
    return draft;
  }
  const rest: NewLibrary = { ...draft };
  delete rest.metadataSource;
  return rest;
}

/** Whether the draft has a folder to read: the server refuses a library without one. */
export function hasARoot(draft: NewLibrary): boolean {
  return (draft.rootPaths ?? []).length > 0;
}

/**
 * Where the library's folders are: a connected node, and which of its configured roots or folders
 * inside them (D359).
 */
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
        {chosenNode !== undefined && (chosenNode.rootPaths ?? []).length === 0 ? (
          <p>{t("libraries.noRootsYet", { name: chosenNode.name })}</p>
        ) : null}
        <FolderTicks
          offered={foldersOffered(chosenNode?.rootPaths ?? [], roots)}
          chosen={roots}
          onChange={(chosen) => {
            setAnswer("rootPaths", chosen);
          }}
        />
      </fieldset>
      {chosenNode === undefined ? null : (
        <BrowseInside
          key={chosenNode.name}
          node={chosenNode}
          onChoose={(path) => {
            setAnswer("rootPaths", withRootChosen(roots, path, true));
          }}
        />
      )}
    </>
  );
}

/** The folders offered, each ticked when chosen, and ticked or unticked by a press. */
function FolderTicks(props: {
  readonly offered: readonly string[];
  readonly chosen: readonly string[];
  readonly onChange: (chosen: string[]) => void;
}): JSX.Element {
  return (
    <>
      {props.offered.map((folder) => (
        <label key={folder}>
          <input
            type="checkbox"
            checked={props.chosen.includes(folder)}
            onChange={(event) => {
              props.onChange(withRootChosen(props.chosen, folder, event.target.checked));
            }}
          />
          <code>{folder}</code>
        </label>
      ))}
    </>
  );
}

/**
 * Choosing a folder inside one of the node's configured roots (D359 §1), in the folder browser's
 * dialog (D361 §3); nothing while it has none.
 */
function BrowseInside(props: {
  readonly node: LibraryNode;
  readonly onChoose: (path: string) => void;
}): JSX.Element | null {
  const { t } = useTranslation();
  const { node } = props;
  const starts = (node.rootPaths ?? []).map((path) => ({ nodeName: node.name, path }));
  if (starts.length === 0) {
    return null;
  }
  return (
    <p>
      <FolderBrowser
        opener={t("libraries.browseInside")}
        heading={t("libraries.browseInsideHeading")}
        starts={starts}
        onChoose={(place) => {
          props.onChoose(place.path);
        }}
      />
    </p>
  );
}

/**
 * The folders a new library's form offers to tick: the node's configured roots, then each folder
 * chosen inside one (D359), ticked, so it can be unticked.
 */
export function foldersOffered(
  configured: readonly string[],
  chosen: readonly string[],
): readonly string[] {
  return [...configured, ...chosen.filter((folder) => !configured.includes(folder))];
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
          ["other", t("libraries.kindOther")],
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
      {draft.kind === "other" ? (
        <p>{t("libraries.sourceOther")}</p>
      ) : (
        <Question
          legend={t("libraries.metadataSource")}
          name="source"
          value={draft.metadataSource ?? "nfo_then_tmdb"}
          onChange={(value) => {
            setAnswer("metadataSource", value);
          }}
          choices={[
            ["nfo_then_tmdb", t("libraries.sourceNfoThenTmdb")],
            ["tmdb", t("libraries.sourceTmdb")],
            ["nfo", t("libraries.sourceNfo")],
          ]}
        />
      )}
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
  /** The server's new_library_default_scan_every_hours, which an interval starts at. */
  readonly checkEveryHoursToStart: number;
  readonly problem: string;
  /** Whether the library is being created, which its button waits for. */
  readonly isCreating?: boolean | undefined;
  readonly onDraftChange: (draft: NewLibrary) => void;
  readonly onSubmit: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const draft = onAConnectedNode(props.draft, props.nodes);
  const setAnswer: SetAnswer = (key, value) => {
    props.onDraftChange({ ...draft, [key]: value });
  };
  return (
    <form onSubmit={submitWith(props.onSubmit)} aria-labelledby="create-library-heading">
      <h1 id="create-library-heading">{t("libraries.createHeading")}</h1>
      <WhatItHolds draft={draft} setAnswer={setAnswer} />
      <ThreeQuestions
        draft={draft}
        checkEveryHoursToStart={props.checkEveryHoursToStart}
        setAnswer={setAnswer}
        onDraftChange={props.onDraftChange}
      />
      <WhereItIs draft={draft} nodes={props.nodes} setAnswer={setAnswer} />
      {props.problem === "" ? null : <p role="alert">{props.problem}</p>}
      <button type="submit" disabled={!hasARoot(draft) || props.isCreating === true}>
        {t("libraries.create")}
      </button>
    </form>
  );
}

/**
 * Creating a library, a page of its own (D361 §4): the form, sent to the server, and Back to the
 * list. A created library is scanned at once, and its own page shows in this one's place, so Back
 * from there returns to the list, not to a form already sent.
 */
export function CreateLibraryPage(): JSX.Element {
  const { navigation } = useServices();
  return (
    <section className="libraries">
      <BackButton up={{ screen: "libraries" }} />
      <CreateLibrary
        onCreated={(libraryId) => {
          navigation.replace({ screen: "library", libraryId });
        }}
      />
    </section>
  );
}

/** The create-library form, sent to the server; a created library is scanned at once. */
export function CreateLibrary(props: {
  /** Told the new library's id once the server made it. */
  readonly onCreated: (libraryId: number) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [draft, setDraft] = useState(blankLibrary);
  const creating = useRequest((cause) =>
    fieldProblemText(t, cause, {
      fields: {
        "body.name": t("libraries.name"),
        "body.nodeName": t("libraries.node"),
        "body.rootPaths": t("libraries.roots"),
        "body.checkEveryHours": t("libraries.checkEvery"),
      },
    }),
  );
  const [choices] = useResource("create-library", async () => {
    const [nodesAnswer, configAnswer] = await Promise.all([api.GET("/nodes"), api.GET("/config")]);
    return {
      nodes: dataOrThrow(nodesAnswer).nodes ?? [],
      checkEveryHoursToStart: checkEveryHoursToStart(dataOrThrow(configAnswer).settings ?? []),
    };
  });
  const nodes = choices.state === "loaded" ? choices.value.nodes : [];
  const submit = (): void => {
    const body = libraryToCreate(onAConnectedNode(draft, nodes));
    creating.runRequest(async (idempotencyKey) => {
      const created = dataOrThrow(
        await api.POST("/libraries", { body, ...idempotent(idempotencyKey) }),
      );
      setDraft(blankLibrary());
      props.onCreated(created.id);
    });
  };
  return (
    <Loaded
      resource={choices}
      render={(value) => (
        <CreateLibraryForm
          draft={draft}
          nodes={value.nodes}
          checkEveryHoursToStart={value.checkEveryHoursToStart}
          problem={creating.problem ?? ""}
          isCreating={creating.isBusy}
          onDraftChange={setDraft}
          onSubmit={submit}
        />
      )}
    />
  );
}
