// SPDX-License-Identifier: MPL-2.0

import type { JSX } from "react";

import { Icon } from "./icon.tsx";

/** A sign's mark: blue and pulsing while its function is on, the banner's white while it is off. */
export function signMark(isOn: boolean): "running" | undefined {
  return isOn ? "running" : undefined;
}

/**
 * A screen playing, two people before it: Watch Together, in the colour of the text it sits in. The
 * owner's choice, drawn by Claude Design (2026-10-12).
 */
export function TogetherIcon(): JSX.Element {
  return (
    <Icon className="status-tray__icon">
      <path
        fillRule="evenodd"
        d="M4 2h16a2 2 0 0 1 2 2v7a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2zm6 2.8v5.4l4.6-2.7z"
      />
      <circle cx="7.5" cy="17.3" r="2.3" />
      <circle cx="16.5" cy="17.3" r="2.3" />
      <path d="M3.2 23a4.3 3.6 0 0 1 8.6 0z" />
      <path d="M12.2 23a4.3 3.6 0 0 1 8.6 0z" />
    </Icon>
  );
}
