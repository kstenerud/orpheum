// SPDX-License-Identifier: MPL-2.0

import "./app-shell.css";

import type { JSX, ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { Link } from "./link.tsx";
import { productName } from "./product-name.ts";
import type { Route } from "./router.ts";

/** A place the menu leads to, and whether this person may go there. */
interface Destination {
  readonly route: Route;
  /** The key of the destination's name in the catalogue, under nav. */
  readonly labelKey:
    | "films"
    | "series"
    | "search"
    | "watchTogether"
    | "identify"
    | "libraries"
    | "people"
    | "settings"
    | "jobs"
    | "system"
    | "account"
    | "about";
  /** Who it shows to: everyone, an admin, or a person whose role may host Watch Together. */
  readonly audience: "everyone" | "admin" | "host";
}

const destinations: readonly Destination[] = [
  { route: { screen: "films" }, labelKey: "films", audience: "everyone" },
  { route: { screen: "series" }, labelKey: "series", audience: "everyone" },
  { route: { screen: "search", query: "" }, labelKey: "search", audience: "everyone" },
  { route: { screen: "watch-together" }, labelKey: "watchTogether", audience: "host" },
  { route: { screen: "identify" }, labelKey: "identify", audience: "admin" },
  { route: { screen: "libraries" }, labelKey: "libraries", audience: "admin" },
  { route: { screen: "people" }, labelKey: "people", audience: "admin" },
  { route: { screen: "settings" }, labelKey: "settings", audience: "admin" },
  { route: { screen: "jobs" }, labelKey: "jobs", audience: "admin" },
  { route: { screen: "system" }, labelKey: "system", audience: "admin" },
  { route: { screen: "account" }, labelKey: "account", audience: "everyone" },
  { route: { screen: "about" }, labelKey: "about", audience: "everyone" },
];

/**
 * The frame every screen renders in: a banner with the product's name and the menu, and one
 * main landmark holding the screen. The admin's destinations show to an admin only, and Watch
 * Together to a person whose role may host; the server refuses them to anyone else whatever the
 * menu shows.
 */
export function AppShell(props: {
  readonly isAdmin: boolean;
  /** Whether the person's role may host Watch Together, which shows them its screen. */
  readonly canHostWatchTogether: boolean;
  readonly currentScreen: Route["screen"];
  readonly onSignOut: () => void;
  /** Who is signed in, and on which device, once known: said in the banner. */
  readonly signedInAs:
    { readonly name: string; readonly deviceName: string | undefined } | undefined;
  readonly children: ReactNode;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <div className="app-shell">
      <header className="app-shell__banner">
        <p className="app-shell__title">{productName}</p>
        <nav className="app-shell__nav" aria-label={t("nav.label")}>
          <ul>
            {destinations
              .filter(
                (destination) =>
                  destination.audience === "everyone" ||
                  (destination.audience === "admin" && props.isAdmin) ||
                  (destination.audience === "host" && props.canHostWatchTogether),
              )
              .map((destination) => (
                <li
                  key={destination.labelKey}
                  aria-current={
                    destination.route.screen === props.currentScreen ? "page" : undefined
                  }
                >
                  <Link to={destination.route}>{t(`nav.${destination.labelKey}`)}</Link>
                </li>
              ))}
          </ul>
          {props.signedInAs === undefined ? null : (
            <span className="app-shell__who">
              {props.signedInAs.deviceName === undefined
                ? t("nav.signedInAs", { name: props.signedInAs.name })
                : t("nav.signedInAsOn", {
                    name: props.signedInAs.name,
                    device: props.signedInAs.deviceName,
                  })}
            </span>
          )}
          <button type="button" onClick={props.onSignOut}>
            {t("nav.signOut")}
          </button>
        </nav>
      </header>
      <main className="app-shell__main">{props.children}</main>
    </div>
  );
}
