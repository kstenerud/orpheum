// SPDX-License-Identifier: MPL-2.0

import { apiPrefix } from "@orpheum/sdk";
import { act, type ReactNode } from "react";
import { createRoot } from "react-dom/client";

import type { Services } from "./services.tsx";
import { resetLazyScreensOnMount } from "./screen-load.tsx";
import { testServices } from "./test-services.tsx";
import { closeTrayDialog } from "./tray-dialog.ts";

/**
 * Requests a stand-in server made by `servedBy` had no answer for, or that reached outside the
 * API's prefix, across every stand-in server a test makes: recorded as they happen, read and
 * cleared by `failOnUnexpectedRequests`, so a screen cannot quietly ask for something the test did
 * not expect (test-architecture-2026-10-26.md, finding 2). `unreachable` stays the way a test
 * declares a request meant to fail; this is only for a request `servedBy` was never told about.
 */
let unexpectedRequests: string[] = [];

/**
 * Requests a `servedBy` is still working out an answer for right now, across every stand-in server
 * a test made, each keyed by a marker unique to that one request: `settleAnswers` waits for this to
 * empty. A request answered by `unanswered` is removed from this the moment it is known to be one
 * — it is deliberately left pending until its signal aborts, so waiting on it here would hang
 * `settleAnswers` forever.
 */
const inFlightRequests = new Map<symbol, string>();

/**
 * Fails with every request recorded in unexpectedRequests since the last call, naming them, and
 * clears the record either way. `unmount()` below calls this as every mounted test's own cleanup
 * already does; a test that never mounts (a few call `servedBy` for its `api` alone) calls this
 * itself in its own `afterEach`, since this file is production-shaped (dependency-cruiser's
 * `not-to-dev-dep`) and may not import vitest to register a global one. test-dom.test.tsx calls it
 * directly to prove the mechanism itself goes red when it should.
 */
export function failOnUnexpectedRequests(): void {
  const found = unexpectedRequests;
  unexpectedRequests = [];
  if (found.length > 0) {
    throw new Error(`a stand-in server (servedBy) had no answer for: ${found.join(", ")}`);
  }
}

/** One answer of a stand-in server: a status and a body. */
export type Answer = readonly [number, unknown];

/**
 * What a stand-in server answers each request with, by "METHOD /path" with the path as the API
 * names it (no /api/v1): one answer, several given in turn, the last repeated, or one chosen by the
 * request's query, as a list read whole asks its pages in an order no test fixes.
 */
export type Answers = Readonly<
  Record<
    string,
    Answer | readonly (Answer | typeof unanswered)[] | AnswerByQuery | typeof unreachable
  >
>;

/** An answer chosen by the request's query string. */
export type AnswerByQuery = (query: URLSearchParams) => Answer | typeof unanswered;

/** An answer that never comes: the request fails as fetch fails when the server cannot be reached. */
export const unreachable = "unreachable";

/**
 * An answer, in a list given in turn, that never comes and never fails: the request waits until its
 * signal aborts, and then fails with the signal's reason, as fetch does.
 */
export const unanswered = "unanswered";

/**
 * A request the stand-in server heard: "METHOD /path", its body when it had one, and its query
 * string ("?a=1&b=2") when it had one, so a test can check the names a screen asks by; for a
 * request given unanswered, its signal.
 */
export interface Heard {
  readonly methodAndPath: string;
  readonly body: unknown;
  readonly query?: string;
  /** The request's signal, kept for one answered by unanswered, so a test sees it given up. */
  readonly signal?: AbortSignal;
}

/** A heard request body's field, or undefined when the body has none by that name. */
export function bodyField(heard: Heard, name: string): unknown {
  return typeof heard.body === "object" && heard.body !== null
    ? Reflect.get(heard.body, name)
    : undefined;
}

/** Why the page asked for each playback it asked for, in order (watching.ts's StartCause). */
export function playbackCauses(heard: readonly Heard[]): unknown[] {
  return heard
    .filter((each) => each.methodAndPath === "POST /playback")
    .map((each) => bodyField(each, "cause"));
}

/**
 * Services whose API is answered by a stand-in server: each request from answers, and one it has
 * no answer for, or one outside the API's prefix, fails the test (`failOnUnexpectedRequests`,
 * called by `unmount()`, or by the test's own `afterEach` where it never mounts), so a screen
 * cannot quietly ask for something the test did not expect. Every request is added to heard. The
 * session is signed in with token when one is given. Tests never touch the network.
 */
