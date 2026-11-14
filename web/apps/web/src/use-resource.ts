// SPDX-License-Identifier: MPL-2.0

import { reportFailure } from "@orpheum/sdk";
import { useCallback, useEffect, useRef, useState } from "react";

import type { AskOutcome, Poller } from "./polling.ts";
import { rememberedInTheTab } from "./remembered.ts";
import { askWhileUnanswered, isUnanswered } from "./reopen.ts";
import type { Resource } from "./resource.ts";
import { resetOnEachTestMount } from "./screen-load.tsx";

export type { Resource } from "./resource.ts";

/**
 * A value another read already holds, which stands for a resource's first read (D476): given, or
 * still waited on ("waiting"), when that read has not answered yet.
 */
export type Given<T> = { readonly value: T } | "waiting";

/** How a resource reads besides its first read: a poller to ask again by, and a value given. */
export interface ResourceOptions<T> {
  readonly poller?: Poller | undefined;
  readonly given?: Given<T> | undefined;
  /**
   * Whether each answer is its reader's alone, good once: a playback opened, a grant, a pairing. It
   * is neither kept for the tab nor shared with another reader of the key (D478).
   */
  readonly isSingleUse?: boolean | undefined;
}

/** An answer the tab holds for a key: read, or handed by the write that answered it (handAnswer). */
interface HeldAnswer {
  readonly value: unknown;
  readonly isHanded: boolean;
}

/** How a mounted reader of a key hears an answer another reader of it had: its value, as JSON. */
type Hear = (value: unknown, answer: string | undefined) => void;

/**
 * How many keys' answers the tab holds, and how many characters of JSON they may come to together:
 * a few megabytes, past which the oldest go (D478).
 */
const answersAtMost = 200;
const answerCharactersAtMost = 4_000_000;

/**
 * The latest answer for each key this tab read (D478): a reader of the key starting later shows it
 * at once and reads again behind it. Forgotten on sign-out (forgetEveryAnswer), and at each test's
 * mount, as a new tab would start.
 */
const answers = rememberedInTheTab<HeldAnswer>(answersAtMost, answerCharactersAtMost);

/** The read of each key under way, which a reader of the key starting now joins, asking nothing. */
const readsUnderway = new Map<string, Promise<unknown>>();

/** The mounted readers of each key, each told what any of them reads. */
const readersOf = new Map<string, Set<Hear>>();

/** Which sign-in the reads under way are of: one begun under an earlier one is kept by no one. */
let readsOf = 0;

function forgetReadsUnderway(): void {
  readsUnderway.clear();
  readsOf += 1;
}

resetOnEachTestMount(forgetReadsUnderway);

/** Forgets every answer the tab holds and every read under way, as a sign-out does (D478). */
export function forgetEveryAnswer(): void {
  answers.clear();
  forgetReadsUnderway();
}

/**
 * Hands key the answer a write just had for it, as POST /together answers the session's details:
 * the next reader of key to start shows it and asks nothing, a poller asking at its first wait; the
 * readers after it take it as any answer read (D478).
 */
export function handAnswer(key: string, value: unknown): void {
  keep(key, value, false);
}

/** Keeps value as key's answer, read apart from any reader of key, as a tray reads ahead for a page. */
export function rememberAnswer(key: string, value: unknown): void {
  keep(key, value, true);
}

/**
 * The answer the tab holds for key, read or handed, if any, as the type its readers load: one key
 * names one read (D478), so asking for it as another reader's type is asking for that read.
 */
// eslint-disable-next-line @typescript-eslint/no-unnecessary-type-parameters -- its caller names the type its key reads
export function heldAnswer<T = unknown>(key: string): T | undefined {
  const held = answers.get(key);
  return held === undefined ? undefined : asRead<T>(held.value);
}

/** An answer as the JSON a reader compares it by, to say whether it changed. */
function answerOf(value: unknown): string | undefined {
  return value === undefined ? undefined : JSON.stringify(value);
}

/**
 * The tab's answer for a key as its reader's type, the one its own load names: one key names one read
 * (D478), so every reader of a key loads the same type. The tab's answers are kept apart from their
 * readers, under any key, so no type can say this; this is the one place it is taken on trust.
 */
