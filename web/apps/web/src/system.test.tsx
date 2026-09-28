// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, expect, it } from "vitest";

import { System, type SystemNode, type SystemReport } from "./system.tsx";
import { mount, servedBy, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

const connected: SystemNode = {
  name: "local",
  isConnected: true,
  version: "0.3.0",
  platform: "linux/amd64",
  ffmpegVersion: "ffmpeg version 7.1",
  ffmpegHash: "0123456789abcdef",
  uid: 1000,
  gid: 1000,
  lastHeardAt: 1_790_000_000_000,
  confinement: [{ mechanism: "seccomp", isInForce: false, cause: "kernel_without" }],
  hardware: {
    gpus: [{ device: "/dev/dri/renderD128", driver: "i915" }],
    trials: [
      { encoder: "h264_qsv" },
      { encoder: "h264_nvenc", failure: "Cannot load libcuda.so.1" },
    ],
    encoder: "h264_qsv",
  },
  disks: [{ path: "/media", totalBytes: 4_000_000_000_000, freeBytes: 1_500_000_000_000 }],
  recentPrepares: [
    {
      fileId: 42,
      path: "Films/Metropolis (1995)/Metropolis.mkv",
      profile: "h264-8m",
      state: "complete",
      videoEncoder: "h264_qsv",
      speed: 6.5,
      finishedAt: 1_790_000_000_000,
    },
  ],
};

function report(node: SystemNode): SystemReport {
  return {
    server: {
      version: "0.3.0",
      platform: "linux/amd64",
      goVersion: "go1.26.1",
      startedAt: 1_790_000_000_000,
      disks: [
        { path: "/var/lib/orpheum", totalBytes: 500_000_000_000, freeBytes: 250_000_000_000 },
      ],
      databases: [
        { name: "library.db", sizeBytes: 40_000_000 },
        { name: "cache.db", sizeBytes: 5_000_000 },
      ],
    },
    findings: [
      { check: "hardware encoding on node local", verdict: "ok", detail: "encodes use h264_qsv" },
    ],
    nodes: [node],
  };
}

async function shown(node: SystemNode): Promise<string> {
  mounted = await mount(withServices(<System />, servedBy({ "GET /system": [200, report(node)] })));
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
  expect(text).toContain("/dev/dri/renderD128 (driver i915) — the server may open it.");
  expect(text).toContain("NVIDIA NVENC: did not work: Cannot load libcuda.so.1");
  expect(text).toContain("Films/Metropolis (1995)/Metropolis.mkv");
  expect(text).toContain("6.5× real time");
  expect(text).toContain("1.5 TB free of 4 TB");
  expect(text).toContain("library.db");
  expect(text).toContain("encodes use h264_qsv");
});

it("the System page warns when a machine has a GPU but encodes on the processor", async () => {
  const text = await shown({
    ...connected,
    hardware: {
      gpus: [{ device: "/dev/dri/renderD128", driver: "i915", noAccess: "permission denied" }],
      trials: [{ encoder: "h264_qsv", failure: "Failed to initialise VAAPI" }],
      encoder: "x264",
    },
  });
  expect(text).toContain("This machine has a GPU, but no hardware encoder works on it");
  expect(text).toContain("the server may not open it: permission denied");
  expect(mounted?.container.querySelectorAll('[role="alert"]').length).toBe(2);
});
