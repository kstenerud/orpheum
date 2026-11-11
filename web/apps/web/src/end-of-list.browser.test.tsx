// SPDX-License-Identifier: MPL-2.0

/**
 * The end of a list asking for more as a real browser lays the page out: while it is still two
 * screens below the view, so the next page has come before focus reaches the last row, and again
 * when a page has come and the end is still that near.
 */

import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { act, useState, type JSX } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { page } from "vitest/browser";

import { EndOfList } from "./end-of-list.tsx";
import { mount, settleUntil, type Mounted } from "./test-dom.tsx";

/** The end of a list below a gap of above pixels, telling each ask; a page loads until finished. */
function List(props: {
  readonly above: number;
  readonly onAsked: (finish: () => void) => void;
}): JSX.Element {
  const [isLoadingMore, setIsLoadingMore] = useState(false);
  const { onAsked } = props;
  return (
    <div>
      <div style={{ height: `${String(props.above)}px` }} />
      <EndOfList
        isLoadingMore={isLoadingMore}
        onMore={() => {
          onAsked(() => {
            setIsLoadingMore(false);
          });
          setIsLoadingMore(true);
        }}
      />
    </div>
  );
}

/**
 * Once a watcher of the test's own has been told of target: the browser tells every watcher of an
 * element in the same step, the list's own among them, so whatever the list would ask it has asked.
 */
async function observedOnce(target: Element | null): Promise<void> {
  if (target === null) {
    throw new Error("no end of the list");
  }
  await act(async () => {
    await new Promise<void>((resolve) => {
      const watcher = new IntersectionObserver(() => {
        watcher.disconnect();
        resolve();
      });
      watcher.observe(target);
    });
  });
}

/** The asks of a list: how many came, and the way to finish the last one's page. */
interface Asks {
  readonly count: () => number;
  readonly finish: () => void;
  readonly onAsked: (finish: () => void) => void;
}

/** No ask yet. */
function asks(): Asks {
  let count = 0;
  let finishLast = (): void => undefined;
  return {
    count: () => count,
    finish: () => {
      finishLast();
    },
    onAsked: (finish) => {
      count += 1;
      finishLast = finish;
    },
  };
}

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

describe("the end of a list (the Fire TV's trace, 2026-11-11)", () => {
  it("asks for more while it is still a screen and a half below the view", async () => {
    await page.viewport(960, 480);
    const asked = asks();
    mounted = await mount(<List above={480 * 2.5} onAsked={asked.onAsked} />);
    await settleUntil(() => asked.count() === 1);
    expect(asked.count()).toBe(1);
  });

  it("does not ask while it is further than two screens below the view", async () => {
    await page.viewport(960, 480);
    const asked = asks();
    mounted = await mount(<List above={480 * 4} onAsked={asked.onAsked} />);
    await observedOnce(mounted.container.querySelector(".end-of-list"));
    expect(asked.count()).toBe(0);
  });

  it("asks again when a page has come and it is still that near", async () => {
    await page.viewport(960, 480);
    const asked = asks();
    mounted = await mount(<List above={480} onAsked={asked.onAsked} />);
    await settleUntil(() => asked.count() === 1);
    act(() => {
      asked.finish();
    });
    await settleUntil(() => asked.count() === 2);
    expect(asked.count()).toBe(2);
  });
});
