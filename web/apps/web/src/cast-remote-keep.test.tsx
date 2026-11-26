// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import type { CastSending, PictureChoice, SoundChoice } from "@orpheum/sdk";
import { useEffect, type JSX } from "react";
import { afterEach, describe, expect, it } from "vitest";

import type { ReceiverChoices } from "./cast-remote-choices.tsx";
import { useKeptByTheSender } from "./cast-remote-keep.ts";
import { IsGuestContext } from "./guest-page.ts";
import { mount, servedBy, settleUntil, type Heard, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

/** A cast of file 7 to a receiver, a guest receiver where isGuestReceiver. */
function sendingTo(isGuestReceiver: boolean): CastSending {
  return {
    castId: 1,
    receiverDeviceId: 2,
    receiverName: "TV",
    fileId: 7,
    itemName: "",
    durationMs: 600_000,
    isAccepted: true,
    isGuestReceiver,
  };
}

/** A receiver's connection noting each choice sent to it. */
function connectionNoting(sent: string[]): ReceiverChoices {
  return {
    chooseSound: (choice: SoundChoice) => {
      sent.push(`sound ${choice.kind}`);
    },
    subscribeToSoundChoices: () => () => undefined,
    choosePicture: (choice: PictureChoice) => {
      sent.push(`picture ${choice.kind}`);
    },
    subscribeToPictureChoices: () => () => undefined,
  };
}

/** Sends each of the remote's choices through the kept connection once it is drawn. */
function Sends(props: {
  readonly connection: ReceiverChoices;
  readonly sending: CastSending;
}): JSX.Element | null {
  const kept = useKeptByTheSender(props.connection, props.sending).connection;
  useEffect(() => {
    kept?.chooseSound({ kind: "boost", isOn: true });
    kept?.chooseSound({ kind: "levelling", isOn: false });
    kept?.chooseSound({ kind: "audioTrack", streamId: 3 });
    kept?.chooseSound({ kind: "soundOffset", ms: 100 });
    kept?.choosePicture({ kind: "subtitles", key: "s4" });
    kept?.choosePicture({ kind: "subtitleOffset", ms: -1500 });
  }, [kept]);
  return null;
}

const answers = {
  "PUT /me/preferences": [204, {}],
  "PUT /files/7/preferences": [204, {}],
} as const;

async function sent(
  sending: CastSending,
  isGuest = false,
): Promise<{ sent: string[]; heard: Heard[] }> {
  const choices: string[] = [];
  const heard: Heard[] = [];
  mounted = await mount(
    withServices(
      <IsGuestContext value={isGuest}>
        <Sends connection={connectionNoting(choices)} sending={sending} />
      </IsGuestContext>,
      servedBy(answers, heard),
    ),
  );
  return { sent: choices, heard };
}

describe("a cast's remote keeping the person's choices (D465 §2)", () => {
  it("keeps each choice it sends a guest receiver, as the Play page keeps it", async () => {
    const { sent: choices, heard } = await sent(sendingTo(true));

    expect(choices).toEqual([
      "sound boost",
      "sound levelling",
      "sound audioTrack",
      "sound soundOffset",
      "picture subtitles",
      "picture subtitleOffset",
    ]);
    await settleUntil(() => heard.length === 5);
    // Each keeps on its own, so they may be heard in any order.
    const kept = heard.map((each) => [each.methodAndPath, each.body]);
    expect(kept).toHaveLength(5);
    expect(kept).toEqual(
      expect.arrayContaining([
        ["PUT /me/preferences", { isDialogueBoostOn: true }],
        ["PUT /me/preferences", { isLevellingOn: false }],
        ["PUT /files/7/preferences", { audioChoice: { streamId: 3 } }],
        ["PUT /files/7/preferences", { subtitleChoice: { streamId: 4 } }],
        ["PUT /files/7/preferences", { subtitleOffsetMs: -1500 }],
      ]),
    );
  });

  it("keeps none of a receiver signed in as the person, which keeps its own", async () => {
    const { sent: choices, heard } = await sent(sendingTo(false));

    expect(choices).toHaveLength(6);
    await new Promise((resolve) => {
      setTimeout(resolve, 0);
    });
    expect(heard).toEqual([]);
  });

  it("keeps none for a guest, refused the person's routes (D469 §4)", async () => {
    const { heard } = await sent(sendingTo(true), true);

    await new Promise((resolve) => {
      setTimeout(resolve, 0);
    });
    expect(heard).toEqual([]);
  });
});
