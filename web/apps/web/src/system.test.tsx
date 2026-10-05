// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, expect, it } from "vitest";

import { System, type SystemNode, type SystemReport } from "./system.tsx";
import { mount, servedBy, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

/** The backups section's answer when there are none (D242 §5). */
const noBackups = [200, { directory: "/srv/orpheum/backups", backups: [] }] as const;

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

const connected: SystemNode = {
  name: "local",
  connection: "connected",
  isConnected: true,
  build: {
    name: "v0.3.0",
    version: "v0.3.0",
    commit: "0f6c5cc1a2b3c4d5",
    hasUncommittedChanges: false,
  },
  isServersBuild: true,
  host: {
    distribution: "Debian GNU/Linux",
    description: "Debian GNU/Linux 13 (trixie)",
    release: "13",
    codename: "trixie",
    kernel: "6.12.107",
    platform: "linux/amd64",
  },
  ffmpegVersion: "ffmpeg version 7.1",
  ffmpegSha256: "0123456789abcdef",
  uid: 1000,
  gid: 1000,
  lastHeardAt: 1_790_000_000_000,
  confinement: [{ mechanism: "seccomp", isInForce: false, causeCode: "kernel_without" }],
  hardware: {
    gpus: [{ devicePath: "/dev/dri/renderD128", driver: "i915" }],
    trials: [
      { encoder: "h264_qsv" },
      { encoder: "h264_nvenc", failure: "Cannot load libcuda.so.1" },
    ],
    chosenEncoder: "h264_qsv",
  },
  disks: [{ path: "/media", totalBytes: 4_000_000_000_000, availableBytes: 1_500_000_000_000 }],
  recentPrepares: [
    {
      sourceFileId: 42,
      path: "Films/Metropolis (1995)/Metropolis.mkv",
      profile: "h264-8m",
      state: "complete",
      videoEncoder: "h264_qsv",
      realtimeFactor: 6.5,
      finishedAt: 1_790_000_000_000,
    },
  ],
  devices: [],
  stuck: [],
};

function report(node: SystemNode): SystemReport {
  return {
    server: {
      build: {
        name: "v0.3.0",
        version: "v0.3.0",
        commit: "0f6c5cc1a2b3c4d5",
        hasUncommittedChanges: false,
      },
      host: { platform: "linux/amd64", kernel: "6.12.107" },
      goVersion: "go1.26.1",
      startedAt: 1_790_000_000_000,
      disks: [
        { path: "/var/lib/orpheum", totalBytes: 500_000_000_000, availableBytes: 250_000_000_000 },
      ],
      databases: [
        { name: "persistent.db", sizeBytes: 40_000_000 },
        { name: "cache.db", sizeBytes: 5_000_000 },
      ],
    },
    findings: [
      { check: "hardware encoding on node local", verdict: "ok", detail: "encodes use h264_qsv" },
    ],
    nodes: [node],
    destinations: [],
    providers: [],
    queue: [],
    offline: [],
  };
}

async function shown(node: SystemNode): Promise<string> {
  mounted = await mount(
    withServices(
      <System />,
      servedBy({
        "GET /system": [200, report(node)],
        "GET /backups": noBackups,
        "GET /now-playing": [200, { playbacks: [] }],
      }),
    ),
  );
  return mounted.container.textContent;
}

it("the System page says whether each machine runs FFmpeg in its sandbox (D112)", async () => {
  const text = await shown(connected);
  expect(text).toContain("local: FFmpeg runs without part of its sandbox");
  expect(text).toContain(
    "seccomp (which system calls FFmpeg may make) is not in this machine's kernel",
  );
});

it("the System page answers whether a machine sees its GPU, may open it, and encodes on it (D225)", async () => {
  const text = await shown(connected);
  expect(text).toContain("Video is encoded on the GPU, with Intel Quick Sync.");
  expect(text).toContain(
    "/dev/dri/renderD128 (driver i915) — Orpheum on this machine may open it.",
  );
  expect(text).toContain("What worked when Orpheum started on this machine");
  expect(text).toContain("NVIDIA NVENC: did not work: Cannot load libcuda.so.1");
  expect(text).toContain("Films/Metropolis (1995)/Metropolis.mkv");
  expect(text).toContain("6.5× real time");
  expect(text).toContain("1.5 TB free of 4 TB");
  expect(text).toContain("persistent.db");
  expect(text).toContain("encodes use h264_qsv");
});

it("the System page warns when a machine has a GPU but encodes on the processor", async () => {
  const text = await shown({
    ...connected,
    hardware: {
      gpus: [
        { devicePath: "/dev/dri/renderD128", driver: "i915", noAccessReason: "permission denied" },
      ],
      trials: [{ encoder: "h264_qsv", failure: "Failed to initialise VAAPI" }],
      chosenEncoder: "x264",
    },
  });
  expect(text).toContain("This machine has a GPU, but no hardware encoder works on it");
  expect(text).toContain("Orpheum on this machine may not open it: permission denied");
  expect(mounted?.container.querySelectorAll('[role="alert"]').length).toBe(2);
});

it("the System page shows each machine's build and system, and warns when it is not the server's (D225)", async () => {
  const same = await shown(connected);
  expect(same).toContain("v0.3.0 from commit 0f6c5cc1a2b3");
  expect(same).toContain("Debian GNU/Linux 13 (trixie)");
  expect(same).toContain("Codenametrixie");
  expect(same).toContain("Kernel6.12.107");
  expect(same).not.toContain("runs a different Orpheum build");
  mounted?.unmount();
  const other = await shown({
    ...connected,
    build: {
      name: "dev-0f6c5cc1a2b3-modified",
      version: "dev",
      commit: "0f6c5cc1a2b3",
      hasUncommittedChanges: true,
    },
    isServersBuild: false,
  });
  expect(other).toContain("This machine runs a different Orpheum build from the server's.");
  expect(other).toContain("(built with uncommitted changes)");
});

it("the System page says a machine that stopped answering is silent, and when it was last heard (D231 §11)", async () => {
  const silent = await shown({
    name: "local",
    connection: "silent",
    isConnected: false,
    isServersBuild: false,
    uid: 0,
    gid: 0,
    lastHeardAt: 1_790_000_000_000,
    confinement: [],
    disks: [],
    recentPrepares: [],
    devices: [],
    stuck: [],
  });
  expect(silent).toContain("Not answering. Last heard from at");
  expect(silent).not.toContain("FFmpeg runs");
  mounted?.unmount();
  const awaiting = await shown({
    name: "attic",
    connection: "awaiting_hello",
    isConnected: false,
    isServersBuild: false,
    uid: 0,
    gid: 0,
    confinement: [],
    disks: [],
    recentPrepares: [],
    devices: [],
    stuck: [],
  });
  expect(awaiting).toContain("Not heard from since the server started.");
});

it("the System page says a machine on another version of Orpheum needs updating (D233 §9)", async () => {
  const older = await shown({
    name: "attic",
    connection: "needs_update",
    isConnected: false,
    isServersBuild: false,
    uid: 0,
    gid: 0,
    confinement: [],
    disks: [],
    recentPrepares: [],
    devices: [],
    stuck: [],
  });
  expect(older).toContain("The media machine needs updating");
  expect(older).not.toContain("Not heard from since the server started.");
});

it("the System page says what is not answering, and each outside service's limits and state (D231 §5, §12)", async () => {
  const down = {
    destination: "tmdb-api",
    name: "TMDB",
    reason: "unreachable",
    downAt: 1_790_000_000_000,
    nextCheckAt: 1_790_000_480_000,
    isChecking: false,
  };
  mounted = await mount(
    withServices(
      <System />,
      servedBy({
        "GET /backups": noBackups,
        "GET /now-playing": [200, { playbacks: [] }],
        "GET /system": [
          200,
          {
            ...report(connected),
            destinations: [down],
            providers: [
              {
                name: "TMDB",
                destination: "tmdb-api",
                ratePerSecond: 40,
                backgroundRatePerSecond: 32,
                credentials: "accepted",
                down,
              },
              {
                name: "OpenSubtitles",
                destination: "opensubtitles",
                ratePerSecond: 5,
                backgroundRatePerSecond: 4,
                credentials: "missing",
                quota: {
                  remaining: 12,
                  allowed: 20,
                  reservedForPeoplePercent: 25,
                  reservedForPeople: 5,
                },
              },
            ],
          },
        ],
      }),
    ),
  );
  const text = mounted.container.textContent;
  expect(text).toMatch(/Not answering.*Waiting for TMDB: unreachable since .+, next check/u);
  expect(text).toContain(
    "TMDB asked at most 40 times a second, at most 32 of them for background work; its key or login works.",
  );
  expect(text).toContain("no key or login is set: an admin can set it in Settings.");
  expect(text).toContain("12 of 20 left today.");
});
