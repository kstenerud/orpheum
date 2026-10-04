// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";

import { RepointRoot, type Place } from "./repoint-root.tsx";
import { buttonWithText, choose, click, mount, type Mounted } from "./test-dom.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

describe("saying a folder moved (D241 §6)", () => {
  it("offers the connected nodes' other folders, and moves it where the admin chose once confirmed", async () => {
    const moves: Place[] = [];
    const places = [
      { nodeName: "devnas", path: "/media/films" },
      { nodeName: "devnas", path: "/mnt/films" },
      { nodeName: "attic", path: "/films" },
    ];
    mounted = await mount(
      <RepointRoot
        from={{ nodeName: "devnas", path: "/media/films" }}
        places={places}
        onRepoint={(to) => moves.push(to)}
      />,
    );
    await click(buttonWithText(mounted.container, "This folder moved…"));
    const options = [...mounted.container.querySelectorAll("option")].map(
      (option) => option.textContent,
    );
    expect(options).toEqual(["devnas: /mnt/films", "attic: /films"]);
    await choose(mounted.container.querySelector("select"), "1");
    expect(moves).toEqual([]);
    await click(buttonWithText(mounted.container, "Use the new place"));
    expect(moves).toEqual([{ nodeName: "attic", path: "/films" }]);
  });
});
