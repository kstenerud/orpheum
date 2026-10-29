// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { sendFailuresTo, SilentRequestError } from "@orpheum/sdk";
import { afterEach, describe, expect, it } from "vitest";

import { Play } from "./play.tsx";
import {
  mount,
  playbackCauses,
  servedBy,
  settleUntil,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { directDecision } from "./test-fixtures.ts";
import { withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
  sendFailuresTo(() => undefined);
});

// The owner's Chrome, hosting Watch Together, back on its Play page from another site: the core
// started its playback, the answer never came over WebRTC, and the page said "Loading…" for good, its
// player, and so its host's Watch Together player, never drawn (staging, 2026-10-29).
describe("a Play page whose playback the core answered nothing for", () => {
  it("asks for it again, plays the film, and tells the core's log what happened", async () => {
    const sent: string[] = [];
    sendFailuresTo((text) => sent.push(text));
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "POST /playback": [200, directDecision({ resumePositionMs: 0 })],
        "GET /files/2/waits": [200, {}],
        "GET /files/2/up-next": [200, {}],
        "DELETE /playback/t": [204, null],
      },
      heard,
    );
    services.session.signIn("token", { deviceId: 5 });
    let isAnswered = false;
    services.api.use({
      onRequest: ({ request }) => {
        if (request.method === "POST" && request.url.endsWith("/playback") && !isAnswered) {
          isAnswered = true;
          const silent = new SilentRequestError("/api/v1/playback");
          // As the page's path names it (namingFailures): the request and the path it went over.
          throw new Error(`${silent.message} (POST /api/v1/playback over WebRTC)`, {
            cause: silent,
          });
        }
        return undefined;
      },
    });
    mounted = await mount(withServices(<Play fileId={2} />, services));
    const shown = mounted;
    await settleUntil(() => shown.container.querySelector("video") !== null);
    expect(playbackCauses(heard)).toEqual(["playPage"]);
    expect(sent.some((text) => text.includes("nothing came for /api/v1/playback"))).toBe(true);
  });
});
