// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { JoinForm } from "./together-join.tsx";
import {
  buttonWithText,
  click,
  enter,
  mount,
  refusal,
  servedBy,
  submit,
  type Heard,
} from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

describe("joining by a link", () => {
  it("tells a guest whose link no longer works that invitations are short-lived and to ask for another (D346)", async () => {
    const services = servedBy(
      {
        "POST /together/join/check": [204, undefined],
        "POST /together/join": refusal(404, "notFound"),
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

describe("a link refused before its form (VQ172(3))", () => {
  // The owner, 2026-10-30, a link used already opened in another browser: "ideally it should tell
  // them right off the bat rather than letting them try to fill out the form uselessly first."
  it("says a link the server would refuse in the form's place, asking only whether it admits", async () => {
    const heard: Heard[] = [];
    const services = servedBy({ "POST /together/join/check": refusal(404, "notFound") }, heard);
    const mounted = await mount(
      withServices(
        <JoinForm secret="used-secret" onSubmitted={() => undefined} onJoined={() => undefined} />,
        services,
      ),
    );
    expect(mounted.container.querySelector("form")).toBeNull();
    expect(mounted.container.querySelector("input")).toBeNull();
    expect(mounted.container.querySelector('[role="alert"]')?.textContent).toContain(
      "invitations are short-lived, and this one has expired, was used already",
    );
    expect(heard).toEqual([
      { methodAndPath: "POST /together/join/check", body: { secret: "used-secret" } },
    ]);
    mounted.unmount();
  });

  it("shows the form once the server says the link admits, and while it is asked shows none", async () => {
    const services = servedBy({ "POST /together/join/check": [204, undefined] }, []);
    const tree = withServices(
      <JoinForm secret="new-secret" onSubmitted={() => undefined} onJoined={() => undefined} />,
      services,
    );
    expect(renderToStaticMarkup(tree)).toContain("Checking your invitation");
    expect(renderToStaticMarkup(tree)).not.toContain("<form");
    const mounted = await mount(tree);
    expect(mounted.container.querySelector("form input")).not.toBeNull();
    expect(buttonWithText(mounted.container, "Join")).toBeDefined();
    mounted.unmount();
  });
});

describe("joining by a link while in another session (D372 §8)", () => {
  it("asks first whether to leave the other, and joins, leaving it, only when told to", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "POST /together/join/check": [204, undefined],
        "POST /together/join": [
          [
            409,
            {
              error: {
                code: "inAnotherSession",
                message: "a note for developers",
                detail: { hostName: "Karl", isHost: false },
              },
            },
          ],
          [200, { sessionId: 4, memberId: 9, membershipCredential: "new", locator: "l" }],
        ],
      },
      heard,
    );
    const joined: unknown[] = [];
    const mounted = await mount(
      withServices(
        <JoinForm
          secret="new-secret"
          heldCredential="kept-credential"
          onSubmitted={() => undefined}
          onJoined={(membership) => joined.push(membership)}
        />,
        services,
      ),
    );
    await enter(mounted.container.querySelector("input"), "Gran");
    await submit(mounted.container.querySelector("form"));
    expect(mounted.container.textContent).toContain("Leave Karl's session to join this one?");
    expect(joined).toEqual([]);
    expect(buttonWithText(mounted.container, "Leave and join")).toBeDefined();
    await submit(mounted.container.querySelector("form"));
    expect(joined).toHaveLength(1);
    const bodies = heard
      .filter((each) => each.methodAndPath === "POST /together/join")
      .map((each) => each.body);
    expect(bodies).toEqual([
      { secret: "new-secret", name: "Gran", heldCredential: "kept-credential" },
      {
        secret: "new-secret",
        name: "Gran",
        heldCredential: "kept-credential",
        shouldLeaveOther: true,
      },
    ]);
    mounted.unmount();
  });

  it("asks a host whether to leave their own session, which ends for everyone, and asks nothing more on Don't join", async () => {
    const services = servedBy(
      {
        "POST /together/join/check": [204, undefined],
        "POST /together/join": [
          409,
          {
            error: {
              code: "inAnotherSession",
              message: "a note for developers",
              detail: { hostName: "Gran", isHost: true },
            },
          },
        ],
      },
      [],
    );
    const mounted = await mount(
      withServices(
        <JoinForm secret="new-secret" onSubmitted={() => undefined} onJoined={() => undefined} />,
        services,
      ),
    );
    await enter(mounted.container.querySelector("input"), "Gran");
    await submit(mounted.container.querySelector("form"));
    expect(mounted.container.textContent).toContain(
      "Leave your own session to join this one? It ends for everyone in it.",
    );
    await click(buttonWithText(mounted.container, "Don't join"));
    expect(buttonWithText(mounted.container, "Join")).toBeDefined();
    expect(mounted.container.textContent).not.toContain("Leave your own session");
    mounted.unmount();
  });
});
