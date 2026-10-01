// SPDX-License-Identifier: MPL-2.0

import { ApiProblem } from "@orpheum/sdk";
import type { TFunction } from "i18next";

import { camelKey } from "./catalogue-key.ts";

// A refusal and its reading are the SDK's (cut-a §5.2); the app puts it in the viewer's words.
export { ApiProblem, dataOrThrow, throwIfRefused } from "@orpheum/sdk";

/**
 * What to tell a person about a failure, in their language, with what they can do about it: by
 * the server's code, and for a session that ended, why; a screen that knows more about one code
 * says so in overrides, keyed by the code in camelCase ("authRequired"). A code this client does
 * not know, and a failure that is neither the server's nor the network's, get general words:
 * never the server's message or an error's own text (standards/typescript.md, Errors).
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
    return t(`problem.${code}`, { defaultValue: t("problem.unknown") });
  }
  if (cause instanceof TypeError) {
    return t("problem.unreachable"); // fetch's own failure: no answer at all
  }
  return t("problem.unknown");
}

/** A form's words for a refusal: its fields' labels, and problemText's overrides. */
export interface FormWords {
  /**
   * Where the server says a problem is ("body.rootPaths", which also covers "body.rootPaths[0]"),
   * mapped to the label the person sees on that field.
   */
  readonly fields: Readonly<Record<string, string>>;
  readonly overrides?: Readonly<Partial<Record<string, string>>>;
}

/**
 * What to tell a person when the server refused what was entered (invalid_request), naming the
 * fields in the form's own words. When a problem is somewhere the form has no label for, the body
 * as a whole included, the words are problemText's, as for any other failure.
 */
export function fieldProblemText(t: TFunction, cause: unknown, words: FormWords): string {
  if (cause instanceof ApiProblem) {
    const problems = cause.withCode("invalid_request")?.detail?.problems ?? [];
    const labels = problems.map((problem) => fieldLabel(words.fields, problem.location));
    if (labels.length > 0 && labels.every((label): label is string => label !== undefined)) {
      return t("problem.invalidFields", { fields: [...new Set(labels)] });
    }
  }
  return problemText(t, cause, words.overrides);
}

/** The label of the field a problem's location is in, or undefined where the form has none. */
function fieldLabel(
  fields: Readonly<Record<string, string>>,
  location: string,
): string | undefined {
  const found = Object.entries(fields).find(
    ([path]) =>
      location === path || location.startsWith(`${path}[`) || location.startsWith(`${path}.`),
  );
  return found?.[1];
}

/**
 * What to tell a person about an outside provider's refusal (provider_error), by the reason the
 * server gives ("no_key"), from the screen's own catalogue of them, which names its provider:
 * `reasons` names it ("identify.providerReason"). A reason the screen has no words for gets the
 * general words for it (problem.providerReason), and one this client does not know the screen's
 * `unknown`. Any other failure, or a provider_error without a reason, is problemText's.
 */
export function providerProblemText(t: TFunction, cause: unknown, reasons: string): string {
  if (cause instanceof ApiProblem) {
    const reason = cause.withCode("provider_error")?.detail?.reason;
    if (reason !== undefined) {
      return t(`${reasons}.${camelKey(reason)}`, {
        defaultValue: t(`problem.providerReason.${camelKey(reason)}`, {
          defaultValue: t(`${reasons}.unknown`),
        }),
      });
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
