// SPDX-License-Identifier: MPL-2.0

import "./app-shell.css";

import {
  createContext,
  useEffect,
  useRef,
  useState,
  type JSX,
  type ReactNode,
  type RefObject,
} from "react";
import { useTranslation } from "react-i18next";

import logo from "./assets/orpheum-logo.svg";
import { DropDown } from "./drop-down.tsx";
import { dangerousMark } from "./focus-navigation.ts";
import { ListNote, ListOption } from "./list-box.tsx";
import { Icon } from "./icon.tsx";
import { StatusTray } from "./status-tray.tsx";
import { Link } from "./link.tsx";
import { ModalDialog } from "./modal-dialog.tsx";
import { sortedLibraries, useLibrariesReached } from "./libraries-reached.ts";
import { FilmsIcon, LibrariesIcon, SeriesIcon } from "./place-icons.tsx";
import { LocalNetworkNote } from "./local-network-note.tsx";
import { NoticeNote } from "./notice-note.tsx";
import { PlainHttpNote } from "./plain-http-note.tsx";
import { SecurityUpdateNote } from "./security-update-note.tsx";
import { WorkerBypassedNote } from "./worker-bypassed-note.tsx";
import { LinkCapBadge } from "./link-debug-tools.tsx";
import { phoneWidthQuery } from "./phone-width.ts";
import { productName } from "./product-name.ts";
import type { Route } from "./router.ts";
import { useRoute } from "./services.tsx";

/** The logo, the way home, underlined while home is the page (the owner, 2026-10-25). */
/** The laurel alone, which the page's favicon is, served beside the client (index.html). */
const markOnly = "/favicon.svg";

/**
 * The places for watching: the way home, which stays put, and the other places beside it in the room
 * left, scrolling sideways when they need more (the owner, 2026-10-28: on a phone "the laurel logo
 * should go on the lower row ... since it belongs with the library links. That should be a single
 * component, made up of an immovable home link and a scrollable set of other links that fills the
 * row's free space").
 */
function WatchingPlaces(props: {
  readonly isHome: boolean;
  readonly children: ReactNode;
}): JSX.Element {
  return (
    <div className="app-shell__places">
      <HomeLogo isHome={props.isHome} />
      <ul className="app-shell__watching">{props.children}</ul>
    </div>
  );
}

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
          <source media={phoneWidthQuery} srcSet={markOnly} />
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
   * Who it shows to: everyone, an admin, or anyone while the admin has the pages for development
   * turned on. The device tests are This device's (this-device.tsx), not the menu's.
   */
  readonly audience: "everyone" | "admin" | "developer";
  /**
   * Where in the banner it is: the places for watching, always in view; or, in the one menu at the
   * banner's end, the admin's area under Manage, the pages for development, or the person's own
   * (the owner, 2026-10-11 and 2026-10-17).
   */
  readonly group: "watching" | "manage" | "develop" | "own";
  /** Its icon, which a place for watching shows in the banner in place of its name (2026-10-28). */
  readonly icon?: (props: { readonly className: string }) => JSX.Element;
  /** Whether it opens the menu of the person's libraries rather than going to its route (LibrariesMenu). */
  readonly isLibrariesMenu?: boolean;
}

const destinations: readonly Destination[] = [
  {
    route: { screen: "films" },
    labelKey: "films",
    audience: "everyone",
    group: "watching",
    icon: FilmsIcon,
  },
  {
    route: { screen: "series" },
    labelKey: "series",
    audience: "everyone",
    group: "watching",
    icon: SeriesIcon,
  },
  {
    // The Libraries page (D349 §9.1), which replaces Search in the top bar; its own labelKey is
    // the admin's "libraries" (Manage), reused: both say "Libraries", each at its own place.
    route: { screen: "browse" },
    labelKey: "libraries",
    audience: "everyone",
    group: "watching",
    icon: LibrariesIcon,
    isLibrariesMenu: true,
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
        isInMenu={group !== "watching"}
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
        <nav className="app-shell__nav" aria-label={t("nav.label")}>
          <WatchingPlaces isHome={props.currentScreen === "home"}>
            {links("watching")}
          </WatchingPlaces>
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
      <ShellMain isAdmin={props.isAdmin}>{props.children}</ShellMain>
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
  }
}

