// SPDX-License-Identifier: MPL-2.0

/**
 * The extend probe's two windows (plans/tv-device-tests.md): the opener on the laptop's screen is
 * the remote, the player on the TV's plays a test pattern, and they speak over a BroadcastChannel,
 * as a remote page and a TV would over the session (watch-together-split-devices §4.3).
 */

/** The channel both windows open. */
export const extendChannelName = "orpheum-lab-extend";

/** What the remote tells the player to do. */
export type ExtendCommand = "toggle" | "back" | "forward";

/** How far back and forward the remote's buttons move the player, in seconds. */
export const extendSeekS = 10;

/** A message between the windows: a command to the player, or the player's state to the remote. */
export type ExtendMessage =
  | { readonly kind: "command"; readonly command: ExtendCommand }
  | {
      readonly kind: "state";
      readonly positionS: number;
      readonly isPlaying: boolean;
      readonly isFullscreen: boolean;
    };

/** A state message read off the channel, or undefined where data is not one. */
function stateOf(data: object): ExtendMessage | undefined {
  const positionS: unknown = Reflect.get(data, "positionS");
  const isPlaying: unknown = Reflect.get(data, "isPlaying");
  const isFullscreen: unknown = Reflect.get(data, "isFullscreen");
  return typeof positionS === "number" &&
    typeof isPlaying === "boolean" &&
    typeof isFullscreen === "boolean"
    ? { kind: "state", positionS, isPlaying, isFullscreen }
    : undefined;
}

/** A message read off the channel, or undefined for anything else. */
export function extendMessageOf(data: unknown): ExtendMessage | undefined {
  if (typeof data !== "object" || data === null) {
    return undefined;
  }
  const kind: unknown = Reflect.get(data, "kind");
  if (kind === "state") {
    return stateOf(data);
  }
  const command: unknown = Reflect.get(data, "command");
  return kind === "command" && (command === "toggle" || command === "back" || command === "forward")
    ? { kind, command }
    : undefined;
}

/** A screen as the Window Management API describes it, as much as the probe uses. */
export interface ScreenPlace {
  readonly label: string;
  readonly left: number;
  readonly top: number;
  readonly width: number;
  readonly height: number;
  readonly isPrimary: boolean;
  readonly isInternal: boolean;
}

/** A number field of holder, or 0. */
function numberOf(holder: object, name: string): number {
  const value: unknown = Reflect.get(holder, name);
  return typeof value === "number" && Number.isFinite(value) ? value : 0;
}

/** The screens getScreenDetails answered with, by their available area. */
export function screensOf(details: unknown): ScreenPlace[] {
  const screens: unknown =
    typeof details === "object" && details !== null ? Reflect.get(details, "screens") : undefined;
  if (!Array.isArray(screens)) {
    return [];
  }
  return screens.flatMap((screen: unknown, index) => {
    if (typeof screen !== "object" || screen === null) {
      return [];
    }
    const label: unknown = Reflect.get(screen, "label");
    return [
      {
        label: typeof label === "string" && label !== "" ? label : `#${String(index + 1)}`,
        left: numberOf(screen, "availLeft"),
        top: numberOf(screen, "availTop"),
        width: numberOf(screen, "availWidth"),
        height: numberOf(screen, "availHeight"),
        isPrimary: Reflect.get(screen, "isPrimary") === true,
        isInternal: Reflect.get(screen, "isInternal") === true,
      },
    ];
  });
}

/** window.open's features placing a window over the whole of screen's available area. */
export function windowFeaturesOn(screen: ScreenPlace): string {
  return [
    `left=${String(screen.left)}`,
    `top=${String(screen.top)}`,
    `width=${String(screen.width)}`,
    `height=${String(screen.height)}`,
  ].join(",");
}

/** The player's position after command, from positionS, never before the start. */
export function positionAfter(command: ExtendCommand, positionS: number): number {
  if (command === "back") {
    return Math.max(0, positionS - extendSeekS);
  }
  return command === "forward" ? positionS + extendSeekS : positionS;
}
