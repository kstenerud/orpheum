// SPDX-License-Identifier: MPL-2.0

import { ApiProblem } from "@orpheum/sdk";
import type { TFunction } from "i18next";

import { camelKey } from "./catalogue-key.ts";
import { dateTimeText } from "./date-time-text.ts";
import { i18n } from "./i18n.ts";

// A refusal and its reading are the SDK's (cut-a §5.2); the app puts it in the viewer's words.
export { ApiProblem, dataOrThrow, throwIfRefused } from "@orpheum/sdk";

/** Whether the server refused what was entered at location ("body.password"). */
export function isRefusedAt(cause: unknown, location: string): boolean {
  const problems =
    cause instanceof ApiProblem ? (cause.withCode("invalidRequest")?.detail?.problems ?? []) : [];
  return problems.some((problem) => problem.location === location);
}

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
    const unreachable = cause.withCode("mediaUnreachable")?.detail;
    if (unreachable !== undefined) {
      return unavailableText(t, unreachable);
    }
    return t(`problem.${code}`, { defaultValue: t("problem.unknown") });
  }
  if (cause instanceof TypeError) {
    return t("problem.unreachable"); // fetch's own failure: no answer at all
  }
  return t("problem.unknown");
}

/**
 * Why something cannot be played now, in the person's words: its machine away, or its folder or
 * disk missing, named, and since when when the server knows (D241 §3, §4). A play's refusal and a
 * listed title's mark say it alike. A reason this client does not know gets the general words.
 */
export function unavailableText(
  t: TFunction,
  detail: {
    readonly reason: string;
    readonly nodeName: string;
    readonly path?: string;
    readonly unavailableAt?: number;
  },
): string {
  const since =
    detail.unavailableAt === undefined
      ? ""
      : t("problem.sinceWhen", { since: dateTimeText(detail.unavailableAt, i18n.language) });
  return t(`problem.mediaReason.${camelKey(detail.reason)}`, {
    node: detail.nodeName,
    path: detail.path ?? "",
    since,
    defaultValue: t("problem.mediaUnreachable"),
  });
}

/** A form's words for a refusal: its fields' labels, and problemText's overrides. */
export interface FormWords {
  /**
   * Where the server says a problem is ("body.rootPaths", which also covers "body.rootPaths[0]"),
   * mapped to the label the person sees on that field.
   */
  readonly fields: Readonly<Record<string, string>>;
  readonly overrides?: Readonly<Partial<Record<string, string>>>;
  /**
   * The fields a refusal with a code is about, by where they are, keyed by the code in camelCase
   * ("authRequired" at sign-in is about the name and the password): marked as failing
   * (failedFields), as the fields an invalid_request names are.
   */
  readonly codeFields?: Readonly<Partial<Record<string, readonly string[]>>>;
}

/**
 * What to tell a person when the server refused what was entered (invalid_request), naming the
 * fields in the form's own words. When a problem is somewhere the form has no label for, the body
 * as a whole included, the words are problemText's, as for any other failure.
 */
export function fieldProblemText(t: TFunction, cause: unknown, words: FormWords): string {
  if (cause instanceof ApiProblem) {
    const named = namedFields(cause, words.fields);
    if (named.length > 0) {
      return t("problem.invalidFields", {
        fields: [...new Set(named.map((field) => words.fields[field]))],
      });
    }
  }
  return problemText(t, cause, words.overrides);
}

/**
 * The fields of a form a failure is about, by where they are ("body.name"): those an
 * invalid_request names, when fieldProblemText names them, and those words.codeFields gives for
 * the refusal's code. None for a failure that is not about what was typed: the network, a server
 * busy, a refusal of the request as a whole.
 */
export function failedFields(cause: unknown, words: FormWords): readonly string[] {
  if (!(cause instanceof ApiProblem)) {
    return [];
  }
  return [
    ...new Set([...namedFields(cause, words.fields), ...(words.codeFields?.[cause.code] ?? [])]),
  ];
}

/**
 * The fields an invalid_request names, each by where the form has it ("body.rootPaths" for
 * "body.rootPaths[0]"); none unless the form has a field for every problem, as only then do
 * fieldProblemText's words name them.
 */
function namedFields(cause: ApiProblem, fields: Readonly<Record<string, string>>): string[] {
  const problems = cause.withCode("invalidRequest")?.detail?.problems ?? [];
  const found = problems.map((problem) => fieldAt(fields, problem.location));
  return found.length > 0 && found.every((field): field is string => field !== undefined)
    ? found
    : [];
}

/** Where the form has the field a problem's location is in, or undefined where it has none. */
function fieldAt(fields: Readonly<Record<string, string>>, location: string): string | undefined {
  return Object.keys(fields).find(
    (path) =>
      location === path || location.startsWith(`${path}[`) || location.startsWith(`${path}.`),
  );
}

/**
 * What to tell a person about an outside provider's refusal (providerError), by the reason the
 * server gives ("noKey"), from the screen's own catalogue of them, which names its provider:
 * `reasons` names it ("identify.providerReason"). A reason the screen has no words for gets the
 * general words for it (problem.providerReason), and one this client does not know the screen's
 * `unknown`. Any other failure, or a providerError without a reason, is problemText's.
 */
export function providerProblemText(t: TFunction, cause: unknown, reasons: string): string {
  if (cause instanceof ApiProblem) {
    const detail = cause.withCode("providerError")?.detail;
    if (detail !== undefined) {
      // A host not answering says since when, and when it is next looked at (D231 §12).
      const when = {
        since:
          detail.notAnsweringAt === undefined
            ? ""
            : t("problem.sinceWhen", { since: dateTimeText(detail.notAnsweringAt, i18n.language) }),
        next:
          detail.nextCheckAt === undefined
            ? ""
            : t("problem.nextCheck", { at: dateTimeText(detail.nextCheckAt, i18n.language) }),
      };
      const key = camelKey(detail.reason) + (detail.notAnsweringAt === undefined ? "" : "Since");
      return t(`${reasons}.${key}`, {
        ...when,
        defaultValue: t(`problem.providerReason.${key}`, {
          ...when,
          defaultValue: t(`${reasons}.unknown`),
        }),
      });
    }
  }
  return problemText(t, cause);
}

/**
 * Why the server refused a session, by the reason its refusal gives ("sessionEnded"); a refusal
 * that gives none says only that there is no session ("noSession").
 */
export function sessionEndReason(problem: ApiProblem): string {
  return problem.withCode("authRequired")?.detail?.reason ?? "noSession";
}

/**
 * Why a session ended, in the person's words, by the reason the server gave ("sessionEnded");
 * a reason this client does not know says only that they are not signed in.
 */
export function signedOutText(t: TFunction, reason: string): string {
  return t(`problem.signedOut.${camelKey(reason)}`, {
    defaultValue: t("problem.signedOut.noSession"),
  });
}
