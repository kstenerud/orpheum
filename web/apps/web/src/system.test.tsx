// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, expect, it } from "vitest";

import { System, type SystemNode, type SystemReport } from "./system.tsx";
import { click, mount, servedBy, settleUntil, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

/** The backups section's answer when there are none (D242 §5). */
const noBackups = [200, { directory: "/srv/orpheum/backups", backups: [] }] as const;
const noResourceUse = [200, { summaries: [], hours: [] }] as const;

/** A server that keeps no network settings, as most of these tests' do not ask after (D462 §2). */
const noNetworkSettings = [
  409,
  { error: { code: "conflict", message: "no network settings" } },
] as const;
const noDiscovery = [
  200,
  {
    issuer: "",
    published: [],
    probe: [],
    claimExpiresAt: 0,
    lastHeartbeatAt: 0,
    portMapping: { isWanted: false, checkedAt: 0, mappings: [], problems: [] },
  },
] as const;

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
  confinement: [{ mechanism: "seccomp", isInForce: false, causeCode: "kernelWithout" }],
  hardware: {
    gpus: [{ devicePath: "/dev/dri/renderD128", driver: "i915" }],
    trials: [
      { encoder: "h264_qsv" },
      { encoder: "h264_nvenc", failure: "Cannot load libcuda.so.1" },
    ],
    chosenEncoder: "h264_qsv",
    isTooSlowToConvert: false,
  },
  disks: [{ path: "/media", totalBytes: 4_000_000_000_000, availableBytes: 1_500_000_000_000 }],
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
      isMultipathTcpOn: false,
      addressAtHome: { address: "", origin: "" },
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
    waiting: [],
    unread: [],
  };
}

async function shown(
  node: SystemNode,
  update: object = { version: "dev", checkedAt: 0 },
  network: readonly [number, object] = noNetworkSettings,
): Promise<string> {
  mounted = await mount(
    withServices(
      <System />,
      servedBy({
        "GET /system": [200, report(node)],
        "GET /backups": noBackups,
        "GET /system/resource-use": noResourceUse,
        "GET /config/unused": [200, { unusedSettings: [] }],
        "GET /filing-reports": [200, { reports: [] }],
        "GET /system/discovery": noDiscovery,
        "GET /system/update": [200, update],
        "GET /system/network": network,
        "GET /now-playing": [200, { playbacks: [] }],
      }),
    ),
  );
  return mounted.container.textContent;
}

it("the System page has a section of what the server and each node used of their machines", async () => {
  const text = await shown(connected);
  const headings = [...(mounted?.container.querySelectorAll("h2") ?? [])].map(
    (each) => each.textContent,
  );
  expect(headings).toContain("Resource use");
  expect(text).toContain("Nothing is measured yet");
});

it("the System page has the house's network, on a server that keeps its settings (D462 §2)", async () => {
  const home = { httpsPort: 8444, webrtcPort: 8444, router: "" };
  await shown(connected, undefined, [
    200,
    { kept: home, running: home, fellBack: [], pending: false, appliedBy: "restart" },
  ]);
  const headings = (): string[] =>
    [...(mounted?.container.querySelectorAll("h2") ?? [])].map((each) => each.textContent);
  await settleUntil(() => headings().includes("The house's network"));
  expect(headings()).toContain("The house's network");
});

it("the System page says whether each machine runs FFmpeg in its sandbox (D112)", async () => {
  const text = await shown(connected);
  expect(text).toContain("local: FFmpeg runs without part of its sandbox");
  expect(text).toContain(
    "seccomp (which system calls FFmpeg may make) needs a kernel built with seccomp filtering",
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
  expect(text).toContain("1.5 TB free of 4 TB");
  expect(text).toContain("persistent.db");
  expect(text).toContain("encodes use h264_qsv");
});

it("the System page names a machine that encodes on an AMD card through VAAPI (D332)", async () => {
  const text = await shown({
    ...connected,
    hardware: {
      gpus: [{ devicePath: "/dev/dri/renderD128", driver: "amdgpu" }],
      trials: [
        { encoder: "h264_qsv", failure: "Error creating a MFX session: -9" },
        { encoder: "h264_nvenc", failure: "Cannot load libcuda.so.1" },
        { encoder: "h264_vaapi" },
      ],
      chosenEncoder: "h264_vaapi",
      isTooSlowToConvert: false,
    },
  });
  expect(text).toContain(
    "Video is encoded on the GPU, with VAAPI (AMD, or another card with a VA driver).",
  );
  expect(text).toContain("VAAPI (AMD, or another card with a VA driver): works");
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
      isTooSlowToConvert: false,
    },
  });
  expect(text).toContain("This machine has a GPU, but no hardware encoder works on it");
  expect(text).toContain("Orpheum on this machine may not open it: permission denied");
  // The GPU's, and none for the sandbox's part the kernel lacks, which is a note (D454 §1).
  expect(mounted?.container.querySelectorAll('[role="alert"]').length).toBe(1);
});