export function servedBy(answers: Answers, heard: Heard[] = [], token?: string): Services {
  const counts = new Map<string, number>();
  return testServices(token, "/films", async (request) => {
    const url = new URL(request.url);
    if (!url.pathname.startsWith(apiPrefix)) {
      unexpectedRequests.push(`${request.method} ${url.pathname} (outside ${apiPrefix})`);
      throw new Error(`the test's server answers only under ${apiPrefix}: ${url.pathname}`);
    }
    const path = url.pathname.slice(apiPrefix.length);
    const methodAndPath = `${request.method} ${path}`;
    const marker = Symbol(methodAndPath);
    inFlightRequests.set(marker, methodAndPath);
    try {
      const text = await request.text();
      const given = answers[methodAndPath];
      const answer = answerTo(given, url.searchParams, counts.get(methodAndPath) ?? 0);
      heard.push(heardOf(request, text, answer === unanswered));
      if (given === unreachable) {
        throw new TypeError("Failed to fetch");
      }
      counts.set(methodAndPath, (counts.get(methodAndPath) ?? 0) + 1);
      if (answer === unanswered) {
        // Deliberately left pending until its signal aborts: not a wait settleAnswers should join.
        inFlightRequests.delete(marker);
        return await untilAborted(request.signal);
      }
      if (answer === undefined) {
        unexpectedRequests.push(methodAndPath);
        throw new Error(`the test has no answer for ${methodAndPath}`);
      }
      const [status, body] = answer;
      if (status === 204) {
        return new Response(null, { status });
      }
      return new Response(JSON.stringify(body), {
        status,
        headers: { "Content-Type": "application/json" },
      });
    } finally {
      inFlightRequests.delete(marker);
    }
  });
}

/** What the stand-in server heard of request, whose body is text; its signal when unanswered. */
function heardOf(request: Request, text: string, isUnanswered: boolean): Heard {
  const url = new URL(request.url);
  return {
    methodAndPath: `${request.method} ${url.pathname.slice(apiPrefix.length)}`,
    body: text === "" ? undefined : JSON.parse(text),
    ...(url.search === "" ? {} : { query: url.search }),
    ...(isUnanswered ? { signal: request.signal } : {}),
  };
}

/** A promise that fails with signal's reason once it aborts, as fetch does, and never otherwise. */
async function untilAborted(signal: AbortSignal): Promise<never> {
  return await new Promise<never>((_, fail) => {
    signal.addEventListener("abort", () => {
      const reason: unknown = signal.reason;
      fail(reason instanceof Error ? reason : new Error(String(reason)));
    });
  });
}

/** The answer for the turn'th time a request is asked: one answer always, or the next in turn. */
/** The answer given holds for a request with query, asked turn times before: none if unreachable. */
function answerTo(
  given: Answers[string] | undefined,
  query: URLSearchParams,
  turn: number,
): Answer | typeof unanswered | undefined {
  if (given === unreachable) {
    return undefined;
  }
  if (typeof given === "function") {
    return given(query);
  }
  return inTurn(given, turn);
}

function inTurn(
  answer: Answer | readonly (Answer | typeof unanswered)[] | undefined,
  turn: number,
): Answer | typeof unanswered | undefined {
  if (answer === undefined || !isSeveral(answer)) {
    return answer;
  }
  return answer[Math.min(turn, answer.length - 1)];
}

/** Whether answer is several in turn: an Answer's first element is its status, a number. */
function isSeveral(
  answer: Answer | readonly (Answer | typeof unanswered)[],
): answer is readonly (Answer | typeof unanswered)[] {
  return typeof answer[0] !== "number";
}

/** A server refusal, as cut-a §5.2 shapes it, with an English note no viewer should see. */
export function refusal(status: number, code: string): Answer {
  return [status, { error: { code, message: "a note for developers" } }];
}

/**
 * A tree rendered into a document, and taking it down. Taking it down also fails the test if any
 * request servedBy heard had no answer (`failOnUnexpectedRequests`): the one checkpoint every
 * mounted test already reaches at cleanup, since this file may not import vitest to register a
 * global `afterEach` (dependency-cruiser's `not-to-dev-dep`).
 */
