// SPDX-License-Identifier: MPL-2.0

import type { components } from "./gen/api.ts";

/** The server's error, as it sends it (cut-a §5.2). */
export type ErrorDetail = components["schemas"]["ErrorDetail"];

/**
 * A request the server refused, with its code and detail kept, so what the person is shown is
 * chosen by the code, in their language, rather than the server's English note for developers
 * (cut-a §5.2).
 */
export class ApiProblem extends Error {
  readonly code: string;
  readonly detail: Readonly<Record<string, string>>;

  constructor(error: ErrorDetail) {
    super(error.message);
    this.name = "ApiProblem";
    this.code = error.code;
    this.detail = error.detail ?? {};
  }
}

/** The answer's data, or the ApiProblem it refused with. */
export function dataOrThrow<T>(result: { data?: T; error?: { error: ErrorDetail } }): T {
  if (result.data === undefined) {
    throw problemOf(result.error);
  }
  return result.data;
}

/** Nothing, or the ApiProblem an answer with no data refused with. */
export function throwIfRefused(result: { error?: { error: ErrorDetail } }): void {
  if (result.error !== undefined) {
    throw problemOf(result.error);
  }
}

function problemOf(errorBody: { error: ErrorDetail } | undefined): Error {
  return errorBody === undefined
    ? new Error("the server did not answer")
    : new ApiProblem(errorBody.error);
}

/** The ApiProblem in an error answer's body, or one naming only its status when it has none. */
export async function problemFromAnswer(answer: Response): Promise<ApiProblem> {
  try {
    const body: unknown = await answer.json();
    if (typeof body === "object" && body !== null && "error" in body && isErrorDetail(body.error)) {
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

/** Whether a value is the server's error shape. */
export function isErrorDetail(value: unknown): value is ErrorDetail {
  return (
    typeof value === "object" &&
    value !== null &&
    "code" in value &&
    typeof value.code === "string" &&
    "message" in value &&
    typeof value.message === "string"
  );
}
