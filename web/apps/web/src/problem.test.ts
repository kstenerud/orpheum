// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { i18n } from "./i18n.ts";
import { ApiProblem, fieldProblemText, problemText, providerProblemText } from "./problem.ts";

const t = i18n.t.bind(i18n);

/** An invalid_request refusal with a problem at each location. */
function refusedAt(...locations: string[]): ApiProblem {
  return new ApiProblem({
    code: "invalidRequest",
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

describe("a play of a file that cannot be read now (D241 §4)", () => {
  it("says what is missing, on which machine, and since when", () => {
    const away = new ApiProblem({
      code: "mediaUnreachable",
      message: "a note for developers",
      detail: { reason: "nodeAway", nodeName: "devnas", unavailableAt: 1_790_000_000_000 },
    });
    const folder = new ApiProblem({
      code: "mediaUnreachable",
      message: "a note for developers",
      detail: { reason: "rootMissing", nodeName: "devnas", path: "/media/films" },
    });
    expect(problemText(t, away)).toMatch(
      /^This cannot be played now: the server cannot reach devnas, the computer the film is on, since .+\. Check that it is on and connected\.$/u,
    );
    expect(problemText(t, folder)).toBe(
      "This cannot be played now: the folder /media/films on devnas is missing.",
    );
  });
});

describe("a provider not answering (D231 §12)", () => {
  it("says since when, and when it will be checked again, when the server knows", () => {
    const knowing = new ApiProblem({
      code: "providerError",
      message: "a note for developers",
      detail: {
        reason: "unavailable",
        notAnsweringAt: 1_790_000_000_000,
        nextCheckAt: 1_790_000_480_000,
      },
    });
    const unknowing = new ApiProblem({
      code: "providerError",
      message: "a note for developers",
      detail: { reason: "unavailable" },
    });
    expect(providerProblemText(t, knowing, "identify.providerReason")).toMatch(
      /^TMDB has not been answering, since .+; it will be checked again at .+\.$/u,
    );
    expect(providerProblemText(t, unknowing, "identify.providerReason")).toBe(
      "TMDB is not answering right now. Try again later.",
    );
  });
});

describe("a playback the server no longer has", () => {
  it("says the server restarted and to press play, not that the film was removed", () => {
    const gone = new ApiProblem({ code: "playbackGone", message: "no such playback" });
    const text = problemText(t, gone);
    expect(text).toContain("The server restarted");
    expect(text).toContain("Reload the page to carry on");
    expect(text).not.toContain("removed");
  });
});

describe("a play of a file that cannot be converted (D344 §5)", () => {
  it("says the file cannot be converted for this device, never the server's message", () => {
    const refusal = new ApiProblem({ code: "cannotConvert", message: "a note for developers" });
    const text = problemText(t, refusal);
    expect(text).toContain("can't be converted for this device");
    expect(text).not.toContain("a note for developers");
  });
});
