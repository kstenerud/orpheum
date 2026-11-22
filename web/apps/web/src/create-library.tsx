// SPDX-License-Identifier: MPL-2.0

import "./libraries-words.ts";

import type { components } from "@orpheum/sdk";
import { useState, type JSX, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { ChooseOpener } from "./file-browser-choose.tsx";
import { LibraryOffer } from "./library-offer.tsx";
import { blankLibrary, libraryToCreate, libraryWords, type NewLibrary } from "./library-draft.ts";
import { useRequest } from "./use-request.ts";
import { idempotent } from "./idempotent.ts";
import { submitWith } from "./form-row.tsx";
import { useFormFields, type FieldMark } from "./field.tsx";
import { dataOrThrow, failedFields, fieldProblemText } from "./problem.ts";
import { useReadLibrariesAgain } from "./libraries-reached.ts";
import { useServices } from "./services.tsx";
import { UpTrail } from "./up-trail.tsx";
import { adminLibrariesCrumb } from "./upward.ts";
import { Loaded } from "./loaded.tsx";
import { useResource } from "./use-resource.ts";
import { StatusNote } from "./status-note.tsx";

/** A node, and the roots it configured when it is connected. */
export type LibraryNode = components["schemas"]["NodeBody"];

/** A server setting, as /config lists it. */
type Setting = components["schemas"]["SettingBody"];

/**
 * The hours between checks a new library checked on an interval starts with: the server's
 * newLibraryDefaultScanEveryHours, so the form holds no default of its own (D200).
 */
export function checkEveryHoursToStart(settings: readonly Setting[]): number {
  const value = settings.find(
    (setting) => setting.key === "newLibraryDefaultScanEveryHours",
  )?.value;
  if (typeof value !== "number") {
    throw new Error("the server did not say newLibraryDefaultScanEveryHours");
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

/** Whether the draft has a folder to read: the server refuses a library without one. */
export function hasARoot(draft: NewLibrary): boolean {
  return (draft.rootPaths ?? []).length > 0;
}

/**
 * Where the library's folders are (D359): every connected machine's configured roots, each with its
 * machine beside it, and folders chosen inside one in Choose (D539 §2), which starts at the places
 * grouped by machine, so the form asks no machine of its own. A library reads one machine's
 * folders: ticking one on another machine starts its folders there. A machine not connected is said,
 * its folders not offered.
 */
/**
 * A library's folders once one on nodeName is ticked or unticked: on its machine, among the others
 * chosen there; on another machine, the library's folders start there, that one alone.
 */
function foldersTicked(
  draft: NewLibrary,
  nodeName: string,
  ticked: { readonly folder: string; readonly isTicked: boolean },
): string[] {
  if (nodeName === draft.nodeName) {
    return withRootChosen(draft.rootPaths ?? [], ticked.folder, ticked.isTicked);
  }
  return ticked.isTicked ? [ticked.folder] : [];
}

/** That each machine named is away, its folders not offered until it answers. */
function AwayNodes(props: { readonly names: readonly string[] }): JSX.Element {
  const { t } = useTranslation();
  return (
    <>
      {props.names.map((name) => (
        <p key={name} className="create-library__away">
          {t("libraries.nodeAway", { name })}
        </p>
      ))}
    </>
  );
}

function WhereItIs(props: {
  readonly draft: NewLibrary;
  readonly nodes: readonly LibraryNode[];
  /** Sets the machine and its folders together, as one change of the draft. */
  readonly setFolders: (nodeName: string, rootPaths: string[]) => void;
  readonly markOf: MarkOf;
}): JSX.Element {
  const { t } = useTranslation();
  const { draft, nodes, setFolders } = props;
  const connected = nodes.filter((node) => node.isConnected);
  if (connected.length === 0) {
    return <p>{t("libraries.noNodes")}</p>;
  }
  const roots = draft.rootPaths ?? [];
  const tick = (nodeName: string, folder: string, isTicked: boolean): void => {
    setFolders(nodeName, foldersTicked(draft, nodeName, { folder, isTicked }));
  };
  const places = connected.flatMap((node) =>
    (node.rootPaths ?? []).map((path) => ({ nodeName: node.name, path })),
  );
  const away = nodes.filter((node) => !node.isConnected).map((node) => node.name);
  return (
    <>
      <fieldset>
        <legend>{t("libraries.roots")}</legend>
        <p>{t("libraries.rootsHelp")}</p>
        {connected.map((node) => (
          <FolderTicks
            key={node.name}
            node={node}
            offered={foldersOffered(
              node.rootPaths ?? [],
              node.name === draft.nodeName ? roots : [],
            )}
            chosen={node.name === draft.nodeName ? roots : []}
            mark={props.markOf("body.rootPaths")}
            onTick={(folder, isTicked) => {
              tick(node.name, folder, isTicked);
            }}
          />
        ))}
        <AwayNodes names={away} />
      </fieldset>
      {places.length === 0 ? null : (
        <p>
          <ChooseOpener
            opener={t("libraries.browseInside")}
            heading={t("libraries.browseInsideHeading")}
            places={places}
            away={away}
            onChoose={(place) => {
              tick(place.nodeName, place.path, true);
            }}
          />
        </p>
      )}
    </>
  );
}

/**
 * A machine's folders offered, each ticked when chosen, and ticked or unticked by a press, its machine
 * named beside it; that it has offered none yet, said.
 */
function FolderTicks(props: {
  readonly node: LibraryNode;
  readonly offered: readonly string[];
  readonly chosen: readonly string[];
  /** The roots' mark while a refusal of them shows, on each, as a group cannot carry one. */
  readonly mark: FieldMark;
  readonly onTick: (folder: string, isTicked: boolean) => void;
}): JSX.Element {
  const { t } = useTranslation();
  if (props.offered.length === 0) {
    return <p>{t("libraries.noRootsYet", { name: props.node.name })}</p>;
  }
  return (
    <>
      {props.offered.map((folder) => (
        <label key={folder}>
          <input
            {...props.mark}
            type="checkbox"
            checked={props.chosen.includes(folder)}
            onChange={(event) => {
              props.onTick(folder, event.target.checked);
            }}
          />
          <code>{folder}</code> <span className="create-library__machine">{props.node.name}</span>
        </label>
      ))}
    </>
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

/** The mark of a field by where the server locates it ("body.name"), while a refusal of it shows. */
type MarkOf = (field: string) => FieldMark;

/** The library's name and what it holds. */
function WhatItHolds(props: {
  readonly draft: NewLibrary;
  readonly setAnswer: SetAnswer;
  readonly markOf: MarkOf;
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
          {...props.markOf("body.name")}
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
          ["film", t("libraries.kindMovies")],
          ["series", t("libraries.kindShows")],
          ["other", t("libraries.kindOther")],
        ]}
      />
    </>
  );
}

/** Where the library's information comes from: a choice for films and series, a note for others. */
function MetadataQuestion(props: {
  readonly draft: NewLibrary;
  readonly setAnswer: SetAnswer;
}): JSX.Element {
  const { t } = useTranslation();
  const { draft, setAnswer } = props;
  return draft.kind === "other" ? (
    <p>{t("libraries.sourceOther")}</p>
  ) : (
    <Question
      legend={t("libraries.metadataSource")}
      name="source"
      value={draft.metadataSource ?? "nfoThenTmdb"}
      onChange={(value) => {
        setAnswer("metadataSource", value);
      }}
      choices={[
        ["nfoThenTmdb", t("libraries.sourceNfoThenTmdb")],
        ["tmdb", t("libraries.sourceTmdb")],
        ["nfo", t("libraries.sourceNfo")],
      ]}
    />
  );
}

/** How changes in the folders are noticed, and, on an interval, how often (D184). */
function ScanningQuestions(props: {
  readonly draft: NewLibrary;
  readonly checkEveryHoursToStart: number;
  readonly setAnswer: SetAnswer;
  readonly onDraftChange: (draft: NewLibrary) => void;
  readonly markOf: MarkOf;
}): JSX.Element {
  const { t } = useTranslation();
  const { draft } = props;
  return (
    <>
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
            {...props.markOf("body.checkEveryHours")}
            onChange={(event) => {
              props.setAnswer("checkEveryHours", Number(event.target.value));
            }}
          />
        </label>
      ) : null}
    </>
  );
}

/** One group of the form under its heading (ui-audit-2026-11-04.md §3, Create a library). */
function FormGroup(props: {
  readonly id: string;
  readonly heading: string;
  readonly children: ReactNode;
}): JSX.Element {
  return (
    <section className="create-library__group" aria-labelledby={props.id}>
      <h2 id={props.id}>{props.heading}</h2>
      {props.children}
    </section>
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

/**
 * Creating a library asks three things, each stating its effect (D184): sources, changes, writing;
 * grouped as what it is, its source and its scanning, with Create and scan after the last, so Down
 * from the last field reaches it (ui-audit-2026-11-04.md §3, Create a library).
 */
export function CreateLibraryForm(props: {
  readonly draft: NewLibrary;
  readonly nodes: readonly LibraryNode[];
  /** The server's newLibraryDefaultScanEveryHours, which an interval starts at. */
  readonly checkEveryHoursToStart: number;
  readonly problem: string;
  /** What the last creating failed with, which marks the fields it is about. */
  readonly failure?: unknown;
  /** Whether the library is being created, which its button waits for. */
  readonly isCreating?: boolean | undefined;
  readonly onDraftChange: (draft: NewLibrary) => void;
  readonly onSubmit: () => void;
  /** Whether the form sits under a heading of its page's, so its own is a second-level one. */
  readonly isNested?: boolean;
}): JSX.Element {
  const { t } = useTranslation();
  const Heading = props.isNested === true ? "h2" : "h1";
  const draft = onAConnectedNode(props.draft, props.nodes);
  const marks = useFormFields(props.failure, failedFields(props.failure, libraryWords(t)));
  // An answer changed takes its field's mark away.
  const setAnswer: SetAnswer = (key, value) => {
    marks.editing(`body.${key}`, props.onDraftChange)({ ...draft, [key]: value });
  };
  return (
    <form onSubmit={submitWith(props.onSubmit)} aria-labelledby="create-library-heading">
      <Heading id="create-library-heading">{t("libraries.createHeading")}</Heading>
      <FormGroup id="create-library-what" heading={t("libraries.groupWhat")}>
        <WhatItHolds draft={draft} setAnswer={setAnswer} markOf={marks.markOf} />
      </FormGroup>
      <FormGroup id="create-library-source" heading={t("libraries.groupSource")}>
        <WhereItIs
          draft={draft}
          nodes={props.nodes}
          setFolders={(nodeName, rootPaths) => {
            marks.editing("body.rootPaths", props.onDraftChange)({ ...draft, nodeName, rootPaths });
          }}
          markOf={marks.markOf}
        />
        <MetadataQuestion draft={draft} setAnswer={setAnswer} />
        <WritableChoice draft={draft} setAnswer={setAnswer} />
      </FormGroup>
      <FormGroup id="create-library-scanning" heading={t("libraries.groupScanning")}>
        <ScanningQuestions
          draft={draft}
          checkEveryHoursToStart={props.checkEveryHoursToStart}
          setAnswer={setAnswer}
          onDraftChange={props.onDraftChange}
          markOf={marks.markOf}
        />
      </FormGroup>
      {props.problem === "" ? null : (
        <StatusNote told={{ kind: "problem", text: props.problem, id: marks.problemId }} />
      )}
      <button type="submit" disabled={!hasARoot(draft) || props.isCreating === true}>
        {t("libraries.create")}
      </button>
    </form>
  );
}

/**
 * Creating a library, a page of its own (D361 §4): the form, sent to the server, and its trail up to
 * the list (D367 §8). A created library is scanned at once, and its own page shows in this one's place, so Back
 * from there returns to the list, not to a form already sent.
 */
export function CreateLibraryPage(): JSX.Element {
  const { t } = useTranslation();
  const { navigation } = useServices();
  // The banner's Libraries menu lists the shell's reading, which nothing else tells of the new library.
  const readLibrariesAgain = useReadLibrariesAgain();
  return (
    <section className="libraries">
      <UpTrail upward={{ crumbs: [adminLibrariesCrumb(t)], here: t("libraries.createHeading") }} />
      <LibraryOffer
        onMade={() => {
          navigation.replace({ screen: "libraries" });
        }}
      />
      <CreateLibrary
        onCreated={(libraryId) => {
          readLibrariesAgain();
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
  /** Whether the form sits under a heading of its page's. */
  readonly isNested?: boolean;
}): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [draft, setDraft] = useState(blankLibrary);
  const creating = useRequest((cause) => fieldProblemText(t, cause, libraryWords(t)));
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
          failure={creating.failure}
          isCreating={creating.isBusy}
          onDraftChange={setDraft}
          onSubmit={submit}
          isNested={props.isNested === true}
        />
      )}
    />
  );
}
