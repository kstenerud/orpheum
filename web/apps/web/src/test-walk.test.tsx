// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { describe, expect, it } from "vitest";

import { App } from "./app.tsx";
import { mount, settleUntil } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import { anAdmin, aLibrary, reachedFrom, walkAnswer, walkServices } from "./test-walk.tsx";

/** The paths the shell's one read carries, which a walk's server is not asked apart (D476). */
const carriedApart = ["/client-settings", "/libraries", "/needs-attention", "/system/update"];

describe("a walk with a remote's keys", () => {
  it("presses nothing once its page is gone, as when its test timed out (2026-10-25)", async () => {
    const pressed: string[] = [];
    const press = async (keys: string): Promise<void> => {
      pressed.push(keys);
      await Promise.resolve();
    };
    const gone = document.createElement("button");
    expect(await reachedFrom(gone, press)).toEqual(new Set([gone]));
    expect(pressed).toEqual([]);
  });

  it("presses each arrow from where it starts while its page is there", async () => {
    const pressed: string[] = [];
    const press = async (keys: string): Promise<void> => {
      pressed.push(keys);
      await Promise.resolve();
    };
    const here = document.createElement("button");
    document.body.append(here);
    await reachedFrom(here, press);
    here.remove();
    expect(pressed).toEqual(["{ArrowUp}", "{ArrowDown}", "{ArrowLeft}", "{ArrowRight}"]);
  });
});

describe("a walk's stand-in server (D476)", () => {
  const origin = "http://localhost";
  const ask = (answers: Record<string, unknown>, path: string): unknown =>
    walkAnswer(answers, new Request(new URL(`/api/v1${path}`, origin)));

  it("answers GET /me?shell=true with the shell's parts, made from the walk's own reads", () => {
    const answer = ask(aLibrary, "/me?shell=true") as { body: Record<string, unknown> };
    expect(answer.body["name"]).toBe("Karl");
    const shell = answer.body["shell"] as Record<string, unknown>;
    expect(shell["libraries"]).toEqual(
      (aLibrary["GET /libraries"] as { libraries: unknown }).libraries,
    );
    expect(shell["clientSettings"]).toHaveProperty("screenRefreshFirstWaitSeconds");
    // Nothing of an admin's for a person who is not one.
    expect(shell).not.toHaveProperty("needsAttention");
    expect(shell).not.toHaveProperty("update");
  });

  it("gives an admin's shell what needs an admin and the update state", () => {
    const answer = ask(anAdmin, "/me?shell=true") as { body: { shell: Record<string, unknown> } };
    expect(answer.body.shell).toHaveProperty("needsAttention");
    expect(answer.body.shell).toHaveProperty("update");
  });

  it("answers the minute's check, a plain GET /me, with the user alone", () => {
    const answer = ask(aLibrary, "/me") as { body: Record<string, unknown> };
    expect(answer.body).not.toHaveProperty("shell");
  });

  it("starts the app on one read of /me and none of the shell's parts apart", async () => {
    const heard: string[] = [];
    const mounted = await mount(withServices(<App />, walkServices(aLibrary, "/", heard)));
    await settleUntil(() => heard.includes("GET /home"));
    mounted.unmount();
    expect(heard.filter((each) => each === "GET /me")).toHaveLength(1);
    expect(heard.filter((each) => carriedApart.includes(each.replace(/^GET /u, "")))).toEqual([]);
  });
});
