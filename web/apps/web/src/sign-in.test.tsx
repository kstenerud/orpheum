// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import { i18n } from "./i18n.ts";
import { SignIn, SignInForm } from "./sign-in.tsx";
import {
  buttonWithText,
  click,
  enter,
  mount,
  refusal,
  servedBy,
  submit,
  unreachable,
  type Answers,
  type Heard,
} from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

const ignore = (): void => undefined;

/** A server with its admin already made, as most sign-ins meet. */
const setUpAlready: Answers[string] = [200, { isRequired: false }];

/** What the sign-in screen says after the server answers a sign-in with answer. */
async function problemShownFor(answer: Answers[string]): Promise<string | undefined> {
  const mounted = await mount(
    withServices(<SignIn />, servedBy({ "GET /setup": setUpAlready, "POST /session": answer })),
  );
  await submit(mounted.container.querySelector("form"));
  const shown = mounted.container.querySelector('[role="alert"]')?.textContent ?? undefined;
  mounted.unmount();
  return shown;
}

describe("sign-in", () => {
  it("names each failure so a person knows what to do", async () => {
    expect(await problemShownFor(refusal(401, "auth_required"))).toBe(
      "The name or password is wrong.",
    );
    expect(await problemShownFor(refusal(429, "rate_limited"))).toContain("Too many failed");
    expect(await problemShownFor(refusal(409, "setup_required"))).toContain("setup code");
    // Every other refusal is told by its code, as on every other screen.
    expect(await problemShownFor(refusal(503, "migrating"))).toBe(
      "The server is starting up, or updating. It will answer in a moment.",
    );
    expect(await problemShownFor(unreachable)).toContain("could not be reached");
    const markup = renderToStaticMarkup(
      <SignInForm
        name="owner"
        password=""
        isSending={false}
        problem={i18n.t("signIn.noAdminYet")}
        onNameChange={ignore}
        onPasswordChange={ignore}
        onSubmit={ignore}
        onUseCode={ignore}
      />,
    );
    expect(markup).toContain('role="alert"');
    expect(markup).toContain("setup code");
    expect(markup).toContain('autoComplete="current-password"');
  });

  it("says a failure that is neither the server's nor the network's in general words, and writes its cause to the console", async () => {
    const written = vi.spyOn(console, "error").mockImplementation(() => undefined);
    // A 204 is an answer with nothing in it: the SDK has no session and no refusal to read.
    expect(await problemShownFor([204, undefined])).toBe(i18n.t("problem.unknown"));
    const causes = written.mock.calls.map((call): unknown => call.at(-1));
    expect(causes).toHaveLength(1);
    expect(causes[0]).toBeInstanceOf(Error);
    vi.restoreAllMocks();
  });

  it("labels every field and says nothing when nothing went wrong", () => {
    const markup = renderToStaticMarkup(
      <SignInForm
        name=""
        password=""
        isSending={false}
        problem={undefined}
        onNameChange={ignore}
        onPasswordChange={ignore}
        onSubmit={ignore}
        onUseCode={ignore}
      />,
    );
    expect(markup).toContain("<label");
    expect(markup).toContain(">Name<");
    expect(markup).toContain(">Password<");
    expect(markup).not.toContain('role="alert"');
  });

  it("keeps the session a sign-in answers with, on the device it names, with the device's secret apart from the token", async () => {
    const services = servedBy({
      "GET /setup": setUpAlready,
      "POST /session": [
        201,
        {
          token: "new-token",
          deviceSecret: "phone-secret",
          user: { device: { id: 9, name: "Phone" } },
        },
      ],
    });
    const mounted = await mount(withServices(<SignIn />, services));
    await submit(mounted.container.querySelector("form"));
    mounted.unmount();
    expect(services.session.token()).toBe("new-token");
    expect(services.session.deviceId()).toBe(9);
    expect(services.session.deviceProof()).toBe("phone-secret");
  });
});

describe("setting up a server no one can sign in to yet (A29, A31)", () => {
  it("opens setup when the server needs it, makes the first admin with the code and signs in", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "GET /setup": setUpAlready,
        "POST /session": [
          refusal(409, "setup_required"),
          [
            201,
            {
              token: "admin-token",
              deviceSecret: "s",
              user: { device: { id: 1, name: "Laptop" } },
            },
          ],
        ],
        "POST /setup": [201, { id: 1, name: "owner", role: "admin" }],
      },
      heard,
    );
    const mounted = await mount(withServices(<SignIn />, services));
    await submit(mounted.container.querySelector("form"));
    expect(mounted.container.textContent).toContain("Set up this server");
    await enter(mounted.container.querySelector('input[name="setupCode"]'), "gh3k-c5vq-hhvh");
    await enter(mounted.container.querySelector('input[name="name"]'), "owner");
    await enter(mounted.container.querySelector('input[type="password"]'), "a password");
    await submit(mounted.container.querySelector("form"));
    expect(heard.find((each) => each.methodAndPath === "POST /setup")?.body).toEqual({
      setupCode: "gh3k-c5vq-hhvh",
      name: "owner",
      password: "a password",
    });
    expect(services.session.token()).toBe("admin-token");
    mounted.unmount();
  });

  it("says when the setup code is wrong, and goes back to sign-in", async () => {
    const services = servedBy({
      "GET /setup": setUpAlready,
      "POST /session": refusal(409, "setup_required"),
      "POST /setup": refusal(403, "forbidden"),
    });
    const mounted = await mount(withServices(<SignIn />, services));
    await submit(mounted.container.querySelector("form"));
    await submit(mounted.container.querySelector("form"));
    expect(mounted.container.textContent).toContain("That setup code is wrong");
    await click(buttonWithText(mounted.container, "Back"));
    expect(mounted.container.textContent).toContain("Get a code");
    mounted.unmount();
  });
});

describe("a server that needs setting up", () => {
  it("opens on setting it up, with no sign-in tried first (A31)", async () => {
    const heard: Heard[] = [];
    const mounted = await mount(
      withServices(<SignIn />, servedBy({ "GET /setup": [200, { isRequired: true }] }, heard)),
    );
    expect(mounted.container.textContent).toContain("Set up this server");
    expect(heard.map((each) => each.methodAndPath)).toEqual(["GET /setup"]);
    mounted.unmount();
    const setUp = await mount(
      withServices(<SignIn />, servedBy({ "GET /setup": [200, { isRequired: false }] })),
    );
    expect(setUp.container.textContent).not.toContain("Set up this server");
    expect(setUp.container.textContent).toContain("Get a code");
    setUp.unmount();
  });
});