// eslint-disable-next-line @typescript-eslint/no-unnecessary-type-parameters -- its caller names the type its key reads
function asRead<T>(value: unknown): T {
  // eslint-disable-next-line @typescript-eslint/consistent-type-assertions -- one key names one read: each reader of a key loads one type (D478)
  return value as T;
}

/** A read failed with cause, saying whether a poller will ask again after it. */
function failedWith(cause: unknown, willAskAgain: boolean): Resource<never> {
  return { state: "failed", cause, ...(willAskAgain ? { willAskAgain: true } : {}) };
}

/** Keeps value as key's answer for the tab, read or handed, and tells every reader of key it. */
function keep(key: string, value: unknown, isRead: boolean): void {
  const answer = answerOf(value);
  answers.set(key, { value, isHanded: !isRead }, answer?.length ?? 0);
  for (const hear of readersOf.get(key) ?? []) {
    hear(value, answer);
  }
}

/**
 * Whether the answer a reader shows, lastAnswer, stays through a read that failed with cause: one its
 * WebRTC connection left unanswered (isUnanswered), while a poller asks again.
 */
function keepsAnswerThrough(
  reader: { readonly willAskAgain: boolean },
  lastAnswer: string | undefined,
  cause: unknown,
): boolean {
  return reader.willAskAgain && lastAnswer !== undefined && isUnanswered(cause);
}

/** Has hear told every answer a reader of key has, until the returned function is called. */
function listen(key: string, hear: Hear): () => void {
  const readers = readersOf.get(key) ?? new Set<Hear>();
  readersOf.set(key, readers);
  readers.add(hear);
  return () => {
    readers.delete(hear);
    if (readers.size === 0 && readersOf.get(key) === readers) {
      readersOf.delete(key);
    }
  };
}

/**
 * How a read of a key asks: joining the one under way, if any ("join", a reader's first); anew, for
 * readers starting after to join ("anew", a reload, which must not take an answer begun before it);
 * or its own ("own", a single-use answer's).
 */
type ReadHow = "join" | "anew" | "own";

/** One reader of one key: what it loads, how it shows what it has, and how it reads. */
interface Reader<T> {
  readonly key: string;
  readonly load: (signal?: AbortSignal) => Promise<T>;
  readonly show: (resource: Resource<T>) => void;
  readonly willAskAgain: boolean;
  readonly isSingleUse: boolean;
}

/**
 * What load reads, asked again at once on a new connection while the core answered nothing over
 * WebRTC or the connection went under it (askWhileUnanswered): a read cut off as a phone comes back
 * from another app is read again, never told as a failure this app does not expect (the owner's
 * Android phone, staging, 2026-11-14). A single-use answer is asked once: the core may have made it.
 */
async function askedOf<T>(reader: Reader<T>, signal: AbortSignal | undefined): Promise<T> {
  const { load } = reader;
  return reader.isSingleUse
    ? await load(signal)
    : await askWhileUnanswered(() => load(signal), reportFailure);
}

/** A read of the reader's key, as how says; a poll's (signal) is its own, so its give-up aborts no one else's. */
async function readOf<T>(
  reader: Reader<T>,
  asked: { readonly signal: AbortSignal | undefined; readonly how: ReadHow },
): Promise<unknown> {
  const { key } = reader;
  if (asked.how === "own" || asked.signal !== undefined) {
    return await askedOf(reader, asked.signal);
  }
  const underway = readsUnderway.get(key);
  if (asked.how === "join" && underway !== undefined) {
    return await underway;
  }
  const started = askedOf(reader, undefined);
  readsUnderway.set(key, started);
  const settled = (): void => {
    if (readsUnderway.get(key) === started) {
      readsUnderway.delete(key);
    }
  };
  void started.then(settled, settled);
  return await started;
}

/** Whether a resource was given a value (Given), is waiting for one, or neither. */
function givenStateOf<T>(given: Given<T> | undefined): "none" | "waiting" | "given" {
  if (given === undefined) {
    return "none";
  }
  return given === "waiting" ? "waiting" : "given";
}

/**
 * One reader's reads of one key: one read, a value given or held by the tab shown as its answer,
 * and a way to drop every answer.
 */
interface KeyReads<T> {
  /** One read; answered, when polling, hears what it came to. */
  readonly read: (
    answered?: (outcome: AskOutcome) => void,
    signal?: AbortSignal,
    how?: ReadHow,
  ) => void;
  readonly showGiven: (value: T) => void;
  readonly showHeld: (value: unknown) => void;
  readonly stop: () => void;
}

