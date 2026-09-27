// SPDX-License-Identifier: MPL-2.0

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
export type Answers = Readonly<Record<string, Answer | readonly Answer[] | typeof unreachable>>;

/** An answer that never comes: the request fails as fetch fails when the server cannot be reached. */
export const unreachable = "unreachable";

/** A request the stand-in server heard: "METHOD /path", and its body when it had one. */
export interface Heard {
  readonly asked: string;
  readonly body: unknown;
}

/**
 * Services whose API is answered by a stand-in server: each request from answers, and one it has
 * no answer for fails the test, so a screen cannot quietly ask for something the test did not
 * expect. Every request is added to heard. Tests never touch the network.
 */
export function servedBy(answers: Answers, heard: Heard[] = []): Services {
  const counts = new Map<string, number>();
  return testServices(undefined, "/films", async (request) => {
    const path = new URL(request.url).pathname.replace(/^\/api\/v1/u, "");
    const asked = `${request.method} ${path}`;
    const text = await request.text();
    heard.push({ asked, body: text === "" ? undefined : JSON.parse(text) });
    const given = answers[asked];
    if (given === unreachable) {
      throw new TypeError("Failed to fetch");
    }
    const answer = inTurn(given, counts.get(asked) ?? 0);
    counts.set(asked, (counts.get(asked) ?? 0) + 1);
    if (answer === undefined) {
      throw new Error(`the test has no answer for ${asked}`);
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

/** The answer for the turn'th time a request is asked: one answer always, or the next in turn. */
function inTurn(answer: Answer | readonly Answer[] | undefined, turn: number): Answer | undefined {
  if (answer === undefined || !isSeveral(answer)) {
    return answer;
  }
  return answer[Math.min(turn, answer.length - 1)];
}

function isSeveral(answer: Answer | readonly Answer[]): answer is readonly Answer[] {
  return Array.isArray(answer[0]);
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
    await answered();
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
    await answered();
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
    await answered();
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
    await answered();
  });
}

/** The button whose text is text, in container. */
export function button(container: HTMLElement, text: string): HTMLButtonElement | undefined {
  return [...container.querySelectorAll("button")].find((each) => each.textContent === text);
}

/** Lets pending answers and the renders they cause finish. */
async function answered(): Promise<void> {
  for (let turn = 0; turn < 5; turn += 1) {
    await new Promise((resolve) => setTimeout(resolve, 0));
  }
}
