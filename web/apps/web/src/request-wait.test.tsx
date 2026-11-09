// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { ApiProblem } from "./problem.ts";
import { RequestWait } from "./request-wait.tsx";
import { buttonWithText, click, mount, type Mounted } from "./test-dom.tsx";
import { useRequest } from "./use-request.ts";

let mounted: Mounted | undefined;

beforeEach(() => {
  // Time passes on its own too, a millisecond at a time, so the test's own settling, which waits
  // on a timer, settles at once rather than in real 20 ms steps (the test audit, 2026-10-08).
  vi.useFakeTimers({
    toFake: ["setTimeout", "clearTimeout", "Date"],
    shouldAdvanceTime: true,
    advanceTimeDelta: 1,
  });
});

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
  vi.useRealTimers();
});

/** A screen that asks TMDB with request each time its button is pressed. */
function Asker(props: {
  readonly request: (idempotencyKey: string) => Promise<void>;
}): React.JSX.Element {
  const state = useRequest(() => "it failed");
  return (
    <div>
      <button
        type="button"
        onClick={() => {
          state.runRequest(props.request);
        }}
      >
        Ask
      </button>
      <RequestWait requestState={state} provider="TMDB" />
      {state.problem === undefined ? null : <p role="alert">{state.problem}</p>}
    </div>
  );
}

/** Lets time pass, and what it set off settle. */
async function pass(ms: number): Promise<void> {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms);
  });
}

describe("a person's request held back says why and until when (D231 §12)", () => {
  it("counts down a host's wait, then asks again on its own", async () => {
    let asked = 0;
    const request = (): Promise<void> => {
      asked++;
      if (asked === 1) {
        return Promise.reject(
          new ApiProblem({
            code: "rateLimited",
            message: "TMDB asks us to wait",
            detail: { retryAfterMs: 3000 },
          }),
        );
      }
      return Promise.resolve();
    };
    mounted = await mount(<Asker request={request} />);
    await click(buttonWithText(mounted.container, "Ask"));
    expect(mounted.container.textContent).toContain("TMDB asked us to wait. Trying again in 3 s…");
    await pass(1000);
    expect(mounted.container.textContent).toContain("Trying again in 2 s…");
    await pass(2000);
    expect(asked).toBe(2);
    expect(mounted.container.textContent).not.toContain("asked us to wait");
    expect(mounted.container.querySelector('[role="alert"]')).toBeNull();
  });

  it("says it is still waiting past a couple of seconds", async () => {
    let finish = (): void => undefined;
    const request = (): Promise<void> =>
      new Promise((resolve) => {
        finish = () => {
          resolve();
        };
      });
    mounted = await mount(<Asker request={request} />);
    await click(buttonWithText(mounted.container, "Ask"));
    expect(mounted.container.textContent).not.toContain("Still waiting");
    await pass(2000);
    expect(mounted.container.textContent).toContain("Still waiting for TMDB…");
    await act(async () => {
      finish();
      await Promise.resolve();
    });
    expect(mounted.container.textContent).not.toContain("Still waiting");
  });

  it("calls the next try after a host was not answering its check", async () => {
    let finish = (): void => undefined;
    let asked = 0;
    const request = async (): Promise<void> => {
      asked++;
      if (asked === 1) {
        throw new ApiProblem({
          code: "providerError",
          message: "not answering",
          detail: { reason: "unavailable" },
        });
      }
      await new Promise<void>((resolve) => {
        finish = resolve;
      });
    };
    mounted = await mount(<Asker request={request} />);
    await click(buttonWithText(mounted.container, "Ask"));
    expect(mounted.container.querySelector('[role="alert"]')?.textContent).toBe("it failed");
    await click(buttonWithText(mounted.container, "Ask"));
    expect(mounted.container.textContent).toContain("Checking whether TMDB answers now…");
    await act(async () => {
      finish();
      await Promise.resolve();
    });
    expect(mounted.container.textContent).not.toContain("Checking");
  });

  it("gives a click's retry the click's key, and the next click a key of its own (D244 §4)", async () => {
    const keys: string[] = [];
    const request = (idempotencyKey: string): Promise<void> => {
      keys.push(idempotencyKey);
      if (keys.length === 1) {
        return Promise.reject(
          new ApiProblem({
            code: "rateLimited",
            message: "TMDB asks us to wait",
            detail: { retryAfterMs: 1000 },
          }),
        );
      }
      return Promise.resolve();
    };
    mounted = await mount(<Asker request={request} />);
    await click(buttonWithText(mounted.container, "Ask"));
    await pass(1000);
    await click(buttonWithText(mounted.container, "Ask"));
    expect(keys).toHaveLength(3);
    expect(keys[1]).toBe(keys[0]);
    expect(keys[2]).not.toBe(keys[0]);
  });
});