/**
 * The reads of one key by one reader, each shown as show says unless stopped since or its answer is
 * unchanged; what another reader of the key reads is shown too. A failure says willAskAgain where a
 * poller asks again. A read its poll gave up is dropped, its failure already reported, so the screen
 * keeps what it shows; so is one its connection left unanswered (isUnanswered) while an answer shows
 * and a poller asks again: Home, its rows drawn, went blank on the owner's phone back from another
 * app until the next poll's answer, some 7 s on, the read cut off as the connection the core had
 * ended went (staging, 2026-11-14).
 */
function keyReads<T>(reader: Reader<T>): KeyReads<T> {
  const { key, show, isSingleUse } = reader;
  let isCurrent = true;
  let lastAnswer: string | undefined;
  const hear: Hear = (value, answer) => {
    const isChanged = answer !== lastAnswer;
    lastAnswer = answer;
    if (isCurrent && isChanged) {
      show({ state: "loaded", value: asRead<T>(value) });
    }
  };
  const showAnswer = (value: unknown): void => {
    lastAnswer = answerOf(value);
    show({ state: "loaded", value: asRead<T>(value) });
  };
  const stopListening = isSingleUse ? undefined : listen(key, hear);
  const read = (answered?: (outcome: AskOutcome) => void, signal?: AbortSignal, how?: ReadHow) => {
    const startedUnder = readsOf;
    readOf(reader, { signal, how: isSingleUse ? "own" : (how ?? "anew") }).then(
      (value) => {
        if (signal?.aborted === true) {
          return;
        }
        const answer = answerOf(value);
        const isChanged = answer !== lastAnswer;
        if (isSingleUse || startedUnder !== readsOf) {
          hear(value, answer);
        } else {
          keep(key, value, true); // this reader hears it with the rest
        }
        answered?.(isChanged ? "changed" : "unchanged");
      },
      (cause: unknown) => {
        if (signal?.aborted !== true) {
          reportFailure(cause);
          const isKept = keepsAnswerThrough(reader, lastAnswer, cause);
          lastAnswer = isKept ? lastAnswer : undefined;
          if (isCurrent && !isKept) {
            show(failedWith(cause, reader.willAskAgain));
          }
          answered?.("failed");
        }
      },
    );
  };
  return {
    read,
    showGiven: (value) => {
      showAnswer(value);
      if (!isSingleUse) {
        keep(key, value, true);
      }
    },
    showHeld: showAnswer,
    stop: () => {
      isCurrent = false;
      stopListening?.();
    },
  };
}

/** The key and attempt a reader read, or was given a value, for. */
interface ReadFor {
  readonly key: string;
  readonly attempt: number;
}

/** Whether a read of key at attempt is a reload: another attempt at the key last read. */
function isReloadOf(last: ReadFor | undefined, key: string, attempt: number): boolean {
  return last?.key === key && last.attempt !== attempt;
}

/** Whether a given value stands for the read of key at attempt: the first, or that one again. */
function isGivenFor(last: ReadFor | undefined, key: string, attempt: number): boolean {
  return last === undefined || (last.key === key && last.attempt === attempt);
}

/**
 * A reader's first read of its key, given nothing: the tab's answer shown at once, if it holds one;
 * one a write handed asks nothing, taken, so the reader after reads; else loading, unless a poller
 * keeps what shows, and a read, joining one under way, or anew for a reload.
 */
function readFirst<T>(
  reads: KeyReads<T>,
  key: string,
  how: { readonly isReload: boolean; readonly isSingleUse: boolean; showLoading?: () => void },
): void {
  const tabs = how.isReload || how.isSingleUse ? undefined : answers.get(key);
  if (tabs?.isHanded === true) {
    reads.showHeld(tabs.value);
    keep(key, tabs.value, true);
    return;
  }
  if (tabs === undefined) {
    how.showLoading?.();
  } else {
    reads.showHeld(tabs.value);
  }
  reads.read(undefined, undefined, how.isReload ? "anew" : "join");
}

