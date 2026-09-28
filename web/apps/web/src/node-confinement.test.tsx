// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { NodeConfinementView, type NodeSandbox } from "./node-confinement.tsx";
import { withServices } from "./test-services.tsx";

function markupOf(node: NodeSandbox): string {
  return renderToStaticMarkup(withServices(<NodeConfinementView node={node} />));
}

describe("each machine's sandbox, said out loud (D112)", () => {
  it("says a machine runs FFmpeg sandboxed, and what is missing on one that does not, and how to fix it", () => {
    const local = markupOf({
      name: "local",
      confinement: [
        { mechanism: "landlock", isInForce: true },
        { mechanism: "seccomp", isInForce: true },
      ],
    });
    const devnas = markupOf({
      name: "devnas",
      confinement: [
        {
          mechanism: "landlock",
          isInForce: false,
          cause: "disabled_at_boot",
          reason: "a note for developers",
        },
        { mechanism: "seccomp", isInForce: true },
      ],
    });
    expect(local).toContain("local: FFmpeg runs in its sandbox");
    expect(devnas).toContain("devnas: FFmpeg runs without part of its sandbox");
    expect(devnas).toContain(
      "Landlock (which files FFmpeg may open) is in the kernel but not switched on at boot",
    );
    expect(devnas).not.toContain("a note for developers");
    expect(`${local}${devnas}`.match(/role="alert"/gu)).toHaveLength(1);
  });

  it("warns about a machine that reported no sandbox at all", () => {
    expect(markupOf({ name: "attic", confinement: [] })).toContain('role="alert"');
  });
});
