// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act, useState, type JSX } from "react";
import { describe, expect, it, vi } from "vitest";

import { DialogClose, ModalDialog } from "./modal-dialog.tsx";
import { GrownFromOpener } from "./popover.tsx";
import { fire, mount } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

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

  // D538 §2: no dialog has a cross; one with nothing to do has Done at its foot (ui.md rule 6).
  it("has no cross, and one with nothing to do closes by the Done at its foot", async () => {
    const onClose = vi.fn();
    const mounted = await mount(
      withServices(
        <ModalDialog label="Gran" onClose={onClose} footer={<DialogClose onClose={onClose} />}>
          <button type="button">inside</button>
        </ModalDialog>,
      ),
    );
    const buttons = [
      ...(mounted.container.querySelector("dialog")?.querySelectorAll("button") ?? []),
    ];
    expect(
      buttons.map((button) => button.getAttribute("aria-label") ?? button.textContent),
    ).toEqual(["inside", "Done"]);
    expect(buttons[1]?.closest("footer")).not.toBeNull();
    await fire(buttons[1], new MouseEvent("click", { bubbles: true }));
    expect(onClose).toHaveBeenCalledOnce();
    mounted.unmount();
  });

  // The owner, 2026-11-23: Watch Together's and Cast's dialogs over the film grow out of their
  // buttons as the menus do (GrownFromContext), a dialog opened from inside one drawn as dialogs are.
  it("grows out of the button it is opened under, its tab closing it, a dialog opened inside it drawn as dialogs are", async () => {
    const onClose = vi.fn();
    const openerRef = { current: document.createElement("button") };
    document.body.append(openerRef.current);
    const mounted = await mount(
      withServices(
        <GrownFromOpener openerRef={openerRef} tab={<span>tab sign</span>}>
          <ModalDialog label="Outer" onClose={onClose}>
            <p>inside</p>
            <ModalDialog label="Inner" onClose={() => undefined}>
              <p>nested</p>
            </ModalDialog>
          </ModalDialog>
        </GrownFromOpener>,
      ),
    );
    const outer = mounted.container.querySelector<HTMLDialogElement>("dialog[aria-label='Outer']");
    const inner = mounted.container.querySelector("dialog[aria-label='Inner']");
    expect(outer?.classList.contains("modal-dialog--grown")).toBe(true);
    const tab = outer?.querySelector(".popover__tab");
    expect(tab?.getAttribute("aria-hidden")).toBe("true");
    expect(tab?.textContent).toBe("tab sign");
    expect(outer?.querySelector(".modal-dialog__panel")?.textContent).toContain("inside");
    expect(inner?.classList.contains("modal-dialog--grown")).toBe(false);
    expect(inner?.querySelector(".popover__tab")).toBeNull();
    await fire(tab?.firstElementChild, new MouseEvent("click", { bubbles: true }));
    expect(onClose).toHaveBeenCalledOnce();
    mounted.unmount();
    openerRef.current.remove();
  });

  it("is drawn as dialogs are where the button it is under is undefined, a sign not over the film", async () => {
    const mounted = await mount(
      withServices(
        <GrownFromOpener openerRef={undefined} tab={<span>tab sign</span>}>
          <ModalDialog label="Plain" onClose={() => undefined}>
            <p>inside</p>
          </ModalDialog>
        </GrownFromOpener>,
      ),
    );
    const dialog = mounted.container.querySelector("dialog");
    expect(dialog?.classList.contains("modal-dialog--grown")).toBe(false);
    expect(dialog?.querySelector(".popover__tab")).toBeNull();
    mounted.unmount();
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

  it("closes on a remote's Back, as on Escape, before Back goes anywhere", async () => {
    const onClose = vi.fn();
    const mounted = await mount(
      withServices(
        <ModalDialog label="Gran" onClose={onClose}>
          <button type="button">inside</button>
        </ModalDialog>,
      ),
    );
    const inside = mounted.container.querySelector("button");
    act(() => {
      inside?.focus();
    });
    const back = new KeyboardEvent("keydown", {
      key: "XF86Back",
      keyCode: 10_009,
      bubbles: true,
      cancelable: true,
    });
    await fire(inside, back);
    expect(onClose).toHaveBeenCalledOnce();
    expect(back.defaultPrevented).toBe(true);
    mounted.unmount();
  });
});
