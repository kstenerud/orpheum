// SPDX-License-Identifier: MPL-2.0

/**
 * What the page's content-security policy refuses, or would refuse, to the core's log: the client's
 * pages are sent `Content-Security-Policy-Report-Only: require-trusted-types-for 'script'`
 * (internal/core/webclient), so each place a string reaches a script sink (innerHTML, a worker's
 * address, a service worker's) without a Trusted Types policy is found on the devices themselves,
 * where no one reads the console, before the policy is enforced (the 11-22 batch's item 6).
 */

import { noteForTheLog } from "@orpheum/sdk";

/**
 * The most places noted in one page's life: a sink in a loop says itself once, and a page that
 * meets many says the first few, enough to find them. A justified constant.
 */
const notedMost = 10;

/** The part of a violation said: what was refused, by which directive, and where. */
export type PolicyViolation = Pick<
  SecurityPolicyViolationEvent,
  "effectiveDirective" | "disposition" | "sample" | "sourceFile" | "lineNumber" | "columnNumber"
>;

/**
 * Notes each place the page's policy refuses, or in report-only would refuse, once a page, at most
 * notedMost, with note (noteForTheLog by default). Returns the stopping.
 */
export function notePolicyViolations(
  page: Pick<Document, "addEventListener" | "removeEventListener">,
  note: (line: string) => void = noteForTheLog,
): () => void {
  const noted = new Set<string>();
  const violated = (event: Event): void => {
    if (!isViolation(event)) {
      return;
    }
    const line = violationLine(event);
    if (noted.has(line) || noted.size >= notedMost) {
      return;
    }
    noted.add(line);
    note(line);
  };
  page.addEventListener("securitypolicyviolation", violated);
  return () => {
    page.removeEventListener("securitypolicyviolation", violated);
  };
}

/** Whether event is a policy's violation, as securitypolicyviolation's are. */
function isViolation(event: Event): event is SecurityPolicyViolationEvent {
  return "effectiveDirective" in event;
}

/**
 * A violation in words: `the policy would refuse (require-trusted-types-for): Element innerHTML|<b>
 * at /assets/app.js:1:200`, "refused" where it was enforced.
 */
export function violationLine(violation: PolicyViolation): string {
  const refusal = violation.disposition === "report" ? "would refuse" : "refused";
  const at =
    violation.sourceFile === ""
      ? ""
      : ` at ${pathOf(violation.sourceFile)}:${String(violation.lineNumber)}:${String(violation.columnNumber)}`;
  return `the policy ${refusal} (${violation.effectiveDirective}): ${violation.sample}${at}`;
}

/** An address's path alone, its origin being the page's; the address as it is where it is none. */
function pathOf(address: string): string {
  try {
    return new URL(address).pathname;
  } catch {
    return address;
  }
}