/**
 * Whether a screen renders in the shell, under its banner and in its one main landmark: a screen that
 * is also a page of its own (a Watch Together guest's, D372 §9) draws no second landmark or logo.
 */
export const InShellContext = createContext(false);

/**
 * The screen, under any word the page has about what the browser is doing, an admin's banner of a
 * security update pending, and the tab's notice.
 */
function ShellMain(props: {
  readonly isAdmin: boolean;
  readonly children: ReactNode;
}): JSX.Element {
  return (
    <main className="app-shell__main" data-focus-memory="">
      <SecurityUpdateNote isAdmin={props.isAdmin} />
      <PlainHttpNote />
      <WorkerBypassedNote />
      <LinkCapBadge />
      <LocalNetworkNote />
      <NoticeNote />
      <InShellContext value={true}>{props.children}</InShellContext>
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

/**
 * One place, in the banner or the menu, marked when it is the current screen: by its icon where it
 * has one, its name then its label for a screen reader and a pointer's hover; Libraries the menu of
 * the person's libraries (LibrariesMenu).
 */
function Place(props: {
  readonly destination: Destination;
  readonly isCurrent: boolean;
  /** Whether it is one of the menu's places, a choice of its list, not the banner's. */
  readonly isInMenu: boolean;
}): JSX.Element {
  if (props.isInMenu) {
    return <MenuPlace destination={props.destination} isCurrent={props.isCurrent} />;
  }
  return props.destination.isLibrariesMenu === true ? (
    <LibrariesMenu destination={props.destination} isCurrent={props.isCurrent} />
  ) : (
    <PlaceLink destination={props.destination} isCurrent={props.isCurrent} />
  );
}

/** One of the menu's places: a choice of its list (list-box.tsx), by its name. */
function MenuPlace(props: {
  readonly destination: Destination;
  readonly isCurrent: boolean;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <ListOption to={props.destination.route} isCurrent={props.isCurrent}>
      {t(`nav.${props.destination.labelKey}`)}
    </ListOption>
  );
}

/** A place as a link to its route, by its icon where it has one, else its name. */
function PlaceLink(props: {
  readonly destination: Destination;
  readonly isCurrent: boolean;
}): JSX.Element {
  const { t } = useTranslation();
  const { destination } = props;
  const name = t(`nav.${destination.labelKey}`);
  const PlaceIcon = destination.icon;
  return (
    <li aria-current={props.isCurrent ? "page" : undefined}>
      {PlaceIcon === undefined ? (
        <Link to={destination.route}>{name}</Link>
      ) : (
        <Link to={destination.route} className="app-shell__place-icon" label={name}>
          <PlaceIcon className="app-shell__place-glyph" />
        </Link>
      )}
    </li>
  );
}

/**
 * The banner's Libraries (the owner, 2026-10-28: "make the libraries icon a menu that lists the
 * libraries under it"): a drop-down of the person's libraries by name, each its page, then the
 * Libraries page, every library together, last (one name for it everywhere, ui-audit-2026-11-04.md
 * Q7), the one shown marked; the shared drop-down (drop-down.tsx), so on a TV it closes as
 * any chooser does, at Back, Left or Right, and scrolls within its height. With one library the icon
 * goes straight to its page, no menu of one; with none, or before they are read, to the Libraries
 * page, as it did.
 */
function LibrariesMenu(props: {
  readonly destination: Destination;
  readonly isCurrent: boolean;
}): JSX.Element {
  const { t } = useTranslation();
  const libraries = useLibrariesReached();
  const route = useRoute();
  const { destination, isCurrent } = props;
  const sorted = libraries.state === "loaded" ? sortedLibraries(libraries.value) : [];
  const [only] = sorted;
  if (sorted.length < 2) {
    return (
      <PlaceLink
        destination={
          only === undefined
            ? destination
            : { ...destination, route: { screen: "browse", library: only.id } }
        }
        isCurrent={isCurrent}
      />
    );
  }
  const name = t(`nav.${destination.labelKey}`);
  const shown = route.screen === "browse" ? route.library : undefined;
  return (
    <li aria-current={isCurrent ? "page" : undefined}>
      <DropDown
        label={name}
        opener={<LibrariesIcon className="app-shell__place-glyph" />}
        openerClassName="app-shell__place-icon"
        openerNaming={{ "aria-label": name, title: name }}
        isFromItsTop
      >
        {sorted.map((library) => (
          <ListOption
            key={library.id}
            to={{ screen: "browse", library: library.id }}
            isCurrent={shown === library.id}
          >
            {library.name}
          </ListOption>
        ))}
        <ListOption to={{ screen: "browse", library: "all" }} isCurrent={shown === "all"}>
          {t("browse.libraries")}
        </ListOption>
      </DropDown>
    </li>
  );
}

/**
 * The one menu, behind its icon: who is signed in where, the places it is given, and Sign out. It is
 * the chooser's drop-down (drop-down.tsx) with no chevron, its icon marked as where one is while the
 * current screen is one of its places (the owner, 2026-10-28: "the menu should share code with it,
 * the only real difference being that there's no extra visual indicator that it's a dropdown").
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
  const label = t("nav.menu");
  const [isSignOutAsked, setIsSignOutAsked] = useState(false);
  return (
    <div
      className={props.isCurrent ? "app-shell__menu app-shell__menu--current" : "app-shell__menu"}
    >
      <DropDown
        label={label}
        opener={<MenuIcon />}
        openerClassName="app-shell__menu-button"
        openerNaming={{ "aria-label": label, title: label }}
        isFromItsTop
      >
        <MenuPlaces
          signedInAs={props.signedInAs}
          realm={props.realm}
          onSignOut={() => {
            setIsSignOutAsked(true);
          }}
          onSwitchHousehold={props.onSwitchHousehold}
        >
          {props.children}
        </MenuPlaces>
      </DropDown>
      {isSignOutAsked ? (
        <SignOutAsked
          onSignOut={props.onSignOut}
          onCancel={() => {
            setIsSignOutAsked(false);
          }}
        />
      ) : null}
    </div>
  );
}

/**
 * The question the menu's Sign out asks first (the owner, 2026-10-30: "Selecting 'sign out' should
 * open a confirmation"): the shared modal dialog, in the menu's place, with Cancel, first and focused,
 * and Sign out, marked a dangerous action (D421), a remote's arrows reaching both; only Sign out
 * signs out, and Cancel, Escape, Back or a press outside it leaves the person signed in.
 */
function SignOutAsked(props: {
  readonly onSignOut: () => void;
  readonly onCancel: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const question = t("nav.signOutAsked");
  return (
    <ModalDialog label={question} onClose={props.onCancel}>
      <p>{question}</p>
      <div className="app-shell__sign-out-buttons">
        <button type="button" onClick={props.onCancel}>
          {t("nav.signOutCancel")}
        </button>
        <button type="button" {...dangerousMark} onClick={props.onSignOut}>
          {t("nav.signOut")}
        </button>
      </div>
    </ModalDialog>
  );
}

/** What the menu holds, in order: who is signed in where, its places, and Sign out last. */
function MenuPlaces(props: Omit<Parameters<typeof MainMenu>[0], "isCurrent">): JSX.Element {
  const { t } = useTranslation();
  const { signedInAs, realm } = props;
  return (
    <>
      {signedInAs === undefined ? null : (
        <ListNote>
          <div className="app-shell__who">
            {signedInAs.deviceName === undefined
              ? t("nav.signedInAs", { name: signedInAs.name })
              : t("nav.signedInAsOn", { name: signedInAs.name, device: signedInAs.deviceName })}
            {realm === undefined ? null : (
              <span className="app-shell__who-realm">{t("nav.realmTitle", { realm })}</span>
            )}
          </div>
        </ListNote>
      )}
      {props.children}
      {props.onSwitchHousehold === undefined ? null : (
        <ListOption onChoose={props.onSwitchHousehold}>{t("nav.switchHousehold")}</ListOption>
      )}
      <ListOption onChoose={props.onSignOut}>{t("nav.signOut")}</ListOption>
    </>
  );
}
