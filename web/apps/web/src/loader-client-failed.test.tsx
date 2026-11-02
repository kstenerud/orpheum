// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, expect, it } from "vitest";

import { ClientFailedPage, clientFailedText } from "./loader-client-failed.tsx";
import { buttonWithText, mount, type Mounted } from "./test-dom.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

it("says the client did not load, with what happened to copy: the server, the file, the worker, the browser", async () => {
  const text = clientFailedText(
    { coreId: "k7xq2", address: "/assets/index-b.js?client", isControlled: true },
    "Mozilla/5.0 Chrome/156",
    ["40 ms loading the core's client"],
  );
  expect(text).toContain("Server: k7xq2");
  expect(text).toContain("File not loaded: /assets/index-b.js?client");
  expect(text).toContain("Service worker controlling the page: yes");
  expect(text).toContain("Browser: Mozilla/5.0 Chrome/156");
  expect(text).toContain("Steps:\n  40 ms loading the core's client");

  expect(text).toContain('untick "Bypass for network" under Application → Service workers');

  mounted = await mount(<ClientFailedPage text={text} isControlled={true} />);
  expect(mounted.container.querySelector('[role="alert"]')?.textContent).toContain("did not load");
  expect(mounted.container.textContent).toContain("untick “Bypass for network”");
  expect(buttonWithText(mounted.container, "Copy what happened")).toBeDefined();
  expect(mounted.container.querySelector(".what-happened pre")?.textContent).toBe(text);
});

it("says nothing of developer tools where no worker controls the page", async () => {
  mounted = await mount(<ClientFailedPage text="" isControlled={false} />);
  expect(mounted.container.textContent).not.toContain("Bypass for network");
});
