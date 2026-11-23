// SPDX-License-Identifier: MPL-2.0

import "./system.css";

import type { JSX } from "react";

import { SystemDiscovery } from "./system-discovery.tsx";

/**
 * Network, a page of its own (D528 §1, named so by D538 §11): linking the server to a discovery, so the household
 * reaches it from outside the house. It was the System page's last section, where the first-time
 * pages' link dropped the admin at the top of a long page about something else (the OpenMediaVault
 * mock install, 2026-11-20).
 */
export function Network(): JSX.Element {
  return (
    <div className="system">
      <SystemDiscovery />
    </div>
  );
}
