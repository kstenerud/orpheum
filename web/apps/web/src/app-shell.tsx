// SPDX-License-Identifier: MPL-2.0

import "./app-shell.css";

import { useEffect, useRef, type JSX, type ReactNode, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import logo from "./assets/orpheum-logo.svg";
import { Icon } from "./icon.tsx";
import { useCommands } from "./input-layer.tsx";
import { StatusTray } from "./status-tray.tsx";
import { Link } from "./link.tsx";
import { LocalNetworkNote } from "./local-network-note.tsx";
import { PlainHttpNote } from "./plain-http-note.tsx";
import { productName } from "./product-name.ts";
import type { Route } from "./router.ts";

/** The logo, the way home, underlined while home is the page (the owner, 2026-10-25). */
/** The laurel alone, which the page's favicon is, served beside the client (index.html). */
const markOnly = "/favicon.svg";
/** A phone's width, as app-shell.css's own breakpoint. */
const PHONE_WIDTH = "(max-width: 599px)";

function HomeLogo(props: { readonly isHome: boolean }): JSX.Element {
  return (
    <p className="app-shell__title">
      {/* The logo is the way home, as on most sites, and the banner's mark of being there
          (the owner, 2026-10-25: no Home entry; the logo, underlined at home). */}
      <Link to={{ screen: "home" }} className="app-shell__home" isCurrent={props.isHome}>
        {/* On a phone, the laurel alone, the favicon's mark: the wordmark beside the bar's end ran
            into it (the owner, 2026-10-28: "reduce the orpheum text to just the logo when the screen
            is small"). */}
        <picture>
          <source media={PHONE_WIDTH} srcSet={markOnly} />
          <img className="app-shell__logo" src={logo} alt={productName} />
        </picture>
      </Link>
    </p>
  );
}

/**
 * Keeps the banner's height, which its content sets (one row from a tablet up, two on a phone), in
 * the page's --banner-height, for what stands under the sticky banner to size itself by: the
 * alphabet rail runs from under it to the window's bottom (the owner, 2026-10-25). Returns the
 * banner's ref.
 */
function useBannerHeight(): RefObject<HTMLElement | null> {
  const bannerRef = useRef<HTMLElement>(null);
  useEffect(() => {
    const banner = bannerRef.current;
    if (banner === null || typeof ResizeObserver === "undefined") {
      return undefined;
    }
    const root = document.documentElement;
    const keep = (): void => {
      root.style.setProperty("--banner-height", `${String(banner.offsetHeight)}px`);
    };
    keep();
    const watching = new ResizeObserver(keep);
    watching.observe(banner);
    return () => {
      watching.disconnect();
    };
  }, []);
  return bannerRef;
}

/** A place the menu leads to, and whether this person may go there. */
interface Destination {
  readonly route: Route;
  /** The key of the destination's name in the catalogue, under nav. */
  readonly labelKey:
    | "films"
    | "series"
    | "deviceTests"
    | "identify"
    | "libraries"
    | "people"
    | "settings"
    | "jobs"
    | "system"
    | "thisDevice"
    | "account"
    | "about"
    | "uiGallery";
  /**
   * Who it shows to: everyone, an admin, anyone while the admin has the pages for development turned
   * on, or anyone while the admin offers the device tests.
   */
  readonly audience: "everyone" | "admin" | "developer" | "tester";
  /**
   * Where in the banner it is: the places for watching, always in view; or, in the one menu at the
   * banner's end, the admin's area under Manage, the pages for development, or the person's own
   * (the owner, 2026-10-11 and 2026-10-17).
   */
  readonly group: "watching" | "manage" | "develop" | "own";
}

const destinations: readonly Destination[] = [
  { route: { screen: "films" }, labelKey: "films", audience: "everyone", group: "watching" },
  { route: { screen: "series" }, labelKey: "series", audience: "everyone", group: "watching" },
  {
    // The Libraries page (D349 §9.1), which replaces Search in the top bar; its own labelKey is
    // the admin's "libraries" (Manage), reused: both say "Libraries", each at its own place.
    route: { screen: "browse" },
    labelKey: "libraries",
    audience: "everyone",
    group: "watching",
  },
  {
    // In the menu, not the places for watching: on a phone the row ran past the window (the owner,
    // 2026-10-28: "the device tests could be moved into the menu").
    route: { screen: "lab" },
    labelKey: "deviceTests",
    audience: "tester",
    group: "own",
  },
  { route: { screen: "identify" }, labelKey: "identify", audience: "admin", group: "manage" },
  { route: { screen: "libraries" }, labelKey: "libraries", audience: "admin", group: "manage" },
  { route: { screen: "people" }, labelKey: "people", audience: "admin", group: "manage" },
  { route: { screen: "settings" }, labelKey: "settings", audience: "admin", group: "manage" },
  { route: { screen: "jobs" }, labelKey: "jobs", audience: "admin", group: "manage" },
  { route: { screen: "system" }, labelKey: "system", audience: "admin", group: "manage" },
  {
    route: { screen: "ui-gallery" },
    labelKey: "uiGallery",
    audience: "developer",
    group: "develop",
  },
  { route: { screen: "this-device" }, labelKey: "thisDevice", audience: "everyone", group: "own" },
  { route: { screen: "account" }, labelKey: "account", audience: "everyone", group: "own" },
  { route: { screen: "about" }, labelKey: "about", audience: "everyone", group: "own" },
];

/**
 * The frame every screen renders in: a banner with the product's name and the menu, and one
 * main landmark holding the screen. The places for watching are always in view; everything else is
 * in one menu behind a menu icon at the banner's end (the owner, 2026-10-11: "consolidate all the
 * extra functionality into a single pull-down menu on the right"): who is signed in, the admin's
 * destinations under Manage, shown to an admin only, the person's own places, Device tests among them
 * while the admin offers them, and Sign out. Watch Together and Cast are no places here:
 * their signs in the status tray are their entry points (D364 §12). The server refuses a destination
 * to anyone else whatever the menu shows.
 */
export function AppShell(props: {
  readonly isAdmin: boolean;
  /** Whether the person's role may host Watch Together, which shows them its sign while it is off. */
  readonly canHostWatchTogether: boolean;
  readonly currentScreen: Route["screen"];
  readonly onSignOut: () => void;
  /** Who is signed in, and on which device, once known: said in the banner. */
  readonly signedInAs:
    { readonly name: string; readonly deviceName: string | undefined } | undefined;
  /** The realm the person signed in to the server by, when they did: shown in the banner (D297 §5). */
  readonly realm?: string | undefined;
  /**
   * Whether the menu offers the pages for development, such as the UI gallery: a server setting the
   * admin turns on (the owner, 2026-10-17: "development items behind a setting").
   */
  readonly showsDeveloperPages?: boolean;
  /**
   * Whether the places for watching offer the device lab's tests, beside Watch Together: a server
   * setting the admin turns on (the owner, 2026-10-25: for running device tests with beta
   * participants).
   */
  readonly showsDeviceTests?: boolean;
  /**
   * Goes to the discovery's list of households, offered in the menu while a discovery's loader runs
   * the page (D308 §8); undefined, and the menu offers none.
   */
  readonly onSwitchHousehold?: (() => void) | undefined;
  /**
   * For an admin who does not administer here, away from home (D297 §6): the server's home address,
   * where they do, or "" when it has none ready; undefined for anyone else. Under Manage the menu
   * then says so, in place of places it cannot offer.
   */
  readonly administersFromHomeAt?: string | undefined;
  readonly children: ReactNode;
}): JSX.Element {
  const { t } = useTranslation();
  const shown = destinations.filter((destination) => isShownTo(destination.audience, props));
  const inGroup = (group: Destination["group"]): readonly Destination[] =>
    shown.filter((destination) => destination.group === group);
  const links = (group: Destination["group"]): JSX.Element[] =>
    inGroup(group).map((destination) => (
      <Place
        key={`${destination.group}-${destination.labelKey}`}
        destination={destination}
        isCurrent={destination.route.screen === props.currentScreen}
      />
    ));
  const isIn = (group: Destination["group"]): boolean =>
    inGroup(group).some((destination) => destination.route.screen === props.currentScreen);
  const bannerRef = useBannerHeight();
  return (
    <div className="app-shell">
      <header
        ref={bannerRef}
        className="app-shell__banner"
        data-focus-container=""
        data-focus-covers-top=""
      >
        <HomeLogo isHome={props.currentScreen === "home"} />
        <nav className="app-shell__nav" aria-label={t("nav.label")}>
          <ul className="app-shell__watching">{links("watching")}</ul>
          <BannerEnd shell={props} isMenuCurrent={isIn("manage") || isIn("develop") || isIn("own")}>
            <MenuSection
              heading={t("nav.manage")}
              places={managePlaces(props.administersFromHomeAt, links("manage"))}
            />
            <MenuSection heading={t("nav.develop")} places={links("develop")} />
            {links("own")}
          </BannerEnd>
        </nav>
      </header>
      <ShellMain>{props.children}</ShellMain>
    </div>
  );
}

/** Whether the shell shows a destination for audience to the person it is drawn for. */
function isShownTo(
  audience: Destination["audience"],
  shell: Parameters<typeof AppShell>[0],
): boolean {
  switch (audience) {
    case "everyone":
      return true;
    case "admin":
      return shell.isAdmin;
    case "developer":
      return shell.showsDeveloperPages === true;
    case "tester":
      return shell.showsDeviceTests === true;
  }
}

/** The screen, under any word the page has about what the browser is doing. */
function ShellMain(props: { readonly children: ReactNode }): JSX.Element {
  return (
    <main className="app-shell__main" data-focus-memory="">
      <PlainHttpNote />
      <LocalNetworkNote />
      {props.children}
    </main>
  );
}

/**
 * The banner's end: the status tray (D280), then the one menu with its places, kept together so
 * the tray sits just before the menu at every width (the owner, 2026-10-11: on a narrow window it
 * had fallen into the row of places for watching).
 */
function BannerEnd(props: {
  readonly shell: Parameters<typeof AppShell>[0];
  readonly isMenuCurrent: boolean;
  readonly children: ReactNode;
}): JSX.Element {
  const { shell } = props;
  return (
    <div className="app-shell__end">
      {shell.realm === undefined ? null : <RealmMark realm={shell.realm} />}
      <StatusTray isAdmin={shell.isAdmin} canHostWatchTogether={shell.canHostWatchTogether} />
      <MainMenu
        signedInAs={shell.signedInAs}
        realm={shell.realm}
        isCurrent={props.isMenuCurrent}
        onSignOut={shell.onSignOut}
        onSwitchHousehold={shell.onSwitchHousehold}
      >
        {props.children}
      </MainMenu>
    </div>
  );
}

/** A titled part of the menu and the divider after it, or nothing when it has no place. */
function MenuSection(props: {
  readonly heading: string;
  readonly places: readonly JSX.Element[];
}): JSX.Element | null {
  if (props.places.length === 0) {
    return null;
  }
  return (
    <>
      <li className="app-shell__section">{props.heading}</li>
      {props.places}
      <li className="app-shell__divider" aria-hidden="true" />
    </>
  );
}

/** Manage's places: the admin's, or for an admin away from home where they administer instead. */
function managePlaces(
  administersFromHomeAt: string | undefined,
  places: JSX.Element[],
): JSX.Element[] {
  return administersFromHomeAt === undefined
    ? places
    : [<AdministerFromHome key="home" address={administersFromHomeAt} />];
}

/**
 * Where an admin away from home administers: at home, at the server's home address when it has one
 * ready (D297 §6, D292 §6).
 */
function AdministerFromHome(props: { readonly address: string }): JSX.Element {
  const { t } = useTranslation();
  return (
    <li className="app-shell__note">
      {props.address === "" ? (
        t("nav.administerFromHome")
      ) : (
        <>
          {t("nav.administerFromHomeAt")} <a href={props.address}>{props.address}</a>
        </>
      )}
    </li>
  );
}

/** The realm the person is signed in to, in view on every screen: which server this is (D297 §5). */
function RealmMark(props: { readonly realm: string }): JSX.Element {
  const { t } = useTranslation();
  return (
    <span className="app-shell__realm" title={t("nav.realmTitle", { realm: props.realm })}>
      {props.realm}
    </span>
  );
}

/** Three lines: the menu's mark, its name for a screen reader. */
export function MenuIcon(): JSX.Element {
  return (
    <Icon className="app-shell__menu-icon">
      <path d="M3 6h18v2H3zm0 5h18v2H3zm0 5h18v2H3z" />
    </Icon>
  );
}

/** One place in the menu, marked when it is the current screen. */
function Place(props: {
  readonly destination: Destination;
  readonly isCurrent: boolean;
}): JSX.Element {
  const { t } = useTranslation();
  const { destination } = props;
  return (
    <li aria-current={props.isCurrent ? "page" : undefined}>
      <Link to={destination.route}>{t(`nav.${destination.labelKey}`)}</Link>
    </li>
  );
}

/**
 * The one menu, behind its icon: who is signed in where, the places it is given, and Sign out.
 */
function MainMenu(props: {
  readonly signedInAs:
    { readonly name: string; readonly deviceName: string | undefined } | undefined;
  /** The realm signed in to, said here on a phone, where the banner has no room for it. */
  readonly realm: string | undefined;
  readonly isCurrent: boolean;
  readonly onSignOut: () => void;
  readonly onSwitchHousehold: (() => void) | undefined;
  readonly children: ReactNode;
}): JSX.Element {
  const { t } = useTranslation();
  const { signedInAs, realm } = props;
  return (
    <BannerMenu label={t("nav.menu")} isCurrent={props.isCurrent}>
      {signedInAs === undefined ? null : (
        <li className="app-shell__who">
          {signedInAs.deviceName === undefined
            ? t("nav.signedInAs", { name: signedInAs.name })
            : t("nav.signedInAsOn", { name: signedInAs.name, device: signedInAs.deviceName })}
          {realm === undefined ? null : (
            <span className="app-shell__who-realm">{t("nav.realmTitle", { realm })}</span>
          )}
        </li>
      )}
      {props.children}
      {props.onSwitchHousehold === undefined ? null : (
        <li>
          <button type="button" onClick={props.onSwitchHousehold}>
            {t("nav.switchHousehold")}
          </button>
        </li>
      )}
      <li>
        <button type="button" onClick={props.onSignOut}>
          {t("nav.signOut")}
        </button>
      </li>
    </BannerMenu>
  );
}

/**
 * Whether event closes the open menu: a press outside it, a click on one of its places, or focus
 * moving to something outside it. A press on its own name is the menu's to toggle.
 */
function closesMenu(event: Event, menu: HTMLDetailsElement): boolean {
  const target = event.target instanceof Element ? event.target : null;
  const isInside = target !== null && menu.contains(target);
  if (event instanceof FocusEvent) {
    return event.relatedTarget instanceof Element && !menu.contains(event.relatedTarget);
  }
  if (event.type === "pointerdown") {
    return !isInside;
  }
  return isInside && target.closest("a") !== null;
}

/**
 * A menu in the banner: its icon, which opens a list below it. It is the page's own disclosure,
 * so it works without script and by keyboard; it closes when one of its places is chosen, when
 * the person presses or taps outside it or moves focus out of it, and on Back, Escape's included,
 * which it hears before anything else does (input-layer.tsx), its icon keeping focus.
 */
/**
 * Brings a focused place of the menu wholly into the menu's view, where the menu scrolls within
 * itself: Firefox left a place partly below its end in view (the browser test, 2026-10-28), which a
 * remote's focus could then not show whole.
 */
function showFocusedPlace(event: Event, menu: HTMLDetailsElement): void {
  if (
    event.target instanceof HTMLElement &&
    menu.querySelector("ul")?.contains(event.target) === true
  ) {
    event.target.scrollIntoView({ block: "nearest", inline: "nearest" });
  }
}

function BannerMenu(props: {
  /** Its name, for a screen reader and a pointer's hover: the eye sees the menu icon. */
  readonly label: string;
  /** Whether the current screen is one of its places: its icon is then marked as where one is. */
  readonly isCurrent: boolean;
  readonly children: ReactNode;
}): JSX.Element {
  const ref = useRef<HTMLDetailsElement>(null);
  useEffect(() => {
    const close = (event: Event): void => {
      const menu = ref.current;
      if (menu?.open === true && closesMenu(event, menu)) {
        menu.open = false;
      }
    };
    const show = (event: Event): void => {
      if (ref.current !== null) {
        showFocusedPlace(event, ref.current);
      }
    };
    document.addEventListener("pointerdown", close);
    document.addEventListener("click", close);
    document.addEventListener("focusout", close);
    document.addEventListener("focusin", show);
    return () => {
      document.removeEventListener("pointerdown", close);
      document.removeEventListener("click", close);
      document.removeEventListener("focusout", close);
      document.removeEventListener("focusin", show);
    };
  }, []);
  useCommands(
    "document",
    (detail, event) => {
      const menu = ref.current;
      if (detail.command === "back" && menu?.open === true) {
        event.preventDefault();
        menu.open = false;
        menu.querySelector("summary")?.focus();
      }
    },
    { isCapture: true },
  );
  const classes = ["app-shell__menu"];
  if (props.isCurrent) {
    classes.push("app-shell__menu--current");
  }
  return (
    <details ref={ref} className={classes.join(" ")} data-focus-container="">
      <summary aria-label={props.label} title={props.label}>
        <MenuIcon />
      </summary>
      <ul>{props.children}</ul>
    </details>
  );
}
