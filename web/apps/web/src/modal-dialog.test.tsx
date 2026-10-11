// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { act, useState, type JSX } from "react";
import { describe, expect, it, vi } from "vitest";

import { ModalDialog } from "./modal-dialog.tsx";
import { fire, mount } from "./test-dom.tsx";

describe("a modal dialog (the owner, 2026-10-11)", () => {
  it("opens as the browser's modal, and Escape closes it without reaching the page", async () => {
    const showModal = vi.spyOn(HTMLDialogElement.prototype, "showModal");
    const onClose = vi.fn();
    const pageHeard = vi.fn();
    document.addEventListener("keydown", pageHeard);
    const mounted = await mount(
      <ModalDialog label="Gran" onClose={onClose}>
        <p>inside</p>
      </ModalDialog>,
    );
    const dialog = mounted.container.querySelector("dialog");
    expect(showModal).toHaveBeenCalledOnce();
    expect(dialog?.open).toBe(true);
    expect(dialog?.getAttribute("aria-label")).toBe("Gran");
    await fire(dialog, new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    expect(pageHeard).not.toHaveBeenCalled();
    await fire(dialog, new Event("cancel", { cancelable: true }));
    expect(onClose).toHaveBeenCalledOnce();
    mounted.unmount();
    document.removeEventListener("keydown", pageHeard);
    showModal.mockRestore();
  });

  it("stays open as its page renders again with a new onClose, its focus kept", async () => {
    const showModal = vi.spyOn(HTMLDialogElement.prototype, "showModal");
    const close = vi.spyOn(HTMLDialogElement.prototype, "close");
    let renderAgain: () => void = () => undefined;
    function Page(): JSX.Element {
      const [count, setCount] = useState(0);
      renderAgain = () => {
        setCount(count + 1);
      };
      return (
        <ModalDialog label="Gran" onClose={() => undefined}>
          <p>{count}</p>
        </ModalDialog>
      );
    }
    const mounted = await mount(<Page />);
    await act(async () => {
      renderAgain();
      await Promise.resolve();
    });
    expect(mounted.container.querySelector("p")?.textContent).toBe("1");
    expect(showModal).toHaveBeenCalledOnce();
    expect(close).not.toHaveBeenCalled();
    mounted.unmount();
    showModal.mockRestore();
    close.mockRestore();
  });
});
