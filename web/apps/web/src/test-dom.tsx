// SPDX-License-Identifier: MPL-2.0

import { apiPrefix } from "@orpheum/sdk";
import { act, type ReactNode } from "react";
import { createRoot } from "react-dom/client";

import type { Services } from "./services.tsx";
import { testServices } from "./test-services.tsx";

/** One answer of a stand-in server: a status and a body. */
export type Answer = readonly [number, unknown];

/**
 * What a stand-in server answers each request with, by "METHOD /path" with the path as the API
 * names it (no /api/v1): one answer, or several given in turn, the last repeated.
 */
export type Answers = Readonly<
  Record<string, Answer | readonly (Answer | typeof unanswered)[] | typeof unreachable>
>;

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
 * no answer for, or one outside the API's prefix, fails the test, so a screen cannot quietly ask
 * for something the test did not expect. Every request is added to heard. The session is signed
 * in with token when one is given. Tests never touch the network.
 */
export function servedBy(answers: Answers, heard: Heard[] = [], token?: string): Services {
  const counts = new Map<string, number>();
  return testServices(token, "/films", async (request) => {
    const url = new URL(request.url);
    if (!url.pathname.startsWith(apiPrefix)) {
      throw new Error(`the test's server answers only under ${apiPrefix}: ${url.pathname}`);
    }
    const path = url.pathname.slice(apiPrefix.length);
    const methodAndPath = `${request.method} ${path}`;
    const text = await request.text();
    const given = answers[methodAndPath];
    const answer =
      given === unreachable ? undefined : inTurn(given, counts.get(methodAndPath) ?? 0);
    heard.push(heardOf(request, text, answer === unanswered));
    if (given === unreachable) {
      throw new TypeError("Failed to fetch");
    }
    counts.set(methodAndPath, (counts.get(methodAndPath) ?? 0) + 1);
    if (answer === unanswered) {
      return await untilAborted(request.signal);
    }
    if (answer === undefined) {
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

/** A tree rendered into a document, and taking it down. */
export interface Mounted {
  readonly container: HTMLElement;
  readonly unmount: () => void;
}

/** Renders a tree into the document, and lets its first requests answer. */
export async function mount(tree: ReactNode): Promise<Mounted> {
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

/** The button whose text is text, in container. */
export function buttonWithText(
  container: HTMLElement,
  text: string,
): HTMLButtonElement | undefined {
  return [...container.querySelectorAll("button")].find((each) => each.textContent === text);
}

/** Lets pending answers and the renders they cause finish. */
/**
 * Lets work outside the page's requests settle until condition holds, for what waits on more than
 * answers (WebCrypto, which runs off the main thread and may take longer than a few turns on a busy
 * machine); it gives up after about two seconds, and the test's own expectation then says what was
 * missing.
 */
export async function settleUntil(condition: () => boolean): Promise<void> {
  for (let turn = 0; turn < 200 && !condition(); turn += 1) {
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 10));
    });
  }
}

async function settleAnswers(): Promise<void> {
  for (let turn = 0; turn < 5; turn += 1) {
    await new Promise((resolve) => setTimeout(resolve, 0));
  }
}
