// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";

import { browserPlaceOf, OpenInBrowser } from "./open-in-browser.tsx";
import { buttonWithText, click, mount, settleUntil, type Mounted } from "./test-dom.tsx";

/** User agents as each browser sends them. */
const agents = {
  iosSafari:
    "Mozilla/5.0 (iPhone; CPU iPhone OS 18_6 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.6 Mobile/15E148 Safari/604.1",
  iosChrome:
    "Mozilla/5.0 (iPhone; CPU iPhone OS 18_6 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/141.0.7390.41 Mobile/15E148 Safari/604.1",
  iosWebView:
    "Mozilla/5.0 (iPhone; CPU iPhone OS 18_6 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148",
  iosInstagram:
    "Mozilla/5.0 (iPhone; CPU iPhone OS 18_6 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148 Instagram 400.0.0.0.0",
  androidChrome:
    "Mozilla/5.0 (Linux; Android 14; Pixel 3a) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Mobile Safari/537.36",
  androidTelegram:
    "Mozilla/5.0 (Linux; Android 14; Pixel 3a; wv) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/141.0.0.0 Mobile Safari/537.36 Telegram-Android/12.0.1",
  androidWebView:
    "Mozilla/5.0 (Linux; Android 14; Pixel 3a; wv) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/141.0.0.0 Mobile Safari/537.36",
  desktopFirefox: "Mozilla/5.0 (X11; Linux x86_64; rv:144.0) Gecko/20100101 Firefox/144.0",
};

const link = "https://k7xq2.orpheum-client-test.com/#link=abc";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

describe("where a page runs", () => {
  it("finds a web view inside an app, and the browser to open the link in instead", () => {
    expect(browserPlaceOf(agents.iosWebView)).toEqual({
      isInApp: true,
      app: undefined,
      ownBrowser: "safari",
    });
    expect(browserPlaceOf(agents.iosInstagram)).toEqual({
      isInApp: true,
      app: "Instagram",
      ownBrowser: "safari",
    });
    expect(browserPlaceOf(agents.androidTelegram)).toEqual({
      isInApp: true,
      app: "Telegram",
      ownBrowser: "chrome",
    });
    expect(browserPlaceOf(agents.androidWebView).isInApp).toBe(true);
  });

  it("finds no app around a phone's own browsers or a desktop's", () => {
    expect(browserPlaceOf(agents.iosSafari).isInApp).toBe(false);
    expect(browserPlaceOf(agents.iosChrome).isInApp).toBe(false);
    expect(browserPlaceOf(agents.androidChrome).isInApp).toBe(false);
    expect(browserPlaceOf(agents.desktopFirefox)).toEqual({
      isInApp: false,
      app: undefined,
      ownBrowser: "other",
    });
  });
});

describe("the page a browser running no service worker shows", () => {
  // A guest in Berlin, 2026-11-02, opened her link from Telegram and was told only that "this
  // browser" could not play; Safari, which she tried herself, played.
  it("tells a page inside an app to open the link in Safari, with the link to copy", async () => {
    const copied: string[] = [];
    mounted = await mount(
      <OpenInBrowser
        address={link}
        userAgent={agents.iosWebView}
        copy={(text) => {
          copied.push(text);
          return Promise.resolve();
        }}
      />,
    );
    const page = mounted.container;
    expect(page.querySelector("h1")?.textContent).toBe("Open this link in Safari");
    expect(page.textContent).toContain("This link opened inside another app");
    expect(page.textContent).toContain("Open in Safari");
    await click(buttonWithText(page, "Copy link"));
    await settleUntil(() => page.textContent.includes("Copied"));
    expect(copied).toEqual([link]);
  });

  it("names the app where its browser tells, and Chrome on Android", async () => {
    mounted = await mount(
      <OpenInBrowser
        address={link}
        userAgent={agents.androidTelegram}
        copy={() => Promise.resolve()}
      />,
    );
    const page = mounted.container;
    expect(page.querySelector("h1")?.textContent).toBe("Open this link in Chrome");
    expect(page.textContent).toContain("This link opened inside Telegram");
    expect(page.textContent).not.toContain("Open in Safari");
  });

  it("shows the link to select where the app will not copy it", async () => {
    mounted = await mount(
      <OpenInBrowser
        address={link}
        userAgent={agents.iosWebView}
        copy={() => Promise.reject(new Error("refused"))}
      />,
    );
    const page = mounted.container;
    expect(page.querySelector("input")).toBeNull();
    await click(buttonWithText(page, "Copy link"));
    await settleUntil(() => page.querySelector("input") !== null);
    expect(page.querySelector("input")?.value).toBe(link);
  });

  it("tells a browser in no app, as a private window, to use an ordinary window", async () => {
    mounted = await mount(<OpenInBrowser address={link} userAgent={agents.desktopFirefox} />);
    const page = mounted.container;
    expect(page.querySelector("h1")?.textContent).toBe("This browser can't play from the server");
    expect(buttonWithText(page, "Copy link")).toBeUndefined();
  });
});
