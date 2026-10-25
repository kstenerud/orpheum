// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { Poster, Still } from "./poster.tsx";
import { mount, type Mounted } from "./test-dom.tsx";

describe("a poster's or a still's image that does not load (the owner, Fire TV, 2026-10-25)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  it("shows the poster's named blank, not a broken image", async () => {
    mounted = await mount(<Poster sourceSha256="ab12" />);
    act(() => {
      mounted?.container.querySelector("img")?.dispatchEvent(new Event("error"));
    });
    expect(mounted.container.querySelector("img")).toBeNull();
    expect(mounted.container.querySelector(".poster--blank")).not.toBeNull();
  });

  it("shows a still's blank of its size, not a broken image", async () => {
    mounted = await mount(<Still sourceSha256="cd34" className="series__still" />);
    act(() => {
      mounted?.container.querySelector("img")?.dispatchEvent(new Event("error"));
    });
    expect(mounted.container.querySelector("img")).toBeNull();
    expect(mounted.container.querySelector("span.series__still")).not.toBeNull();
  });
});
