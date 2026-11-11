// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { notePolicyViolations, violationLine, type PolicyViolation } from "./policy-violations.ts";

/** A violation of the Trusted Types policy as a browser reports it, report-only. */
const innerHtml: PolicyViolation = {
  effectiveDirective: "require-trusted-types-for",
  disposition: "report",
  sample: "Element innerHTML|<b>x</b>",
  sourceFile: "https://core.example/assets/app-1.js",
  lineNumber: 1,
  columnNumber: 200,
};

/** Dispatches violation on page as the browser's securitypolicyviolation event. */
function violate(page: EventTarget, violation: PolicyViolation): void {
  page.dispatchEvent(Object.assign(new Event("securitypolicyviolation"), violation));
}

describe("the page's policy violations", () => {
  it("say what would be refused, by which directive and where", () => {
    expect(violationLine(innerHtml)).toBe(
      "the policy would refuse (require-trusted-types-for): Element innerHTML|<b>x</b> at /assets/app-1.js:1:200",
    );
    expect(violationLine({ ...innerHtml, disposition: "enforce", sourceFile: "" })).toBe(
      "the policy refused (require-trusted-types-for): Element innerHTML|<b>x</b>",
    );
  });

  it("go to the log once a place, a few in a page's life, until stopped", () => {
    const page = new EventTarget() as unknown as Document;
    const lines: string[] = [];
    const stop = notePolicyViolations(page, (line) => lines.push(line));
    violate(page, innerHtml);
    violate(page, innerHtml);
    for (let line = 2; line < 20; line++) {
      violate(page, { ...innerHtml, lineNumber: line });
    }
    expect(lines).toHaveLength(10);
    expect(lines[0]).toContain(":1:200");
    expect(lines[1]).toContain(":2:200");
    stop();
    violate(page, { ...innerHtml, sample: "Worker constructor|blob:x" });
    expect(lines).toHaveLength(10);
  });
});