it("the System page says how fast a machine converts (D457)", async () => {
  const fast = await shown({
    ...connected,
    hardware: {
      gpus: [],
      trials: [],
      chosenEncoder: "x264",
      encodeSpeedPercent: 450,
      isTooSlowToConvert: false,
    },
  });
  expect(fast).toContain("It converts video at 450% of the speed it plays.");
  expect(fast).not.toContain("too slow to convert");
});

it("the System page warns when a machine is too slow to convert a film as it plays (D457)", async () => {
  const slow = await shown({
    ...connected,
    hardware: {
      gpus: [],
      trials: [],
      chosenEncoder: "x264",
      encodeSpeedPercent: 60,
      isTooSlowToConvert: true,
    },
  });
  expect(slow).toContain("It converts video at 60% of the speed it plays.");
  expect(slow).toContain("That is too slow to convert a film while it plays");
  expect(slow).toContain("told this server can't convert it fast enough");
  expect(mounted?.container.querySelectorAll('[role="alert"]').length).toBe(1);
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
    devices: [],
    stuck: [],
  });
  expect(silent).toContain("Not answering. Last heard from at");
  // Where to look when it stops as it starts (N2); a kernel without Landlock or seccomp locks it
  // instead, shown by NodeUnconfinedLeave (D450 §1).
  expect(silent).toContain(
    "If it stops each time it starts, its log says why. See Setting up a server: problems seen",
  );
  expect(silent).not.toContain("FFmpeg runs");
  mounted?.unmount();
  const awaiting = await shown({
    name: "attic",
    connection: "awaitingHello",
    isConnected: false,
    isServersBuild: false,
    uid: 0,
    gid: 0,
    confinement: [],
    disks: [],
    devices: [],
    stuck: [],
  });
  expect(awaiting).toContain("Not heard from since the server started.");
});

it("the System page shows a locked machine's leave to run unconfined, not that it is not connected (D450 §1)", async () => {
  const locked = await shown({
    name: "nas",
    connection: "locked",
    isConnected: false,
    isServersBuild: false,
    uid: 0,
    gid: 0,
    confinement: [{ mechanism: "landlock", isInForce: false, causeCode: "disabledAtBoot" }],
    disks: [],
    devices: [],
    stuck: [],
  });
  expect(locked).toContain("This machine is locked");
  expect(locked).toContain("Run without confinement…");
  expect(locked).not.toContain("If it stops each time it starts, its log says why.");
});

it("the System page says a newer release is available and how to take it (D450 §2)", async () => {
  await shown(connected, {
    version: "v0.3.1",
    latest: "v0.3.3",
    command: "docker run --rm ghcr.io/kstenerud/orpheum:v0.3.3 install-script | sh",
    installRoute: "omv",
    checkedAt: 1_793_000_000_000,
  });
  const shownPage = mounted;
  await settleUntil(
    () => shownPage?.container.textContent.includes("v0.3.3 is available.") ?? false,
  );
  const headings = [...(shownPage?.container.querySelectorAll("h2") ?? [])].map(
    (each) => each.textContent,
  );
  expect(headings).toContain("Updates");
  expect(shownPage?.container.textContent).toContain("docker compose pull && docker compose up -d");
});

it("the System page says a machine on another version of Orpheum needs updating (D233 §9)", async () => {
  const older = await shown({
    name: "attic",
    connection: "needsUpdate",
    isConnected: false,
    isServersBuild: false,
    uid: 0,
    gid: 0,
    confinement: [],
    disks: [],
    devices: [],
    stuck: [],
  });
  expect(older).toContain("The media machine needs updating");
  expect(older).not.toContain("Not heard from since the server started.");
});

