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

/** The ApiProblem in an error answer's body, or one naming only its status when it has none. */
export async function problemFromAnswer(answer: Response): Promise<ApiProblem> {
  try {
    const body: unknown = await answer.json();
    if (typeof body === "object" && body !== null && "error" in body && isProblemBody(body.error)) {
      return new ApiProblem(body.error);
    }
  } catch {
    // Not our error shape; the status says what there is.
  }
  return new ApiProblem({
    code: "internal",
    message: `the server answered ${String(answer.status)}`,
  });
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
}