export interface Mounted {
  readonly container: HTMLElement;
  readonly unmount: () => void;
}

/**
 * Renders a tree into the document, and lets its first requests answer. Each mount meets the
 * screens of later-screens.tsx cold (resetLazyScreensOnMount), so a test never passes only
 * because an earlier one in its file already warmed the same lazy-loaded screen or tray dialog
 * (tray-dialog-code.tsx), and with no tray dialog open, as an earlier one may have left one
 * (tray-dialog.ts).
 */
export async function mount(tree: ReactNode): Promise<Mounted> {
  resetLazyScreensOnMount();
  closeTrayDialog();
  Reflect.set(globalThis, "IS_REACT_ACT_ENVIRONMENT", true);
  const container = document.createElement("div");
  document.body.append(container);
  const root = createRoot(container);
  await act(async () => {
    root.render(tree);
    await settleAnswers();
  });
  return {
    container,
    unmount: () => {
      act(() => {
        root.unmount();
      });
      container.remove();
      failOnUnexpectedRequests();
    },
  };
}

/** Clicks an element, and lets the requests it made answer. */
export async function click(element: Element | null | undefined): Promise<void> {
  if (!(element instanceof HTMLElement)) {
    throw new Error("nothing to click");
  }
  await act(async () => {
    element.click();
    await settleAnswers();
  });
}

/**
 * Submits a form as the browser would after its checks passed: a required field left empty does
 * not stop it, so a test can show what the server's refusal does.
 */
export async function submit(form: Element | null | undefined): Promise<void> {
  if (!(form instanceof HTMLFormElement)) {
    throw new Error("no form to submit");
  }
  await act(async () => {
    form.dispatchEvent(new Event("submit", { bubbles: true, cancelable: true }));
    await settleAnswers();
  });
}

/** Fires event at an element, as the browser does, and lets the requests it made answer. */
export async function fire(element: Element | null | undefined, event: Event): Promise<void> {
  if (element === null || element === undefined) {
    throw new Error("nothing to fire at");
  }
  await act(async () => {
    element.dispatchEvent(event);
    await settleAnswers();
  });
}

/** Chooses value in a select, as a person does, and lets the requests it made answer. */
export async function choose(select: Element | null | undefined, value: string): Promise<void> {
  if (!(select instanceof HTMLSelectElement)) {
    throw new Error("no select to choose in");
  }
  await act(async () => {
    // eslint-disable-next-line no-param-reassign -- choosing is setting the select's value
    select.value = value;
    select.dispatchEvent(new Event("change", { bubbles: true }));
    await settleAnswers();
  });
}

/**
 * Chooses the choice whose text is text in the chooser (route-chooser.tsx) whose field is named field
 * in container, as a person does: opens its list and presses the choice; lets the requests it made
 * answer.
 */
export async function chooseIn(container: Element, field: string, text: string): Promise<void> {
  const chooser = [...container.querySelectorAll(".route-chooser")].find(
    (each) => each.querySelector(".route-chooser__field")?.textContent === field,
  );
  const opener = chooser?.querySelector<HTMLButtonElement>("button[aria-haspopup]");
  if (opener === null || opener === undefined) {
    throw new Error(`no chooser of ${field}`);
  }
  await click(opener);
  const choice = [...(chooser?.querySelectorAll("dialog .list-box button") ?? [])].find(
    (button) => button.textContent === text,
  );
  if (choice === undefined) {
    throw new Error(`no choice ${text} in the chooser of ${field}`);
  }
  await click(choice);
}

/**
 * Types text into an input, as a person does, and lets the requests it made answer. React reads an
 * input's value through the element's own setter, so the value is set through it.
 */
export async function enter(input: Element | null | undefined, text: string): Promise<void> {
  if (!(input instanceof HTMLInputElement)) {
    throw new Error("no input to type in");
  }
  await act(async () => {
    // The prototype's setter, with the input as its receiver: what a person's typing calls.
    Reflect.set(HTMLInputElement.prototype, "value", text, input);
    input.dispatchEvent(new Event("input", { bubbles: true }));
    await settleAnswers();
  });
}

/**
 * The button named name in container, as a screen reader names it: by its aria-label, else its
 * text. An icon button (icon-button.tsx) has no text, its name its label.
 */
export function buttonNamed(container: HTMLElement, name: string): HTMLButtonElement | undefined {
  return [...container.querySelectorAll("button")].find(
    (each) => (each.getAttribute("aria-label") ?? each.textContent) === name,
  );
}

