// SPDX-License-Identifier: MPL-2.0

import { ApiProblem } from "@orpheum/sdk";
import type { TFunction } from "i18next";

import { camelKey } from "./catalogue-key.ts";

// A refusal and its reading are the SDK's (cut-a §5.2); the app puts it in the viewer's words.
export { ApiProblem, dataOrThrow, throwIfRefused } from "@orpheum/sdk";

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
      return signedOutText(t, sessionEndReason(cause));
    }
    return t(`problem.${code}`, { defaultValue: t("problem.unknown", { message: cause.message }) });
  }
  if (cause instanceof TypeError) {
    return t("problem.unreachable"); // fetch's own failure: no answer at all
  }
  return t("problem.unknown", { message: cause instanceof Error ? cause.message : String(cause) });
}

/**
 * What to tell a person about an outside provider's refusal (provider_error), by the reason the
 * server gives ("no_key"), from the screen's own catalogue of them: `reasons` names it
 * ("identify.providerReason"), and its `unknown` says a reason this client does not know. Any
 * other failure, or a provider_error without a reason, is problemText's.
 */
export function providerProblemText(t: TFunction, cause: unknown, reasons: string): string {
  if (cause instanceof ApiProblem) {
    const reason = cause.withCode("provider_error")?.detail?.reason;
    if (reason !== undefined) {
      return t(`${reasons}.${camelKey(reason)}`, { defaultValue: t(`${reasons}.unknown`) });
    }
  }
  return problemText(t, cause);
}

/**
 * Why the server refused a session, by the reason its refusal gives ("session_ended"); a refusal
 * that gives none says only that there is no session ("no_session").
 */
export function sessionEndReason(problem: ApiProblem): string {
  return problem.withCode("auth_required")?.detail?.reason ?? "no_session";
}

/**
 * Why a session ended, in the person's words, by the reason the server gave ("session_ended");
 * a reason this client does not know says only that they are not signed in.
 */
export function signedOutText(t: TFunction, reason: string): string {
  return t(`problem.signedOut.${camelKey(reason)}`, {
    defaultValue: t("problem.signedOut.noSession"),
  });
}
