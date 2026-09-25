// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { createApi } from "@orpheum/sdk";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { PlayView, requestPlayback } from "./play.tsx";
import { testServices, withServices } from "./test-services.tsx";

describe("playing a file", () => {
  it("asks the server with this browser's menu, over the local link", async () => {
    const menu = testServices().menu();
    let sent: unknown;
    const api = createApi({ origin: "http://orpheum.test", token: () => "token" });
    // Answered here, before any request leaves: tests never touch the network.
    api.use({
      async onRequest({ request }) {
        sent = await request.json();
        return new Response(JSON.stringify({ kind: "direct", url: "/u", reasons: [] }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      },
    });
    const decision = await requestPlayback(api, 7, menu);
    expect(sent).toEqual({ fileId: 7, menu, link: "local" });
    expect(decision.kind).toBe("direct");
  });

  it("plays the original in a video element and says so", () => {
    const markup = renderToStaticMarkup(
      withServices(
        <PlayView
          decision={{ kind: "direct", url: "/api/v1/bytes/media/direct/abc", reasons: [] }}
        />,
      ),
    );
    expect(markup).toContain('src="/api/v1/bytes/media/direct/abc"');
    expect(markup).toContain("playsInline");
    expect(markup).toContain("Playing the original file, as it is.");
  });

  it("names every reason a file needs preparing", () => {
    const markup = renderToStaticMarkup(
      withServices(
        <PlayView
          decision={{ kind: "hls", profile: "remux", reasons: ["container", "audio_codec"] }}
        />,
      ),
    );
    expect(markup).toContain(
      "the browser cannot open its container, the browser cannot decode its sound",
    );
    expect(markup).toContain("not built yet");
  });
});
