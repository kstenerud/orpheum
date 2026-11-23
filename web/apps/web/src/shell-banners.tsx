// SPDX-License-Identifier: MPL-2.0

import type { JSX } from "react";

import { Banners, type BannerNotice } from "./banner.tsx";
import { useLinkCapNotice } from "./link-debug-tools.tsx";
import { useLocalNetworkNotice } from "./local-network-note.tsx";
import { usePlainHttpNotice } from "./plain-http-note.tsx";
import { useSecurityUpdateNotice } from "./security-update-note.tsx";
import { useWorkerBypassedNotice } from "./worker-bypassed-note.tsx";

/**
 * The shell's banners, at the top of its main area under the top bar (D538 §1): every notice the
 * page has about the server or the browser, in the player spec's order (a security update pending;
 * then for testers, the worker bypassed and the tab's rate capped; then the page over plain http, and
 * the browser asking for the local network), at most two at once (Banners). While a film fills the
 * window they wait until it leaves it, hidden with the page beneath the film (player-overlay.css);
 * the loader's Lost the connection is the one that shows over a film (loader-path-refused.css).
 */
export function ShellBanners(props: { readonly isAdmin: boolean }): JSX.Element | null {
  const notices = [
    useSecurityUpdateNotice(props.isAdmin),
    useWorkerBypassedNotice(),
    useLinkCapNotice(),
    usePlainHttpNotice(),
    useLocalNetworkNotice(),
  ].filter((notice): notice is BannerNotice => notice !== undefined);
  return <Banners notices={notices} />;
}
