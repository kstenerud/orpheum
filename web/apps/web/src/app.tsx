// SPDX-License-Identifier: MPL-2.0

import type { Api } from "@orpheum/sdk";
import { useEffect, useMemo, useSyncExternalStore, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { About } from "./about.tsx";
import { Account } from "./account.tsx";
import { AppShell } from "./app-shell.tsx";
import { tabStorage } from "./browser-storage.ts";
import { Browse } from "./browse.tsx";
import { Files } from "./files.tsx";
import { Identify } from "./identify.tsx";
import { Jobs } from "./jobs.tsx";
import { Libraries } from "./libraries.tsx";
import { OfflineReminder } from "./offline-reminder.tsx";
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
import { useRoute, useServices, useToken } from "./services.tsx";
import { SignIn } from "./sign-in.tsx";
import { System } from "./system.tsx";
import { ThisDevice } from "./this-device.tsx";
import { Title } from "./title.tsx";
import { haveBackMySessions } from "./together-mine.ts";
import { readMembership, subscribeToMemberships } from "./together-storage.ts";
import { TogetherViewer } from "./together-viewer.tsx";
import { useResource } from "./use-resource.ts";
import { WatchTogether } from "./watch-together.tsx";

/**
 * The client: sign-in until there is a session, then the shell and the route's screen. A
 * registration invitation is its own page, signed in or not (D218 §2); so is a Watch Together
 * viewer's, who sees only the host's playback, never the app around it (D246 §3).
 */
export function App(): JSX.Element {
  const route = useRoute();
  const token = useToken();
  if (route.screen === "register") {
    return <Register />;
  }
  if (route.screen === "together") {
    return <TogetherViewer />;
  }
  return token === undefined ? <SignIn /> : <SignedIn />;
}

/**
 * The signed-in client. A session the server no longer knows is signed out by the watch every
 * request passes (signed-out-watch.ts), with the server's reason.
 */
function SignedIn(): JSX.Element {
  const { api, session } = useServices();
  const route = useRoute();
  const [me] = useResource("me", async () => dataOrThrow(await api.GET("/me")));
  const signOut = (): void => {
    signOutHere(api, session);
  };
  const isAdmin = me.state === "loaded" && me.value.isAdmin;
  const poller = useServersPoller();
  const isHostingTogether = useIsHostingTogether();
  useEffect(() => {
    // A session this person is in follows them to this device (D267 §2); one not had back is
    // offered again by the screens that start or join one.
    void haveBackMySessions(api).catch(() => undefined);
  }, [api]);
  return (
    <BackgroundChangesContext value={poller}>
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
        onSignOut={signOut}
      >
        {isAdmin ? <OfflineReminder /> : null}
        <Screen route={route} />
      </AppShell>
    </BackgroundChangesContext>
  );
}

/** Whether this tab hosts a Watch Together session now, re-rendering as one starts or ends. */
function useIsHostingTogether(): boolean {
  const isHosting = (): boolean => readMembership(tabStorage(), "host") !== undefined;
  return useSyncExternalStore(subscribeToMemberships, isHosting, isHosting);
}

/**
 * The poller of screens showing what the server changes in the background, on the waits the
 * server's client settings say (D237); D237's starting waits until they are read. A longest wait
 * shorter than the first is the first.
 */
function useServersPoller(): Poller {
  const { api } = useServices();
  const [clientSettings] = useResource("client-settings", async () =>
    dataOrThrow(await api.GET("/client-settings")),
  );
  const read = clientSettings.state === "loaded" ? clientSettings.value : undefined;
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
 * server refuses a session to a role that may not host) and About.
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
