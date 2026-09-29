// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { byReason, Files, ReasonGroups, type SeenFile } from "./files.tsx";
import { mount, servedBy, type Heard } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

const file = (
  id: number,
  filing: string,
  why: { reason?: string; detail?: NonNullable<SeenFile["errorDetail"]> } = {},
): SeenFile => ({
  id,
  libraryId: 1,
  path: `/media/${String(id)}.mkv`,
  filing,
  firstSeenAt: 1,
  lastSeenAt: 1,
  ...(why.reason === undefined ? {} : { filingReason: why.reason }),
  ...(why.detail === undefined ? {} : { errorDetail: why.detail }),
});

describe("what the scanner saw", () => {
  it("groups files by filing and reason, the largest group first", () => {
    const groups = byReason([
      file(1, "errored", { reason: "permission_denied" }),
      file(2, "skipped", { reason: "extension" }),
      file(3, "errored", { reason: "permission_denied" }),
      file(4, "listed"),
    ]);
    expect(groups.map((group) => [group.filing, group.reason, group.files.length])).toEqual([
      ["errored", "permission_denied", 2],
      ["skipped", "extension", 1],
      ["listed", "", 1],
    ]);
  });

  it("names each group and opens to its paths and the platform's words", () => {
    const markup = renderToStaticMarkup(
      withServices(
        <ReasonGroups
          files={[
            file(1, "errored", {
              reason: "permission_denied",
              detail: {
                text: "permission denied",
                ownerUid: 1000,
                groupGid: 100,
                modeText: "-rw-------",
                occurrenceCount: 1,
              },
            }),
          ]}
        />,
      ),
    );
    expect(markup).toContain("<summary>Could not be read · permission_denied (1)</summary>");
    expect(markup).toContain("<code>/media/1.mkv</code>");
    expect(markup).toContain("Details: permission denied (owner 1000, group 100, mode -rw-------)");
  });
});

describe("the files screen", () => {
  it("asks for one library's files by libraryId, the parameter's name (D227)", async () => {
    const heard: Heard[] = [];
    const services = servedBy({ "GET /files": [200, { files: [file(1, "listed")] }] }, heard);
    const mounted = await mount(withServices(<Files libraryId={3} />, services));
    const asked = new URLSearchParams(heard[0]?.query);
    expect(heard[0]?.asked).toBe("GET /files");
    expect(asked.get("libraryId")).toBe("3");
    expect(mounted.container.textContent).toContain("/media/1.mkv");
    mounted.unmount();
  });
});
