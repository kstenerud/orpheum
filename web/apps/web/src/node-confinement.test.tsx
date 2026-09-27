// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { NodeConfinementView } from "./node-confinement.tsx";
import { withServices } from "./test-services.tsx";

describe("each machine's sandbox, said out loud (D112)", () => {
  it("says a machine runs FFmpeg sandboxed, and what is missing on one that does not, and how to fix it", () => {
    const markup = renderToStaticMarkup(
      withServices(
        <NodeConfinementView
          nodes={[
            {
              name: "local",
              isConnected: true,
              roots: [],
              confinement: [
                { mechanism: "landlock", isInForce: true },
                { mechanism: "seccomp", isInForce: true },
              ],
            },
            {
              name: "devnas",
              isConnected: true,
              roots: [],
              confinement: [
                {
                  mechanism: "landlock",
                  isInForce: false,
                  cause: "disabled_at_boot",
                  reason: "a note for developers",
                },
                { mechanism: "seccomp", isInForce: true },
              ],
            },
            { name: "attic", isConnected: false, roots: [], confinement: [] },
          ]}
        />,
      ),
    );
    expect(markup).toContain("local: FFmpeg runs in its sandbox");
    expect(markup).toContain("devnas: FFmpeg runs without part of its sandbox");
    expect(markup).toContain(
      "Landlock (which files FFmpeg may open) is in the kernel but not switched on at boot",
    );
    expect(markup).not.toContain("a note for developers");
    expect(markup).not.toContain("attic");
    expect(markup.match(/role="alert"/gu)).toHaveLength(1);
  });
});
