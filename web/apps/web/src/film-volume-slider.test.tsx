// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, expect, it, vi } from "vitest";

import { FilmVolumeSlider } from "./film-volume-slider.tsx";
import { mount } from "./test-dom.tsx";

afterEach(() => {
  vi.restoreAllMocks();
});

it("is a slider where the page may set the film's volume", async () => {
  const mounted = await mount(<FilmVolumeSlider />);
  expect(mounted.container.querySelector(".film-volume input")).not.toBeNull();
  mounted.unmount();
});

it("is the phone's volume buttons where the page may not, as on an iPhone", async () => {
  // iOS ignores a page setting a media element's volume: it reads back 1 (idiosyncrasies.md).
  vi.spyOn(HTMLMediaElement.prototype, "volume", "set").mockImplementation(() => undefined);
  vi.spyOn(HTMLMediaElement.prototype, "volume", "get").mockReturnValue(1);
  const mounted = await mount(<FilmVolumeSlider />);
  expect(mounted.container.querySelector("input")).toBeNull();
  expect(mounted.container.textContent).toBe("Use your phone's volume buttons for the film.");
  mounted.unmount();
});
