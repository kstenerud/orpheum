// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import type { TogetherVoice, VoiceSnapshot } from "@orpheum/sdk";
import { act, createRef } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { commandEventName } from "./input-commands.ts";
import type { PlayerSound } from "./player-controls.tsx";
import { PlayerMenus } from "./player-menus.tsx";
import { fire, mount, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import { SessionVoiceContext, type VoiceInUse } from "./together-voice.tsx";
import { lettersOf, personColourCount, personColourOf } from "./volume-row.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

describe("a person's letters and colour in 🔈's row (D440 §2)", () => {
  it("gives each their first letter, and two where first letters collide", () => {
    expect(lettersOf(["dan", "Karl", "Kim", "eve"])).toEqual(["D", "Ka", "Ki", "E"]);
  });

  it("gives each member a colour of the palette, the same each time, apart for six in a row", () => {
    const colours = [1, 2, 3, 4, 5, 6].map(personColourOf);
    expect(new Set(colours).size).toBe(personColourCount);
    expect(personColourOf(7)).toBe(personColourOf(1));
    expect(colours.every((colour) => colour >= 1 && colour <= personColourCount)).toBe(true);
  });
});

/** A session's voice as a test holds it: two others, Dan and Kim, each gain set noted in set. */
function voiceNoting(set: [number, number][]): VoiceInUse {
  const snapshot = {
    status: "off",
    partners: [
      { memberId: 2, name: "Dan" },
      { memberId: 3, name: "Kim" },
      { memberId: 3, name: "Kim" }, // Kim's second device: one slider for her
    ],
    partnerGains: { 3: 2 },
  } as unknown as VoiceSnapshot;
  const voice = {
    setPartnerGain: (memberId: number, gain: number) => {
      set.push([memberId, gain]);
    },
  } as unknown as TogetherVoice;
  return { voice, snapshot, tap: { isWaiting: false, play: () => undefined } };
}

function soundNoting(set: (number | boolean)[]): PlayerSound {
  return {
    volume: 0.4,
    isMuted: false,
    canSetVolume: true,
    setVolume: (volume) => {
      set.push(volume);
    },
    setMuted: (isMuted) => {
      set.push(isMuted);
    },
  };
}

/** A command offered at target, as the input layer offers one; whether something took it. */
function offered(target: Element | null, command: string): boolean {
  const event = new CustomEvent(commandEventName, {
    detail: { command, source: "key" },
    bubbles: true,
    cancelable: true,
  });
  act(() => {
    target?.dispatchEvent(event);
  });
  return event.defaultPrevented;
}

/** 🔈 drawn and opened, in voice's session or alone. */
async function openedRow(
  sound: PlayerSound,
  voice: VoiceInUse | undefined,
  told: boolean[] = [],
): Promise<Mounted> {
  const on = await mount(
    withServices(
      <SessionVoiceContext value={voice}>
        <PlayerMenus
          videoRef={createRef()}
          shown={["sound"]}
          sound={sound}
          onOpenChange={(isOpen) => told.push(isOpen)}
        />
      </SessionVoiceContext>,
    ),
  );
  mounted = on;
  const opener = on.container.querySelector("button[aria-label='Volume']");
  await fire(opener, new MouseEvent("click", { bubbles: true }));
  return on;
}

function sliders(on: Mounted): HTMLElement[] {
  return [...on.container.querySelectorAll<HTMLElement>("dialog [data-volume-slider]")];
}

describe("🔈's row of sliders (D440 §2)", () => {
  it("draws the film's slider alone outside a session", async () => {
    const on = await openedRow(soundNoting([]), undefined);
    expect(sliders(on)).toHaveLength(1);
    expect(on.container.querySelectorAll("dialog input[type=range]")).toHaveLength(1);
  });

  it("draws the film's, then one per other person, with their letters and colour", async () => {
    const on = await openedRow(soundNoting([]), voiceNoting([]));
    const row = sliders(on);
    expect(row).toHaveLength(3);
    expect(row[0]?.classList.contains("volume-row__film")).toBe(true);
    expect(
      row.slice(1).map((slider) => slider.querySelector(".volume-row__letters")?.textContent),
    ).toEqual(["D", "K"]);
    expect(row.slice(1).map((slider) => slider.dataset["colour"])).toEqual([
      String(personColourOf(2)),
      String(personColourOf(3)),
    ]);
    const kims = row[2]?.querySelector<HTMLInputElement>("input[type=range]");
    expect(kims?.getAttribute("aria-label")).toBe("Kim's voice");
    expect(kims?.tabIndex).toBe(-1);
  });

  it("moves along the row at Left and Right, Up and Down stepping the focused slider, closing past its end", async () => {
    const filmSet: (number | boolean)[] = [];
    const gainSet: [number, number][] = [];
    const told: boolean[] = [];
    const on = await openedRow(soundNoting(filmSet), voiceNoting(gainSet), told);
    const mute = on.container.querySelector("dialog button[aria-label='Mute']");
    expect(offered(mute, "up")).toBe(true);
    expect(filmSet).toEqual([0.45]);

    expect(offered(mute, "right")).toBe(true);
    expect(sliders(on)[1]?.hasAttribute("data-focused")).toBe(true);
    expect(sliders(on)[1]?.querySelector(".volume-row__name")?.textContent).toBe("Dan");
    expect(offered(mute, "up")).toBe(true);
    expect(offered(mute, "right")).toBe(true);
    expect(sliders(on)[2]?.querySelector(".volume-row__name")?.textContent).toBe("Kim");
    expect(sliders(on)[1]?.querySelector(".volume-row__name")?.textContent).toBe("");
    expect(offered(mute, "down")).toBe(true);
    // Dan from 1 a step up, Kim from 2 a step down, each a step of the film's over the voice's span.
    expect(gainSet).toEqual([
      [2, 1.2],
      [3, 1.8],
    ]);
    expect(filmSet).toEqual([0.45]);

    expect(on.container.querySelector("dialog")).not.toBeNull();
    expect(offered(mute, "right")).toBe(true);
    expect(on.container.querySelector("dialog")).toBeNull();
    expect(told.at(-1)).toBe(false);
  });

  it("closes at Left from the film's slider, as a pop-up of one control does", async () => {
    const on = await openedRow(soundNoting([]), voiceNoting([]));
    const mute = on.container.querySelector("dialog button[aria-label='Mute']");
    expect(offered(mute, "left")).toBe(true);
    expect(on.container.querySelector("dialog")).toBeNull();
  });

  it("shows the phone's buttons' word in the film's slider's place where the page cannot set it", async () => {
    const sound = { ...soundNoting([]), canSetVolume: false };
    const on = await openedRow(sound, voiceNoting([]));
    expect(sliders(on)[0]?.querySelector(".film-volume__buttons")).not.toBeNull();
    expect(sliders(on)[1]?.querySelector("input[type=range]")).not.toBeNull();
  });
});

describe("Talk among the overlay's buttons (D440 §3)", () => {
  async function drawnTalk(voice: VoiceInUse | undefined): Promise<Mounted> {
    mounted = await mount(
      withServices(
        <SessionVoiceContext value={voice}>
          <PlayerMenus videoRef={createRef()} shown={[]} onOpenChange={() => undefined} />
        </SessionVoiceContext>,
      ),
    );
    return mounted;
  }

  function withMicrophone(isListed: boolean): () => void {
    const kind = isListed ? "audioinput" : "audiooutput";
    Object.defineProperty(navigator, "mediaDevices", {
      configurable: true,
      value: { enumerateDevices: () => Promise.resolve([{ kind }]) },
    });
    return () => {
      Reflect.deleteProperty(navigator, "mediaDevices");
    };
  }

  it("shows in a session on a device with a microphone", async () => {
    const restore = withMicrophone(true);
    try {
      const on = await drawnTalk(voiceNoting([]));
      await act(async () => {
        await Promise.resolve();
      });
      expect(on.container.querySelector(".together-voice__talk--over-video")).not.toBeNull();
    } finally {
      restore();
    }
  });

  it("does not show without a microphone, nor outside a session", async () => {
    const restore = withMicrophone(false);
    try {
      let on = await drawnTalk(voiceNoting([]));
      await act(async () => {
        await Promise.resolve();
      });
      expect(on.container.querySelector(".together-voice__talk")).toBeNull();
      on.unmount();
      on = await drawnTalk(undefined);
      expect(on.container.querySelector(".together-voice__talk")).toBeNull();
    } finally {
      restore();
    }
  });
});
