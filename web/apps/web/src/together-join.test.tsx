// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { describe, expect, it } from "vitest";

import { JoinForm } from "./together-join.tsx";
import { buttonWithText, enter, mount, refusal, servedBy, submit } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

describe("joining by a link", () => {
  it("tells a guest whose link no longer works that invitations are short-lived and to ask for another (D346)", async () => {
    const services = servedBy(
      {
        "GET /health": [200, { status: "ready", reasons: [] }],
        "POST /together/join": refusal(404, "not_found"),
      },
      [],
    );
    const mounted = await mount(
      withServices(
        <JoinForm secret="old-secret" onSubmitted={() => undefined} onJoined={() => undefined} />,
        services,
      ),
    );
    await enter(mounted.container.querySelector("input"), "Gran");
    await submit(mounted.container.querySelector("form"));
    expect(mounted.container.textContent).toContain(
      "invitations are short-lived, and this one has expired",
    );
    expect(mounted.container.textContent).toContain("Ask the person who sent it for a new one.");
    expect(buttonWithText(mounted.container, "Join")).toBeDefined();
    mounted.unmount();
  });
});
