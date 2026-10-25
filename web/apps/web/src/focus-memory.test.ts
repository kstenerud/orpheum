// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { afterEach, describe, expect, it } from "vitest";

import {
  asFocusNote,
  createContainerMemory,
  elementOfNote,
  noteOf,
  restoreWhenDrawn,
} from "./focus-memory.ts";

/** A row of two links and two buttons both named Hide, marked as a focus container. */
function row(): HTMLElement {
  const container = document.createElement("ul");
  container.dataset["focusContainer"] = "";
  for (const href of ["/titles/1", "/titles/2"]) {
    const link = document.createElement("a");
    link.href = href;
    link.textContent = href;
    container.append(link);
  }
  for (let index = 0; index < 2; index += 1) {
    const button = document.createElement("button");
    button.textContent = "Hide";
    container.append(button);
  }
  document.body.append(container);
  return container;
}

/** Lets a MutationObserver's records be delivered. */
async function observed(): Promise<void> {
  await new Promise((resolve) => setTimeout(resolve, 0));
}

describe("focus memory", () => {
  afterEach(() => {
    document.body.replaceChildren();
  });

  it("notes a link by its address and anything else by its name and which of the so-named it is", () => {
    const container = row();
    const [, second, , secondHide] = [...container.children] as HTMLElement[];
    if (second === undefined || secondHide === undefined) {
      throw new Error("the row is not drawn");
    }
    expect(noteOf(second, document.body)).toEqual({ href: "/titles/2", ordinal: 0 });
    expect(noteOf(secondHide, document.body)).toEqual({ name: "Hide", ordinal: 1 });
    expect(elementOfNote({ name: "Hide", ordinal: 1 }, document.body)).toBe(secondHide);
    expect(elementOfNote({ href: "/titles/9", ordinal: 0 }, document.body)).toBeUndefined();
  });

  it("reads back only a note it could have written", () => {
    expect(asFocusNote({ href: "/titles/2", ordinal: 0 })).toEqual({
      href: "/titles/2",
      ordinal: 0,
    });
    expect(asFocusNote({ name: "Hide", ordinal: 1 })).toEqual({ name: "Hide", ordinal: 1 });
    expect(asFocusNote({ name: "Hide", ordinal: -1 })).toBeUndefined();
    expect(asFocusNote({ ordinal: 0 })).toBeUndefined();
    expect(asFocusNote("Hide")).toBeUndefined();
    expect(asFocusNote(undefined)).toBeUndefined();
  });

  it("remembers the last element focused in each container around it, while it is still there", () => {
    const outer = document.createElement("main");
    outer.dataset["focusMemory"] = "";
    document.body.append(outer);
    const container = row();
    outer.append(container);
    const memory = createContainerMemory(() => true);
    const second = container.children[1];
    if (second === undefined) {
      throw new Error("the row is not drawn");
    }
    memory.noteFocused(second);
    expect(memory.rememberedIn(container)).toBe(second);
    expect(memory.rememberedIn(outer)).toBe(second);
    second.remove();
    expect(memory.rememberedIn(container)).toBeUndefined();
  });

  it("gives focus to the noted element once the page draws it, as a list read again does", async () => {
    const focused: HTMLElement[] = [];
    const stop = restoreWhenDrawn({ href: "/titles/2", ordinal: 0 }, document.body, (element) => {
      focused.push(element);
    });
    try {
      expect(focused).toEqual([]);
      const container = row();
      await observed();
      expect(focused).toEqual([container.children[1]]);
    } finally {
      stop();
    }
  });

  it("gives up waiting once the person moves focus anywhere first", async () => {
    const focused: HTMLElement[] = [];
    restoreWhenDrawn({ href: "/titles/2", ordinal: 0 }, document.body, (element) => {
      focused.push(element);
    });
    const elsewhere = document.createElement("button");
    document.body.append(elsewhere);
    elsewhere.focus();
    row();
    await observed();
    expect(focused).toEqual([]);
  });
});
