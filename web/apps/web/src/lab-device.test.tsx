// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { describe, expect, it } from "vitest";

import { LabDevice } from "./lab-device.tsx";
import { mount, servedBy } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

describe("the device page", () => {
  it("shows what the screen says of high dynamic range and colours (the owner's TCL TV, 2026-11-06)", async () => {
    const page = await mount(withServices(<LabDevice />, servedBy({}, [], "token")));
    const display = [...page.container.querySelectorAll("dt")].find(
      (term) => term.textContent === "High dynamic range",
    );
    expect(display?.nextElementSibling?.textContent).toMatch(/^page .+, video .+; colours .+$/u);
  });
});
