// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { keepScreenAwake, type ScreenLock, type ScreenLocks } from "./wake.ts";

/** Listeners by event, which a test fires. */
class Listened {
  readonly #listeners = new Map<string, Set<(event: Event) => void>>();

  addEventListener(type: string, listener: (event: Event) => void): void {
    this.#listeners.set(type, (this.#listeners.get(type) ?? new Set()).add(listener));
  }

  removeEventListener(type: string, listener: (event: Event) => void): void {
    this.#listeners.get(type)?.delete(listener);
  }

  fire(type: string): void {
    for (const listener of this.#listeners.get(type) ?? []) {
      listener(new Event(type));
    }
  }
}

/** A stand-in video, paused until a test plays it. */
class FakeVideo extends Listened {
  /* eslint-disable @typescript-eslint/naming-convention -- HTMLMediaElement's own names, which this stands in for */
  paused = true;
  ended = false;
  /* eslint-enable @typescript-eslint/naming-convention -- the fake's own names follow */
}

/** A stand-in document, shown until a test hides it. */
class FakeDocument extends Listened {
  visibilityState: DocumentVisibilityState = "visible";
}

/** A stand-in wake lock, held until let go. */
class FakeLock implements ScreenLock {
  // eslint-disable-next-line @typescript-eslint/naming-convention -- WakeLockSentinel's own name, which this stands in for
  released = false;

  release(): Promise<void> {
    this.released = true;
    return Promise.resolve();
  }
}

/** Stand-in locks: each lock given, or a refusal while refusing. */
class FakeLocks implements ScreenLocks {
  readonly given: FakeLock[] = [];
  isRefusing = false;

  request(): Promise<ScreenLock> {
    if (this.isRefusing) {
      return Promise.reject(new Error("NotAllowedError"));
    }
    const lock = new FakeLock();
    this.given.push(lock);
    return Promise.resolve(lock);
  }

  released(): boolean[] {
    return this.given.map((lock) => lock.released);
  }
}

/** Lets the locks' promises settle. */
async function settled(): Promise<void> {
  await Promise.resolve();
  await Promise.resolve();
}

describe("keeping the screen awake while a film plays (D287)", () => {
  it("holds a lock while the video plays, and lets it go when it pauses or ends", async () => {
    const video = new FakeVideo();
    const locks = new FakeLocks();
    keepScreenAwake(video, new FakeDocument(), locks);
    await settled();
    expect(locks.released()).toEqual([]); // paused: the screen may lock
    video.paused = false;
    video.fire("playing");
    await settled();
    expect(locks.released()).toEqual([false]);
    video.paused = true;
    video.fire("pause");
    expect(locks.released()).toEqual([true]);
    video.paused = false;
    video.fire("play");
    await settled();
    video.ended = true;
    video.fire("ended");
    expect(locks.released()).toEqual([true, true]);
  });

  it("asks again when the page is shown after the browser let it go", async () => {
    const video = new FakeVideo();
    const page = new FakeDocument();
    const locks = new FakeLocks();
    video.paused = false;
    keepScreenAwake(video, page, locks);
    await settled();
    page.visibilityState = "hidden";
    const first = locks.given[0];
    if (first !== undefined) {
      first.released = true; // the browser lets it go as the page hides
    }
    page.fire("visibilitychange");
    page.visibilityState = "visible";
    page.fire("visibilitychange");
    await settled();
    expect(locks.released()).toEqual([true, false]);
  });

  it("plays on without a lock when refused, and asks again at the next change", async () => {
    const video = new FakeVideo();
    const locks = new FakeLocks();
    locks.isRefusing = true;
    video.paused = false;
    keepScreenAwake(video, new FakeDocument(), locks);
    await settled();
    expect(locks.released()).toEqual([]);
    locks.isRefusing = false;
    video.fire("playing");
    await settled();
    expect(locks.released()).toEqual([false]);
  });

  it("lets go of a lock given after it stopped, and of the one it holds when stopped", async () => {
    const video = new FakeVideo();
    const page = new FakeDocument();
    const locks = new FakeLocks();
    video.paused = false;
    const stop = keepScreenAwake(video, page, locks);
    stop(); // before the first lock is given
    await settled();
    expect(locks.released()).toEqual([true]);
    const again = keepScreenAwake(video, page, locks);
    await settled();
    again();
    expect(locks.released()).toEqual([true, true]);
  });

  it("does nothing where the browser gives no locks", () => {
    const video = new FakeVideo();
    video.paused = false;
    expect(() => {
      keepScreenAwake(video, new FakeDocument(), undefined)();
    }).not.toThrow();
  });
});