/** The button whose text is text, in container. */
export function buttonWithText(
  container: HTMLElement,
  text: string,
): HTMLButtonElement | undefined {
  return [...container.querySelectorAll("button")].find((each) => each.textContent === text);
}

/** The first button whose text holds text, as a list's row holds several words. */
export function buttonContaining(
  container: HTMLElement,
  text: string,
): HTMLButtonElement | undefined {
  return [...container.querySelectorAll("button")].find((each) => each.textContent.includes(text));
}

/**
 * The text of each notice of what needs fixing told politely in container: an `<output>`, a polite
 * status, drawn in the warning yellow (needs-fixing, base.css), not an alert that breaks into what a
 * screen reader is reading.
 */
export function politeNotices(container: Element): string[] {
  return [...container.querySelectorAll(".needs-fixing output, output.needs-fixing")].map(
    (each) => each.textContent,
  );
}

/**
 * How long settleUntil and settleAnswers wait for real time before failing: a passing test never
 * reaches this, so it is set to the scale of Go's `testutil.ReceiveLimit` (a minute), not to a
 * guess at how long the work under test takes. A shorter bound that looked generous alone failed
 * under the gate's own load (test-architecture-2026-10-26.md, finding 3; the load reproduction of
 * 7655c7d0 at ~10x oversubscription).
 */
const SETTLE_FAILURE_LIMIT_MS = 60_000;

/**
 * Lets work outside the page's requests settle until condition holds, for what waits on more than
 * answers (WebCrypto, which runs off the main thread and may take longer than a few turns on a busy
 * machine). It fails the test after about a minute, naming the condition that never held: a wait
 * that gave up silently let a test pass that never saw what it waited for (account.test.tsx held a
 * checkbox the page had replaced, the audit of 2026-10-25, W11).
 */
export async function settleUntil(condition: () => boolean): Promise<void> {
  const start = Date.now();
  while (!condition() && Date.now() - start < SETTLE_FAILURE_LIMIT_MS) {
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 10));
    });
  }
  if (!condition()) {
    throw new Error(
      `settleUntil: this never held within about ${String(SETTLE_FAILURE_LIMIT_MS / 1000)} s: ${condition.toString()}`,
    );
  }
}

/**
 * How many consecutive quiet ticks (no request in flight, no DOM mutation) settleAnswers needs
 * before it calls the page settled. More than one: a response's handler can itself start another
 * request or render in the same tick its predecessor cleared.
 */
const QUIET_TICKS_NEEDED = 3;

/**
 * Lets pending answers and the renders they cause finish: waits until no request servedBy is
 * holding is in flight and the DOM has stopped changing, for QUIET_TICKS_NEEDED ticks running, so
 * it settles as soon as the page does rather than after a fixed guess at how many ticks that takes
 * (test-architecture-2026-10-26.md, finding 3). A page that never quiets fails after about a
 * minute, naming what was still in flight.
 */
export async function settleAnswers(): Promise<void> {
  const start = Date.now();
  let hasMutated: boolean;
  const observer = new MutationObserver(() => {
    hasMutated = true;
  });
  observer.observe(document.body, {
    childList: true,
    subtree: true,
    attributes: true,
    characterData: true,
  });
  try {
    let quietTicks = 0;
    while (quietTicks < QUIET_TICKS_NEEDED) {
      if (Date.now() - start >= SETTLE_FAILURE_LIMIT_MS) {
        const inFlight = [...inFlightRequests.values()];
        throw new Error(
          `settleAnswers: still waiting after about ${String(SETTLE_FAILURE_LIMIT_MS / 1000)} s: ` +
            (inFlight.length > 0 ? `in flight: ${inFlight.join(", ")}` : "the DOM kept changing"),
        );
      }
      hasMutated = false;
      // A positive delay, not 0: a chain of same-instant timers never advances a faked clock, so a
      // test driving this with vi.advanceTimersByTimeAsync would spin forever rather than time out.
      await new Promise((resolve) => setTimeout(resolve, 5));
      if (observer.takeRecords().length > 0) {
        hasMutated = true;
      }
      quietTicks = inFlightRequests.size === 0 && !hasMutated ? quietTicks + 1 : 0;
    }
  } finally {
    observer.disconnect();
  }
}
