// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act } from "react";
import { afterEach, expect, it } from "vitest";

import { FilingReportButton, SystemFilingReports } from "./filing-report.tsx";
import { buttonWithText, click, mount, servedBy, type Heard, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import { IsAdminContext } from "./warning-sign.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

const report = {
  name: "filed-wrongly-606-20261016T210000Z.md",
  path: "/var/lib/orpheum/core/backups/reports/filed-wrongly-606-20261016T210000Z.md",
  markdown: "# Filed wrongly: Greywolf\n",
};

it("writes an admin's report on the server and shows it, with where it is, to copy (D306 §3)", async () => {
  const heard: Heard[] = [];
  mounted = await mount(
    withServices(
      <IsAdminContext value={true}>
        <FilingReportButton itemId={606} />
      </IsAdminContext>,
      servedBy({ "POST /items/606/filing-report": [201, report] }, heard),
    ),
  );
  await click(buttonWithText(mounted.container, "Report this"));
  expect(heard.map((each) => each.methodAndPath)).toEqual(["POST /items/606/filing-report"]);
  expect(mounted.container.querySelector("textarea")?.value).toBe("# Filed wrongly: Greywolf\n");
  expect(mounted.container.textContent).toContain(`Saved on the server as ${report.path}.`);
  expect(buttonWithText(mounted.container, "Copy")).not.toBeNull();
});

it("offers no report to someone who is not an admin", async () => {
  mounted = await mount(withServices(<FilingReportButton itemId={606} />, servedBy({})));
  expect(mounted.container.textContent).toBe("");
});

it("lists the System page's reports, and reads one when it is opened", async () => {
  const heard: Heard[] = [];
  mounted = await mount(
    withServices(
      <SystemFilingReports />,
      servedBy(
        {
          "GET /filing-reports": [
            200,
            { reports: [{ name: report.name, sizeBytes: 30, writtenAt: 1_792_190_000_000 }] },
          ],
          [`GET /filing-reports/${report.name}`]: [
            200,
            { name: report.name, markdown: report.markdown },
          ],
        },
        heard,
      ),
    ),
  );
  expect(mounted.container.textContent).toContain(report.name);
  const details = mounted.container.querySelector("details");
  await act(async () => {
    if (details !== null) {
      details.open = true;
      details.dispatchEvent(new Event("toggle"));
    }
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
  expect(heard.map((each) => each.methodAndPath)).toContain(`GET /filing-reports/${report.name}`);
  expect(mounted.container.querySelector("textarea")?.value).toBe(report.markdown);
});
