// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { i18n } from "./i18n.ts";
import { ApiProblem, fieldProblemText, problemText } from "./problem.ts";

const t = i18n.t.bind(i18n);

/** An invalid_request refusal with a problem at each location. */
function refusedAt(...locations: string[]): ApiProblem {
  return new ApiProblem({
    code: "invalid_request",
    message: "validation failed",
    detail: {
      problems: locations.map((location) => ({ location, message: "expected array length >= 1" })),
    },
  });
}

const fields = { "body.name": "Name", "body.rootPaths": "Folders this library reads" };

describe("fieldProblemText", () => {
  it("names the field the server refused in the form's own words (D188 §1)", () => {
    expect(fieldProblemText(t, refusedAt("body.rootPaths"), { fields })).toBe(
      "The server did not accept what was entered in Folders this library reads. Correct what is there and try again.",
    );
  });

  it("names each refused field once, a list's items under the list's label", () => {
    expect(
      fieldProblemText(t, refusedAt("body.name", "body.rootPaths[0]", "body.rootPaths[1]"), {
        fields,
      }),
    ).toBe(
      "The server did not accept what was entered in Name and Folders this library reads. Correct what is there and try again.",
    );
  });

  it("never shows the server's message", () => {
    expect(fieldProblemText(t, refusedAt("body.rootPaths"), { fields })).not.toContain("expected");
  });

  it("falls back to the general words when a problem is not somewhere the form labels", () => {
    const general = problemText(t, refusedAt("body.name"));
    expect(fieldProblemText(t, refusedAt("body.name", "body.kind"), { fields })).toBe(general);
    expect(fieldProblemText(t, refusedAt("body"), { fields })).toBe(general);
    expect(fieldProblemText(t, refusedAt(), { fields })).toBe(general);
  });

  it("leaves any other failure, and its overrides, to problemText", () => {
    const conflict = new ApiProblem({ code: "conflict", message: "taken" });
    expect(
      fieldProblemText(t, conflict, {
        fields,
        overrides: { conflict: "That name is taken." },
      }),
    ).toBe("That name is taken.");
    expect(fieldProblemText(t, new TypeError("Failed to fetch"), { fields })).toBe(
      problemText(t, new TypeError("Failed to fetch")),
    );
  });
});