it("the System page says what is not answering, and each outside service's limits and state (D231 §5, §12)", async () => {
  const down = {
    destination: "tmdbApi",
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
        "GET /system/resource-use": noResourceUse,
        "GET /config/unused": [200, { unusedSettings: [] }],
        "GET /filing-reports": [200, { reports: [] }],
        "GET /system/discovery": noDiscovery,
        "GET /system/update": [200, { version: "dev", checkedAt: 0 }],
        "GET /system/network": noNetworkSettings,
        "GET /now-playing": [200, { playbacks: [] }],
        "GET /system": [
          200,
          {
            ...report(connected),
            destinations: [down],
            providers: [
              {
                name: "TMDB",
                destination: "tmdbApi",
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
                  reservedForUsersPercent: 25,
                  reservedForUsers: 5,
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

it("the System page says when the public name's certificate expires, or that there is none yet (D243 §6)", async () => {
  const withCertificate = async (expiresAt?: number): Promise<string> => {
    const shownReport = report(connected);
    shownReport.server.certificate =
      expiresAt === undefined
        ? { name: "media.orpheum-app.com" }
        : { name: "media.orpheum-app.com", expiresAt };
    mounted = await mount(
      withServices(
        <System />,
        servedBy({
          "GET /system": [200, shownReport],
          "GET /backups": noBackups,
          "GET /system/resource-use": noResourceUse,
          "GET /config/unused": [200, { unusedSettings: [] }],
          "GET /filing-reports": [200, { reports: [] }],
          "GET /system/discovery": noDiscovery,
          "GET /system/update": [200, { version: "dev", checkedAt: 0 }],
          "GET /system/network": noNetworkSettings,
          "GET /now-playing": [200, { playbacks: [] }],
        }),
      ),
    );
    return mounted.container.textContent;
  };
  const obtained = await withCertificate(1_797_000_000_000);
  expect(obtained).toContain("The certificate for media.orpheum-app.com");
  expect(obtained).toContain("it is renewed a month before.");
  mounted?.unmount();
  expect(await withCertificate()).toContain("Not obtained yet");
});

it("the System page notes a machine whose kernel has MPTCP on, and says nothing of it otherwise (D454 §4)", async () => {
  const withMultipathTCP = async (isOn: boolean): Promise<string> => {
    const shownReport = report(connected);
    shownReport.server.isMultipathTcpOn = isOn;
    mounted = await mount(
      withServices(
        <System />,
        servedBy({
          "GET /system": [200, shownReport],
          "GET /backups": noBackups,
          "GET /system/resource-use": noResourceUse,
          "GET /config/unused": [200, { unusedSettings: [] }],
          "GET /filing-reports": [200, { reports: [] }],
          "GET /system/discovery": noDiscovery,
          "GET /system/update": [200, { version: "dev", checkedAt: 0 }],
          "GET /system/network": noNetworkSettings,
          "GET /now-playing": [200, { playbacks: [] }],
        }),
      ),
    );
    return mounted.container.textContent;
  };
  expect(await withMultipathTCP(true)).toContain(
    "This machine's kernel has MPTCP (Multipath TCP) on.",
  );
  mounted?.unmount();
  expect(await withMultipathTCP(false)).not.toContain("MPTCP");
});

it("the System page opens with a contents row whose links focus their sections' headings (ui-audit-2026-11-04.md §3)", async () => {
  await shown(connected);
  const container = mounted?.container;
  const links = [...(container?.querySelectorAll('nav[aria-label="On this page"] a') ?? [])];
  const names = links.map((each) => each.textContent);
  expect(names).toEqual([
    "This server",
    "What doctor finds",
    "Outside services",
    "Machine: local",
    "Now playing",
    "Resource use",
    "Backups",
    "Reached from outside",
    "Remote access",
  ]);
  // Each link leads to the heading that names it.
  expect(
    links.map((each) => container?.querySelector(each.getAttribute("href") ?? "")?.textContent),
  ).toEqual(names);
  await click(links.find((each) => each.textContent === "Backups"));
  expect(document.activeElement).toBe(container?.querySelector("#system-backups"));
  await click(links.find((each) => each.textContent === "Machine: local"));
  expect(document.activeElement).toBe(container?.querySelector("#system-node-local"));
  expect(location.hash).toBe("");
});
