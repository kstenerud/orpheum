// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { NodeConfinementView, type NodeConfinement } from "./node-confinement.tsx";
import { withServices } from "./test-services.tsx";

function markupOf(node: NodeConfinement): string {
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
          causeCode: "disabledAtBoot",
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
    const blocked = markupOf({
      name: "boxed",
      confinement: [
        { mechanism: "landlock", isInForce: false, causeCode: "blockedByRuntime" },
        { mechanism: "seccomp", isInForce: true },
      ],
    });
    expect(blocked).toContain("boxed: FFmpeg runs without part of its sandbox");
    // How the machine stands, not a failure of anything done: told politely, in the yellow.
    expect(`${local}${devnas}${blocked}`.match(/class="needs-fixing"/gu)).toHaveLength(1);
    expect(blocked).toContain(
      '<div class="needs-fixing"><p><output>boxed: FFmpeg runs without part of its sandbox',
    );
    expect(blocked).not.toContain('role="alert"');
  });

  it("notes calmly a part the machine's kernel lacks, naming the kernel that brings it (D454 §3)", () => {
    const old = markupOf({
      name: "ds218",
      confinement: [
        { mechanism: "landlock", isInForce: false, causeCode: "kernelWithout" },
        { mechanism: "seccomp", isInForce: true },
      ],
    });
    expect(old).toContain(
      "ds218: FFmpeg runs without part of its sandbox, which this machine&#x27;s kernel",
    );
    expect(old).toContain("Landlock (which files FFmpeg may open) needs Linux 5.13 or newer.");
    expect(old).not.toContain("needs-fixing");
    const failing = markupOf({
      name: "ds218",
      confinement: [
        { mechanism: "landlock", isInForce: false, causeCode: "kernelWithout" },
        { mechanism: "seccomp", isInForce: false, causeCode: "unexplained" },
      ],
    });
    expect(failing).toContain('class="needs-fixing"');
  });

  it("warns about a machine that reported no sandbox at all", () => {
    expect(markupOf({ name: "attic", confinement: [] })).toContain('class="needs-fixing"');
  });
});
