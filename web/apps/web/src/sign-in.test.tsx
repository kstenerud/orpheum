// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { SignInForm, signInProblem } from "./sign-in.tsx";

const ignore = (): void => undefined;

describe("sign-in", () => {
  it("names each failure so a person knows what to do", () => {
    expect([401, 429, 409, 500].map(signInProblem)).toEqual([
      "wrongNameOrPassword",
      "tooManyAttempts",
      "noAdminYet",
      "failed",
    ]);
    const markup = renderToStaticMarkup(
      <SignInForm
        name="owner"
        password=""
        isSending={false}
        problem="noAdminYet"
        onName={ignore}
        onPassword={ignore}
        onSubmit={ignore}
      />,
    );
    expect(markup).toContain('role="alert"');
    expect(markup).toContain("setup code");
    expect(markup).toContain('autoComplete="current-password"');
  });

  it("labels every field and says nothing when nothing went wrong", () => {
    const markup = renderToStaticMarkup(
      <SignInForm
        name=""
        password=""
        isSending={false}
        problem={undefined}
        onName={ignore}
        onPassword={ignore}
        onSubmit={ignore}
      />,
    );
    expect(markup).toContain("<label");
    expect(markup).toContain(">Name<");
    expect(markup).toContain(">Password<");
    expect(markup).not.toContain('role="alert"');
  });
});
