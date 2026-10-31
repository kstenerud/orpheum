// SPDX-License-Identifier: MPL-2.0

import type { components } from "./gen/api.ts";

/**
 * The server's error, as it sends it (cut-a §5.2): one form per code, so narrowing by the code
 * types the detail that code carries.
 */
export type ProblemBody = components["schemas"]["ProblemBody"];

/** Every code the server's API document names. */
export type ProblemCode = ProblemBody["code"];

/** The form of ProblemBody one code has, with that code's detail. */
export type ProblemWithCode<C extends ProblemCode> = Extract<ProblemBody, { code: C }>;

/**
 * An error in the server's shape whose code this client may not know: a newer server's code, or
 * a body read before its code could be checked.
 */
export interface UnknownProblemBody {
  code: string;
  message: string;
}

/**
 * A request the server refused, with its code and detail kept, so what the person is shown is
 * chosen by the code, in their language, rather than the server's English note for developers
 * (cut-a §5.2).
 */
export class ApiProblem extends Error {
  readonly code: string;
  readonly #body: ProblemBody | UnknownProblemBody;

  constructor(body: ProblemBody | UnknownProblemBody) {
    super(body.message);
    this.name = "ApiProblem";
    this.code = body.code;
    this.#body = body;
  }

  /** The problem as the form its code has, with that code's typed detail; undefined for another. */
  withCode<C extends ProblemCode>(code: C): ProblemWithCode<C> | undefined {
    const body = this.#body;
    return hasCode(body, code) ? body : undefined;
  }
}

function hasCode<C extends ProblemCode>(
  body: ProblemBody | UnknownProblemBody,
  code: C,
): body is ProblemWithCode<C> {
  return body.code === code;
}

/** The answer's data, or the ApiProblem it refused with. */
export function dataOrThrow<T>(result: { data?: T; error?: { error: ProblemBody } }): T {
  if (result.data === undefined) {
    throw problemOf(result.error);
  }
  return result.data;
}

/** Nothing, or the ApiProblem an answer with no data refused with. */
export function throwIfRefused(result: { error?: { error: ProblemBody } }): void {
  if (result.error !== undefined) {
    throw problemOf(result.error);
  }
}

function problemOf(errorBody: { error: ProblemBody } | undefined): Error {
  return errorBody === undefined
    ? new Error("the server did not answer")
    : new ApiProblem(errorBody.error);
}

/**
 * The codes this client gives an error answer that is not in the server's shape, which the server
 * never sends: something between the client and the server answered instead. not_answering is a
 * gateway's 502, 503 or 504, which a proxy in front of a server that is down or restarting
 * answers (Vite's dev proxy among them); unreadable_answer is any other, which nothing expected
 * sends, so its words are the general ones and the console says what came.
 */
export const notAnsweringCode = "not_answering";
export const unreadableAnswerCode = "unreadable_answer";

/** The problem for an error answer with status that is not in the server's shape. */
export function foreignAnswerProblem(status: number): UnknownProblemBody {
  const isGateway = status === 502 || status === 503 || status === 504;
  return {
    code: isGateway ? notAnsweringCode : unreadableAnswerCode,
    message: `something other than the server answered ${String(status)}`,
  };
}

/** The server's error in an answer's body, or undefined when the body is not in its shape. */
export async function serversProblemIn(answer: Response): Promise<UnknownProblemBody | undefined> {
  try {
    const body: unknown = await answer.json();
    if (typeof body === "object" && body !== null && "error" in body && isProblemBody(body.error)) {
      return body.error;
    }
  } catch {
    // Not JSON: not the server's shape.
  }
  return undefined;
}

/** The ApiProblem in an error answer's body, or foreignAnswerProblem's when it is not the server's. */
export async function problemFromAnswer(answer: Response): Promise<ApiProblem> {
  return new ApiProblem((await serversProblemIn(answer)) ?? foreignAnswerProblem(answer.status));
}

/** Whether a value is the server's error shape: a code and a message. */
export function isProblemBody(value: unknown): value is UnknownProblemBody {
  return (
    typeof value === "object" &&
    value !== null &&
    "code" in value &&
    typeof value.code === "string" &&
    "message" in value &&
    typeof value.message === "string"
  );
}

/**
 * Writes a failure to the browser's console with its cause. A person is shown only words chosen
 * by an ApiProblem's code, and general words for anything else, so the console is where what
 * actually failed can be read: an empty answer, a reply that is not JSON, a request cut off.
 */
export function reportFailure(cause: unknown): void {
  // eslint-disable-next-line no-console -- this is the reporter the console is for (standards/typescript.md)
  console.error("Orpheum: a request failed", cause);
  sendOn(failureText(cause), "failure");
}

/** The longest line the core's log takes from a page (POST /client-problems' message). */
const textMost = 500;

/**
 * Notes a line for the core's log at its debug level, beside the console's: what the page did, not
 * what failed, so a rare misbehaviour on a device no one watches explains itself the next time it
 * comes (a Fire TV's sign-in that submitted on a checkbox's OK, the owner, 2026-10-30). It goes as a
 * failure goes (sendFailuresTo), within the same bound.
 */
export function noteForTheLog(text: string): void {
  // eslint-disable-next-line no-console -- the console is this note's first reader (standards/typescript.md)
  console.debug("Orpheum:", text);
  sendOn(text.slice(0, textMost), "debug");
}

/** How a line sent to the core's log is logged there: a failure, or a note at the debug level. */
export type SentLevel = "failure" | "debug";

/** Sends text on as level, or holds it until there is somewhere to send it, within the bound. */
function sendOn(text: string, level: SentLevel): void {
  if (failureSink.send === undefined) {
    if (failureSink.held.length < failureSink.most) {
      failureSink.held.push({ text, level });
    }
  } else if (failureSink.sent < failureSink.most) {
    failureSink.sent += 1;
    failureSink.send(text, level);
  }
}

/** How many failures one page load sends on at most: a few say what went wrong, more only flood. */
export const failuresSentMost = 20;

/**
 * Where failures go beside the console, how many this page load sent, and those met before there was
 * anywhere to send them: a sign-in's own connections are made before it, and one that took half a
 * minute said so nowhere (staging, 2026-10-23).
 */
const failureSink: {
  send?: (text: string, level: SentLevel) => void;
  sent: number;
  most: number;
  held: { readonly text: string; readonly level: SentLevel }[];
} = {
  sent: 0,
  most: failuresSentMost,
  held: [],
};

/**
 * Sends each failure reportFailure is given, and each note noteForTheLog is, on to send as well as
 * to the console, with its level, at most failuresSentMost of them for the page's life: on a phone no
 * one reads the console, and a playback that started over by itself left nothing to say why
 * (staging, 2026-10-14). send must not report its own failure, or one failure would send another.
 */
export function sendFailuresTo(send: (text: string, level: SentLevel) => void): void {
  failureSink.send = send;
  failureSink.sent = 0;
  for (const { text, level } of failureSink.held.splice(0)) {
    failureSink.sent += 1;
    send(text, level);
  }
}

/** A failure as a line of text, its kind and its words, at most 500 characters. */
export function failureText(cause: unknown): string {
  let text: string;
  if (cause instanceof ApiProblem) {
    text = `${cause.code}: ${cause.message}`;
  } else if (cause instanceof Error) {
    text = `${cause.name}: ${cause.message}`;
  } else if (typeof cause === "object" && cause !== null) {
    text = JSON.stringify(cause);
  } else {
    // A string, or what JSON cannot say: undefined, a function, a symbol.
    text = String(cause);
  }
  return text.slice(0, textMost);
}
