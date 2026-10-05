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
  readonly isAdminOnly: boolean;
}

const destinations: readonly Destination[] = [
  { route: { screen: "films" }, labelKey: "films", isAdminOnly: false },
  { route: { screen: "series" }, labelKey: "series", isAdminOnly: false },
  { route: { screen: "search", query: "" }, labelKey: "search", isAdminOnly: false },
  { route: { screen: "watch-together" }, labelKey: "watchTogether", isAdminOnly: false },
  { route: { screen: "identify" }, labelKey: "identify", isAdminOnly: true },
  { route: { screen: "libraries" }, labelKey: "libraries", isAdminOnly: true },
  { route: { screen: "people" }, labelKey: "people", isAdminOnly: true },
  { route: { screen: "settings" }, labelKey: "settings", isAdminOnly: true },
  { route: { screen: "jobs" }, labelKey: "jobs", isAdminOnly: true },
  { route: { screen: "system" }, labelKey: "system", isAdminOnly: true },
  { route: { screen: "account" }, labelKey: "account", isAdminOnly: false },
  { route: { screen: "about" }, labelKey: "about", isAdminOnly: false },
];

/**
 * The frame every screen renders in: a banner with the product's name and the menu, and one
 * main landmark holding the screen. The admin's destinations show to an admin only; the server
 * refuses them to anyone else whatever the menu shows.
 */
export function AppShell(props: {
  readonly isAdmin: boolean;
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
              .filter((destination) => props.isAdmin || !destination.isAdminOnly)
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
