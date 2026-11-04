// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";
import { act, useState, type JSX } from "react";
import { afterEach, expect, it } from "vitest";

import { buttonNamed, mount, settleUntil, type Mounted } from "./test-dom.tsx";
import { TogetherPanel, TogetherSignOverTheFilm } from "./together-over-the-film.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

const signLabel = "Watch Together is on: its controls";

/** A panel holding state of its own, as the voice controls do: a count its button raises. */
function Counting(): JSX.Element {
  const [count, setCount] = useState(0);
  return (
    <button
      type="button"
      className="counting"
      onClick={() => {
        setCount(count + 1);
      }}
    >
      {`Count ${String(count)}`}
    </button>
  );
}

function Player(props: {
  readonly hasPanel: boolean;
  readonly onOpenChange: (isOpen: boolean) => void;
}): JSX.Element {
  return (
    <>
      <div className="page">
        {props.hasPanel ? (
          <TogetherPanel>
            <Counting />
          </TogetherPanel>
        ) : null}
      </div>
      <div className="player-overlay">
        <TogetherSignOverTheFilm onOpenChange={props.onOpenChange} />
      </div>
    </>
  );
}

function sign(): HTMLButtonElement | null {
  return document.querySelector<HTMLButtonElement>(
    `.player-overlay button[aria-label="${signLabel}"]`,
  );
}

it("shows Watch Together's sign over the film only while a session's panel is drawn, blue", async () => {
  mounted = await mount(<Player hasPanel={false} onOpenChange={() => undefined} />);
  expect(sign()).toBeNull();
  mounted.unmount();
  mounted = await mount(<Player hasPanel onOpenChange={() => undefined} />);
  await settleUntil(() => sign() !== null);
  expect(sign()?.classList.contains("icon-button--over-video")).toBe(true);
  expect(sign()?.classList.contains("icon-button--running")).toBe(true);
});

it("moves the session's panel, its state kept, into the dialog the sign opens over the film, and back to its page as it closes", async () => {
  const opens: boolean[] = [];
  mounted = await mount(<Player hasPanel onOpenChange={(isOpen) => opens.push(isOpen)} />);
  await settleUntil(() => sign() !== null);
  const counting = document.querySelector<HTMLButtonElement>(".page .counting");
  if (counting === null) {
    throw new Error("the panel is not drawn on its page");
  }
  await act(async () => {
    counting.click();
    await Promise.resolve();
  });
  await act(async () => {
    sign()?.click();
    await Promise.resolve();
  });
  await settleUntil(
    () => document.querySelector(".player-overlay dialog[open] .counting") !== null,
  );
  const moved = document.querySelector(".player-overlay dialog[open] .counting");
  // The same element, never drawn again: what it held goes on.
  expect(moved).toBe(counting);
  expect(moved?.textContent).toBe("Count 1");
  expect(document.querySelector(".page .counting")).toBeNull();
  expect(opens).toEqual([true]);
  const dialog = document.querySelector<HTMLDialogElement>(".player-overlay dialog[open]");
  if (dialog === null) {
    throw new Error("no dialog over the film");
  }
  await act(async () => {
    buttonNamed(dialog, "Close")?.click();
    await Promise.resolve();
  });
  await settleUntil(() => document.querySelector(".page .counting") !== null);
  expect(document.querySelector(".page .counting")).toBe(counting);
  expect(counting.textContent).toBe("Count 1");
  expect(opens).toEqual([true, false]);
});
