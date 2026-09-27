// SPDX-License-Identifier: MPL-2.0

import type { components } from "@orpheum/sdk";
import type { TFunction } from "i18next";

import { camelKey } from "./catalogue-key.ts";

/** The server's error, as it sends it (cut-a §5.2). */
export type ErrorDetail = components["schemas"]["ErrorDetail"];

/**
 * A request the server refused, with its code and detail kept, so what the person is shown is
 * chosen here by the code, in their language, rather than the server's English note for
 * developers (cut-a §5.2).
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

function problemOf(error: { error: ErrorDetail } | undefined): Error {
  return error === undefined ? new Error("the server did not answer") : new ApiProblem(error.error);
}

/**
 * What to tell a person about a failure, in their language, with what they can do about it: by
 * the server's code, and for a session that ended, why; a screen that knows more about one code
 * says so in overrides, keyed by the code in camelCase ("authRequired"). A failure that is not the
 * server's (the network, the browser) says what it can.
 */
export function problemText(
  t: TFunction,
  cause: unknown,
  overrides: Readonly<Partial<Record<string, string>>> = {},
): string {
  if (cause instanceof ApiProblem) {
    const code = camelKey(cause.code);
    const override = overrides[code];
    if (override !== undefined) {
      return override;
    }
    if (code === "authRequired") {
      return t(`problem.signedOut.${camelKey(cause.detail["reason"] ?? "no_session")}`, {
        defaultValue: t("problem.signedOut.noSession"),
      });
    }
    return t(`problem.${code}`, { defaultValue: t("problem.unknown", { message: cause.message }) });
  }
  if (cause instanceof TypeError) {
    return t("problem.unreachable"); // fetch's own failure: no answer at all
  }
  return t("problem.unknown", { message: cause instanceof Error ? cause.message : String(cause) });
}
