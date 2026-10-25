// SPDX-License-Identifier: MPL-2.0

import type { Api, components } from "@orpheum/sdk";
import { useEffect, useMemo, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { ReloadWhenOutdated } from "./reload-when-outdated.tsx";
import { About } from "./about.tsx";
import { Account } from "./account.tsx";
import { AppShell } from "./app-shell.tsx";
import { tabStorage } from "./browser-storage.ts";
import { Browse } from "./browse.tsx";
import { Home } from "./home.tsx";
import { NeedsAttention } from "./needs-attention.tsx";
import { IsAdminContext } from "./warning-sign.tsx";
import { InputLayer } from "./input-layer.tsx";
import { Files } from "./files.tsx";
import { Identify } from "./identify.tsx";
import { Jobs } from "./jobs.tsx";
import { Lab } from "./lab.tsx";
import { LabExtendPlayer } from "./lab-extend-player.tsx";
import { DeviceTestsOfferedContext } from "./lab-offered.ts";
import { Libraries } from "./libraries.tsx";
import { People } from "./people.tsx";
import {
  BackgroundChangesContext,
  backgroundChangesPoller,
  backingOffPoller,
  clientWaits,
  type Poller,
} from "./polling.ts";
import { ApiProblem, dataOrThrow, throwIfRefused } from "./problem.ts";
import { PersonPage } from "./person.tsx";
import { Play } from "./play.tsx";
import { Register } from "./register.tsx";
import type { Route } from "./router.ts";
import { Search } from "./search.tsx";
import { Series } from "./series.tsx";
import type { Session } from "./session.ts";
import { Settings } from "./settings.tsx";
import { InterestSaidEveryContext } from "./page-interest.ts";
import { usePathMoveCount, useRoute, useServices, useToken } from "./services.tsx";
import { SignIn } from "./sign-in.tsx";
import { System } from "./system.tsx";
import { ThisDevice } from "./this-device.tsx";
import { Title } from "./title.tsx";
import { haveBackMySessions } from "./together-mine.ts";
import { useMembership } from "./together-membership.ts";
import { HostVoiceProvider } from "./together-voice.tsx";
import { TogetherViewer } from "./together-viewer.tsx";
import { Remembered, useRemembered } from "./remembered.ts";
import { useResource, type Resource } from "./use-resource.ts";
import { UiGallery } from "./ui-gallery.tsx";
import { WatchTogether } from "./watch-together.tsx";

/**
 * The client: sign-in until there is a session, then the shell and the route's screen, every one
 * driven by a remote, a keyboard or a controller through the input layer (input-layer.tsx). A
 * registration invitation is its own page, signed in or not (D218 §2); so is a Watch Together
 * viewer's, who sees only the host's playback, never the app around it (D246 §3).
 */
export function App(): JSX.Element {
  const route = useRoute();
  return (
    <InputLayer>
      <RoutedApp route={route} />
      <ReloadWhenOutdated route={route} />
    </InputLayer>
  );
}

/** The screen route names, signed in or not. */
function RoutedApp(props: { readonly route: ReturnType<typeof useRoute> }): JSX.Element {
  const token = useToken();
  if (props.route.screen === "register") {
    return <Register />;
  }
  if (props.route.screen === "together") {
    return <TogetherViewer />;
  }
  if (props.route.screen === "lab-extend-player") {
    // The lab's test pattern on the TV's screen: no shell, and no server, so no sign-in.
    return <LabExtendPlayer />;
  }
  return token === undefined ? <SignIn /> : <SignedIn />;
}

/**
 * The signed-in client. A session the server no longer knows is signed out by the watch every
 * request passes (signed-out-watch.ts), with the server's reason.
 */
function SignedIn(): JSX.Element {
  const { api, session, realm, switchHousehold } = useServices();
  const route = useRoute();
  // Read again on each path the page moves to: an admin administers only from home (D297 §6), and a
  // page that started on the core's public name and moved to its LAN name showed no admin until
  // reloaded (the owner's Chrome, 2026-10-15).
  const [meNow] = useResource(`me:${String(usePathMoveCount())}`, async () =>
    dataOrThrow(await api.GET("/me")),
  );
  // While it is read again, what it said last stays: the warning sign and the admin's places went
  // and came back at each move (the owner, 2026-10-17: "disappears for a split-second every now and
  // then").
  // Kept by this signed-in page alone, so another sign-in never starts from it.
  const [rememberedMe] = useState(() => new Remembered<components["schemas"]["UserBody"]>());
  const me = useRemembered("me", meNow, rememberedMe);
  const signOut = (): void => {
    signOutHere(api, session);
  };
  const isAdmin = me.state === "loaded" && me.value.isAdmin;
  const clientSettings = useClientSettings();
  const poller = useServersPoller(clientSettings);
  // The session this tab hosts, whose voice every screen shares (D281 §5).
  const hostMembership = useMembership(tabStorage, "host");
  const isHostingTogether = hostMembership !== undefined;
  useEffect(() => {
    // A session this person is in follows them to this device (D267 §2); one not had back is
    // offered again by the screens that start or join one.
    void haveBackMySessions(api).catch(() => undefined);
  }, [api]);
  return (
    <IsAdminContext value={isAdmin}>
      <HostVoiceProvider membership={hostMembership}>
        <BackgroundChangesContext value={poller}>
          <InterestSaidEveryContext value={clientSettings?.interestSaidEverySeconds ?? 15}>
            <AppShell
              isAdmin={isAdmin}
              canHostWatchTogether={me.state === "loaded" && me.value.canHostWatchTogether}
              isHostingTogether={isHostingTogether}
              signedInAs={
                me.state === "loaded"
                  ? { name: me.value.name, deviceName: me.value.device?.name }
                  : undefined
              }
              currentScreen={route.screen}
              administersFromHomeAt={administersFromHomeAt(me)}
              realm={realm}
              showsDeveloperPages={clientSettings?.shouldShowDeveloperPages === true}
              showsDeviceTests={clientSettings?.shouldOfferDeviceTests === true}
              onSwitchHousehold={switchHousehold}
              onSignOut={signOut}
            >
              <DeviceTestsOfferedContext value={clientSettings?.shouldOfferDeviceTests}>
                <Screen route={route} />
              </DeviceTestsOfferedContext>
            </AppShell>
          </InterestSaidEveryContext>
        </BackgroundChangesContext>
      </HostVoiceProvider>
    </IsAdminContext>
  );
}

/** The server settings every client reads, once read. */
function useClientSettings(): components["schemas"]["ClientSettingsBody"] | undefined {
  const { api } = useServices();
  const [clientSettings] = useResource("client-settings", async () =>
    dataOrThrow(await api.GET("/client-settings")),
  );
  return clientSettings.state === "loaded" ? clientSettings.value : undefined;
}

/**
 * The poller of screens showing what the server changes in the background, on the waits the
 * server's client settings say (D237); D237's starting waits until they are read. A longest wait
 * shorter than the first is the first.
 */
function useServersPoller(read: components["schemas"]["ClientSettingsBody"] | undefined): Poller {
  const firstSeconds = read?.screenRefreshFirstWaitSeconds;
  const longestSeconds = read?.screenRefreshLongestWaitSeconds;
  const failedSeconds = read?.screenRefreshFailedWaitSeconds;
  return useMemo(
    () =>
      firstSeconds === undefined || longestSeconds === undefined || failedSeconds === undefined
        ? backgroundChangesPoller
        : backingOffPoller(clientWaits({ firstSeconds, longestSeconds, failedSeconds })),
    [firstSeconds, longestSeconds, failedSeconds],
  );
}

/** The screen a route names. */
export function Screen(props: { readonly route: Route }): JSX.Element {
  const { t } = useTranslation();
  return (
    viewerScreen(props.route) ??
    nonLibraryScreen(props.route) ??
    adminScreen(props.route) ?? <p>{t("shell.notFound")}</p>
  );
}

/** The screens anyone signed in may see to watch: lists, a title, a series, playing and search. */
function viewerScreen(route: Route): JSX.Element | undefined {
  if (route.screen === "home") {
    return <Home />;
  }
  if (route.screen === "films" || route.screen === "series") {
    return <Browse key={route.screen} kind={route.screen === "films" ? "movie" : "series"} />;
  }
  if (route.screen === "title") {
    return <Title key={route.itemId} id={route.itemId} />;
  }
  if (route.screen === "series-page") {
    return (
      <Series
        key={`${String(route.collectionId)}/${String(route.season)}`}
        id={route.collectionId}
        season={route.season}
      />
    );
  }
  if (route.screen === "play") {
    return <Play key={route.fileId} fileId={route.fileId} />;
  }
  if (route.screen === "search") {
    return <Search key={route.query} query={route.query} />;
  }
  return undefined;
}

/**
 * The screens anyone signed in may see beyond the library: their account, Watch Together (the
 * server refuses a session to a role that may not host), the device lab and About.
 */
function nonLibraryScreen(route: Route): JSX.Element | undefined {
  if (route.screen === "account") {
    return <Account />;
  }
  if (route.screen === "this-device") {
    return <ThisDevice />;
  }
  if (route.screen === "watch-together") {
    return <WatchTogether />;
  }
  if (route.screen === "ui-gallery") {
    return <UiGallery />;
  }
  if (route.screen === "lab") {
    return <Lab key={route.probe ?? "menu"} probe={route.probe} />;
  }
  return route.screen === "about" ? <About /> : undefined;
}

/** The admin's screens; the server refuses them to anyone else. */
function adminScreen(route: Route): JSX.Element | undefined {
  if (route.screen === "identify") {
    return <Identify />;
  }
  if (route.screen === "libraries") {
    return <Libraries />;
  }
  if (route.screen === "settings") {
    return <Settings />;
  }
  if (route.screen === "jobs") {
    return <Jobs />;
  }
  if (route.screen === "system") {
    return <System />;
  }
  if (route.screen === "needs-attention") {
    return <NeedsAttention />;
  }
  if (route.screen === "people") {
    return <People />;
  }
  if (route.screen === "person") {
    return <PersonPage key={route.userId} userId={route.userId} />;
  }
  return route.screen === "files" ? (
    <Files key={route.libraryId} libraryId={route.libraryId} />
  ) : undefined;
}

/**
 * Signs this device out: at once here, whatever the server says. When the server could not be
 * told (it refused, or could not be reached), the sign-in page says so, since the session may
 * still work there until it expires; one the server had already ended needs no word.
 */
function signOutHere(api: Api, session: Session): void {
  api
    .DELETE("/session")
    .then(throwIfRefused)
    .then(
      () => {
        session.signOut();
      },
      (cause: unknown) => {
        if (cause instanceof ApiProblem && cause.code === "auth_required") {
          session.signOut();
          return;
        }
        session.signOutBecause("sign_out_unheard");
      },
    );
}

/**
 * For an admin who does not administer here (away from home, D297 §6): the server's home address,
 * or "" when it has none ready; undefined for anyone else, or before /me is read.
 */
function administersFromHomeAt(
  me: Resource<components["schemas"]["UserBody"]>,
): string | undefined {
  if (me.state !== "loaded" || me.value.role !== "admin" || me.value.isAdmin) {
    return undefined;
  }
  return me.value.homeAddresses?.[0] ?? "";
}
