// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { createElement } from "react";
import { describe, expect, it } from "vitest";

import { mount } from "./test-dom.tsx";
import * as trayDialogs from "./tray-dialog-code.tsx";
import { closeTrayDialog, currentTrayDialog, openTrayDialog } from "./tray-dialog.ts";

describe("the tray's dialogs in a test (the shared module state note, test-architecture-2026-10-26.md)", () => {
  it("meet each mount cold and closed, never as an earlier test left them", async () => {
    const before = [trayDialogs.CastDialog, trayDialogs.TogetherDialog];
    openTrayDialog("together");
    const mounted = await mount(createElement("div"));
    expect([trayDialogs.CastDialog, trayDialogs.TogetherDialog]).not.toContain(before[0]);
    expect(trayDialogs.TogetherDialog).not.toBe(before[1]);
    expect(currentTrayDialog()).toBeUndefined();
    mounted.unmount();
    closeTrayDialog();
  });
});
