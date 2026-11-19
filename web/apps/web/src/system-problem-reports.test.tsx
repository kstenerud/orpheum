// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";

import { SystemProblemReports } from "./system-problem-reports.tsx";
import {
  buttonWithText,
  click,
  mount,
  servedBy,
  type Answers,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

const report = {
  id: 4,
  userName: "dan",
  madeAt: 1_790_000_000_000,
  version: "0.9.0",
  words: "the sound drifts",
};

/** The System page's section, its server answering answers. */
async function section(answers: Answers, heard: Heard[] = []): Promise<HTMLElement> {
  mounted = await mount(withServices(<SystemProblemReports />, servedBy(answers, heard)));
  return mounted.container;
}

/** The downloads the section asked for, by their query. */
function downloads(heard: readonly Heard[]): (string | undefined)[] {
  return heard
    .filter((each) => each.methodAndPath === "GET /problem-reports/4")
    .map((each) => each.query);
}

describe("the System page's problem reports (D524 §2, §4)", () => {
  it("says there is none when no one has reported", async () => {
    const container = await section({ "GET /problem-reports": [200, { reports: [] }] });
    expect(container.textContent).toContain("No one has reported a problem.");
  });

  it("lists each report with who, when, which version and their words, and one gone as gone", async () => {
    const gone = { ...report, id: 5, userName: undefined, words: "it stopped" };
    const container = await section({
      "GET /problem-reports": [200, { reports: [report, gone] }],
    });
    const summaries = [...container.querySelectorAll("summary")].map((each) => each.textContent);
    expect(summaries[0]).toMatch(/^dan, .+, version 0\.9\.0$/u);
    expect(summaries[1]).toMatch(/^Someone whose account is gone, /u);
    expect(container.querySelector("blockquote")?.textContent).toBe("the sound drifts");
  });

  it("downloads the form chosen, the other library's names only in the full one when asked", async () => {
    const heard: Heard[] = [];
    const container = await section(
      {
        "GET /problem-reports": [200, { reports: [report] }],
        "GET /problem-reports/4": [200, { whole: "# Report\n", summary: "the summary" }],
      },
      heard,
    );
    const checkbox = container.querySelector<HTMLInputElement>('input[type="checkbox"]');
    await click(checkbox);
    await click(buttonWithText(container, "Shareable"));
    expect(container.textContent).toContain("every name it found is hidden");
    expect(container.querySelector("textarea")?.value).toBe("the summary");
    expect(buttonWithText(container, "Save problem-report-4-shareable.md")).toBeDefined();
    await click(buttonWithText(container, "Full"));
    expect(container.textContent).toContain("Do not share it.");
    await click(checkbox);
    await click(buttonWithText(container, "Full"));
    expect(downloads(heard)).toEqual([
      "?form=shareable",
      "?form=full&includeOther=true",
      "?form=full",
    ]);
  });

  it("deletes a report only at the confirmed press, then lists the reports again", async () => {
    const heard: Heard[] = [];
    const container = await section(
      {
        "GET /problem-reports": [
          [200, { reports: [report] }],
          [200, { reports: [] }],
        ],
        "DELETE /problem-reports/4": [204, undefined],
      },
      heard,
    );
    await click(buttonWithText(container, "Delete"));
    expect(heard.filter((each) => each.methodAndPath.startsWith("DELETE"))).toEqual([]);
    await click(buttonWithText(container, "Delete the report"));
    expect(heard.filter((each) => each.methodAndPath.startsWith("DELETE"))).toHaveLength(1);
    expect(container.textContent).toContain("No one has reported a problem.");
  });
});
