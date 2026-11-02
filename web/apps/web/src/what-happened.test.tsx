// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, expect, it } from "vitest";

import { buttonWithText, click, mount, type Mounted } from "./test-dom.tsx";
import { CopyWhatHappened, whatHappened } from "./what-happened.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

it("says what happened at a dead end: the server, the addresses tried, what failed and the browser", () => {
  const text = whatHappened(
    {
      coreId: "k7xq2",
      addressesTried: ["https://v4.k7xq2.zone.example", "https://v6.k7xq2.zone.example"],
      problem: new Error("no WebRTC connection connected within 10 s"),
    },
    "Mozilla/5.0 Firefox/140",
    [],
  );
  expect(text.split("\n")).toEqual([
    "Orpheum could not reach its server.",
    "Server: k7xq2",
    "Addresses tried: https://v4.k7xq2.zone.example, https://v6.k7xq2.zone.example",
    "What failed: no WebRTC connection connected within 10 s",
    "Browser: Mozilla/5.0 Firefox/140",
  ]);
  expect(
    whatHappened({ coreId: undefined, addressesTried: [], problem: "refused" }, "x"),
  ).toContain("Server: not found\nAddresses tried: none\nWhat failed: refused");
});

it("tells the steps the page took, last, each on its own line, so a connection's loss is placed", () => {
  const text = whatHappened({ coreId: "k7xq2", addressesTried: [], problem: "closed" }, "Safari", [
    "12 ms reached the core over webrtc",
    "15 ms service worker: registering (the page is not controlled)",
  ]);
  expect(text.split("\n").slice(-3)).toEqual([
    "Steps:",
    "  12 ms reached the core over webrtc",
    "  15 ms service worker: registering (the page is not controlled)",
  ]);
  expect(
    whatHappened({ coreId: "k7xq2", addressesTried: [], problem: "closed" }, "x", []),
  ).not.toContain("Steps:");
});

it("names each alternative and cause behind what failed, an AggregateError's empty message hiding none", () => {
  const problem = new AggregateError([
    new Error("the client could not start", {
      cause: new Error("the core refused the offer: the offer's proof does not hold"),
    }),
    new Error("no name answered"),
  ]);
  expect(whatHappened({ coreId: "k7xq2", addressesTried: [], problem }, "x")).toContain(
    "What failed: the client could not start; because the core refused the offer: the offer's proof does not hold; no name answered",
  );
});

it("copies what happened, and says so, or that it could not, leaving the text to select", async () => {
  const copied: string[] = [];
  mounted = await mount(
    <CopyWhatHappened
      text="Orpheum could not reach its server."
      copy={(text) => {
        copied.push(text);
        return Promise.resolve();
      }}
    />,
  );
  await click(buttonWithText(mounted.container, "Copy what happened"));
  expect(copied).toEqual(["Orpheum could not reach its server."]);
  expect(mounted.container.textContent).toContain("Copied. Send it to whoever runs your server.");
  mounted.unmount();
  mounted = await mount(
    <CopyWhatHappened text="the text" copy={() => Promise.reject(new Error("not allowed"))} />,
  );
  await click(buttonWithText(mounted.container, "Copy what happened"));
  expect(mounted.container.textContent).toContain("select the text below instead");
  expect(mounted.container.querySelector("pre")?.textContent).toBe("the text");
  mounted.unmount();
  mounted = await mount(<CopyWhatHappened text={undefined} copy={() => Promise.resolve()} />);
  expect(mounted.container.querySelector("button")).toBeNull();
});
