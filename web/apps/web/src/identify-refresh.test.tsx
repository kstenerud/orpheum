// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";

import { WaitingList } from "./identify.tsx";
import { buttonWithText, click, mount, servedBy, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

const glassTide = {
  id: 7,
  kind: "series" as const,
  name: "Glass Tide",
  identificationState: "ambiguous" as const,
  libraryId: 1,
};

describe("looking a title up again while TMDB is not answering (D231 §12)", () => {
  it("says what it waits for, and since when", async () => {
    const services = servedBy({
      "POST /items/7/refresh": [
        202,
        {
          isQueued: true,
          waitingFor: {
            destination: "tmdb-api",
            name: "TMDB",
            reason: "unreachable",
            downAt: 1_790_000_000_000,
            isChecking: false,
          },
        },
      ],
    });
    mounted = await mount(
      withServices(
        <WaitingList waitingFilter="ambiguous" onChanged={() => undefined} items={[glassTide]} />,
        services,
      ),
    );
    await click(buttonWithText(mounted.container, "Look again"));
    expect(mounted.container.querySelector("output")?.textContent).toMatch(
      /^It will be looked up again once TMDB answers: it has not been answering since .+\.$/u,
    );
  });
});
