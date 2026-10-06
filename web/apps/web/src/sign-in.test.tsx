// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import { i18n } from "./i18n.ts";
import { SignIn, SignInForm } from "./sign-in.tsx";
import { mount, refusal, servedBy, submit, unreachable, type Answers } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

const ignore = (): void => undefined;

/** What the sign-in screen says after the server answers a sign-in with answer. */
async function problemShownFor(answer: Answers[string]): Promise<string | undefined> {
  const mounted = await mount(withServices(<SignIn />, servedBy({ "POST /session": answer })));
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

  it("keeps the session a sign-in answers with, on the device it names", async () => {
    const services = servedBy({
      "POST /session": [201, { token: "new-token", user: { device: { id: 9, name: "Phone" } } }],
    });
    const mounted = await mount(withServices(<SignIn />, services));
    await submit(mounted.container.querySelector("form"));
    mounted.unmount();
    expect(services.session.token()).toBe("new-token");
    expect(services.session.deviceId()).toBe(9);
  });
});