/** What a reader still loading shows before its effect runs: a value given, or the tab's answer. */
function shownBeforeItsRead<T>(
  key: string,
  before: {
    readonly given: { readonly value: T } | undefined;
    readonly isUnread: boolean;
    readonly isSingleUse: boolean;
  },
): Resource<T> {
  if (before.given !== undefined) {
    return { state: "loaded", value: before.given.value };
  }
  const tabs = before.isSingleUse || !before.isUnread ? undefined : answers.get(key);
  return tabs === undefined
    ? { state: "loading" }
    : { state: "loaded", value: asRead<T>(tabs.value) };
}

/**
 * Loads a value when the component mounts and whenever key changes, and reloads when the returned
 * function is called. An answer that arrives after the key changed is dropped. A poll's read hands
 * load the ask's signal, which aborts when the poll gives it up (askAnswersWithinMs).
 *
 * The answer is the tab's, by key (D478): a reader of a key the tab read before shows its latest
 * answer at once and reads again behind it; readers starting while a read of the key is under way
 * join it, one request for all; and every mounted reader of the key shows what any of them reads.
 * One key names one read, so two readers of it must load the same thing. An answer handed by a write
 * (handAnswer) stands for the first read, which asks nothing. A single-use answer (isSingleUse) is
 * its reader's alone. The tab forgets every answer on sign-out (forgetEveryAnswer).
 *
 * Given options' poller, it also asks again for as long as the component shows it, as the poller's
 * schedule says (D237): backgroundChangesPoller backs off while the answer is unchanged. It keeps what it shows meanwhile: a
 * reload does not go back to loading, so a view that follows something moving (the jobs, a scan's
 * films) does not flicker. A new key starts from loading, unless the tab holds an answer for it.
 * This is the one way the client polls the server; usePages polls a paged list the same way.
 *
 * Given options' given, a value another read holds, it shows that in place of its first read and
 * asks nothing then: a poller asks next at its first wait, and a reload reads. While that read is
 * waited on ("waiting"), it asks nothing and shows loading, or the tab's answer (D476: the shell's
 * one read hands each part on).
 */
export function useResource<T>(
  key: string,
  load: (signal?: AbortSignal) => Promise<T>,
  options: ResourceOptions<T> = {},
): [Resource<T>, () => void] {
  const { poller, given } = options;
  const isSingleUse = options.isSingleUse === true;
  const [held, setHeld] = useState<{ readonly key: string; readonly resource: Resource<T> }>({
    key,
    resource: { state: "loading" },
  });
  const [attempt, setAttempt] = useState(0);
  const givenState = givenStateOf(given);
  const givenValue = given === undefined || given === "waiting" ? undefined : given;
  const givenRef = useRef(givenValue);
  givenRef.current = givenValue;
  // The key and attempt the given value stood for: a reload, or a new key, reads.
  const givenFor = useRef<ReadFor | undefined>(undefined);
  // The key and attempt last read for: another attempt at the same key is a reload, which reads anew.
  const readFor = useRef<ReadFor | undefined>(undefined);
  useEffect(() => {
    if (givenState === "waiting") {
      return undefined;
    }
    const show = (resource: Resource<T>): void => {
      setHeld({ key, resource });
    };
    const reads = keyReads({ key, load, show, willAskAgain: poller !== undefined, isSingleUse });
    const isReload = isReloadOf(readFor.current, key, attempt);
    readFor.current = { key, attempt };
    if (isGivenFor(givenFor.current, key, attempt) && givenRef.current !== undefined) {
      givenFor.current = { key, attempt };
      reads.showGiven(givenRef.current.value);
    } else {
      const showLoading = (): void => {
        show({ state: "loading" });
      };
      readFirst(reads, key, { isReload, isSingleUse, ...(poller ? {} : { showLoading }) });
    }
    const stopPolling = poller?.start(reads.read);
    return () => {
      reads.stop();
      stopPolling?.();
    };
  }, [key, attempt, poller, givenState, isSingleUse]);
  const reload = useCallback(() => {
    setAttempt((count) => count + 1);
  }, []);
  const resource: Resource<T> = held.key === key ? held.resource : { state: "loading" };
  if (resource.state !== "loading") {
    return [resource, reload];
  }
  const before = givenFor.current === undefined ? givenValue : undefined;
  const isUnread = readFor.current?.key !== key;
  return [shownBeforeItsRead(key, { given: before, isUnread, isSingleUse }), reload];
}
