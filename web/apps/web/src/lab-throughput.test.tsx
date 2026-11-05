// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";

import { LabThroughput, throughputBytes } from "./lab-throughput.tsx";
import {
  bodyField,
  buttonWithText,
  click,
  enter,
  mount,
  servedBy,
  settleUntil,
  type Heard,
} from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

/** The stand-in's body: a JSON string, its quotes counted, as the server's bytes would be. */
const body = "x".repeat(1000);
const bodyBytes = JSON.stringify(body).length;

afterEach(() => {
  localStorage.clear();
});

/** The throughput page mounted against a stand-in server, and what that server heard. */
async function mounted(): Promise<{
  readonly heard: Heard[];
  readonly page: Awaited<ReturnType<typeof mount>>;
}> {
  const heard: Heard[] = [];
  const services = servedBy(
    {
      "GET /lab/throughput": [200, body],
      "POST /lab/results": [200, { id: "T8QW2K3J" }],
    },
    heard,
    "token",
  );
  return { heard, page: await mount(withServices(<LabThroughput />, services)) };
}

/** The leg each load of the body asked for, in turn. */
function legsAsked(heard: readonly Heard[]): string[] {
  return heard
    .filter((request) => request.methodAndPath === "GET /lab/throughput")
    .map((request) => new URLSearchParams(request.query).get("leg") ?? "");
}

describe("the throughput test", () => {
  it("loads the body each way in turn, shows each one's rate, and sends them all to the server's log", async () => {
    const { heard, page } = await mounted();
    await enter(page.container.querySelector('input[type="url"]'), "https://lan.example:8443/");
    await click(buttonWithText(page.container, "Run"));
    await settleUntil(() => heard.some((request) => request.methodAndPath === "POST /lab/results"));

    expect(legsAsked(heard)).toEqual(["transport", "relay", "lan"]);
    const asked = heard.find((request) => request.methodAndPath === "GET /lab/throughput");
    expect(new URLSearchParams(asked?.query).get("bytes")).toBe(String(throughputBytes));
    const sent = heard.find((request) => request.methodAndPath === "POST /lab/results");
    expect(sent === undefined ? undefined : bodyField(sent, "probe")).toBe("throughput");
    const result = sent === undefined ? undefined : bodyField(sent, "result");
    for (const leg of ["transport", "relay", "lan"]) {
      expect(Reflect.get(Object(Reflect.get(Object(result), leg)), "bytes")).toBe(bodyBytes);
    }
    expect(Reflect.get(Object(result), "userAgent")).toBe(navigator.userAgent);
    expect(page.container.textContent).toContain("Mbit/s");
    expect(page.container.textContent).toContain("Sent as T8QW2K3J");
    page.unmount();
  });

  it("skips the address at home while none is typed, and keeps one typed for the next visit", async () => {
    const first = await mounted();
    await click(buttonWithText(first.page.container, "Run"));
    await settleUntil(() =>
      first.heard.some((request) => request.methodAndPath === "POST /lab/results"),
    );
    expect(legsAsked(first.heard)).toEqual(["transport", "relay"]);
    expect(first.page.container.textContent).toContain("skipped: no address typed");
    await enter(first.page.container.querySelector('input[type="url"]'), "https://lan.example");
    first.page.unmount();

    const again = await mounted();
    expect(again.page.container.querySelector("input")?.value).toBe("https://lan.example");
    again.page.unmount();
  });
});
