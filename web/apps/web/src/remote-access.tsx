// SPDX-License-Identifier: MPL-2.0

import "./system.css";

import type { JSX } from "react";

import { SystemDiscovery } from "./system-discovery.tsx";

/**
 * Remote access, a page of its own (D528 §1): linking the server to a discovery, so the household
 * reaches it from outside the house. It was the System page's last section, where the first-time
 * pages' link dropped the admin at the top of a long page about something else (the OpenMediaVault
 * mock install, 2026-11-20).
 */
export function RemoteAccess(): JSX.Element {
  return (
    <div className="system">
      <SystemDiscovery />
    </div>
  );
}
