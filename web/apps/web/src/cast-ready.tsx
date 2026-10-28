// SPDX-License-Identifier: MPL-2.0

import "./cast.css";

import type { CastToReceive } from "@orpheum/sdk";
import { useCallback, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { leaveFullScreen } from "./cast-asked.tsx";
import { ControlledMode, useEndsAsTheCastDoes } from "./cast-receiver.tsx";
import { endCast } from "./cast.ts";
import { Link } from "./link.tsx";
import { useLive } from "./live.tsx";
import { useServices, useToken } from "./services.tsx";
import { FollowsTheGroupHere } from "./together-viewer.tsx";
import { useTogether } from "./use-together.ts";

/**
 * A cast's receiver between videos (D364 §5): full screen, "Ready, controlled from ‹Device›", so the
 * next video the person plays starts full screen and with sound, with no press here. Taking control
 * back (Back, Escape, OK twice) ends the session and goes to this device's home page (§9); Stop
 * casting on the sender returns it to the page it was on before the session (§10). A guest's cast,
 * handed their part in the Watch Together session they are in, follows that session's film here, just
 * another follower, as the guest's own device did (D371 §4). Without a cast waiting ready, as a page
 * loaded again before it is handed its cast again, it says so.
 */
export function CastReady(): JSX.Element {
  const { t } = useTranslation();
  const { receiving } = useLive();
  if (receiving?.fileId !== 0) {
    return (
      <section className="cast-ready-none">
        <p>{t("cast.notReceiving")}</p>
        <Link to={{ screen: "home" }}>{t("cast.toHome")}</Link>
      </section>
    );
  }
  return <ReadyFor key={receiving.castId} cast={receiving} />;
}

/** The ready page of one cast, which hears how the cast ends from its session. */
function ReadyFor(props: { readonly cast: CastToReceive }): JSX.Element {
  const { t } = useTranslation();
  const { api, navigation } = useServices();
  const token = useToken();
  const { endReceiving, leaveReceiving } = useLive();
  const { cast } = props;
  const { snapshot } = useTogether({
    credential: cast.membershipCredential,
    sessionToken: token,
    role: "panel", // it plays nothing; it hears the cast's end
  });
  const goHome = useCallback(() => {
    endReceiving();
    leaveFullScreen(document);
    navigation.go({ screen: "home" });
  }, [endReceiving, navigation]);
  useEndsAsTheCastDoes(snapshot?.status === "ended" ? snapshot.endReason : undefined, {
    onOver: goHome,
    onLeave: leaveReceiving,
  });
  const takeBack = useCallback(() => {
    endCast(api, cast.castId, "takenBack").catch(() => undefined);
    goHome();
  }, [api, cast.castId, goHome]);
  const { togetherCredential } = cast;
  return (
    <div className="cast-ready">
      {togetherCredential !== undefined && cast.isTogetherFollower === true ? (
        <FollowsTheGroupHere credential={togetherCredential} />
      ) : (
        <p className="cast-ready__line">{t("cast.ready", { name: cast.fromDeviceName })}</p>
      )}
      <ControlledMode fromName={cast.fromDeviceName} onTakeBack={takeBack} />
    </div>
  );
}
