// SPDX-License-Identifier: MPL-2.0

/**
 * The English catalogue's System page: the server's build, its disks, its outside services'
 * state, the discovery and reachability checks, and offline/unreadable media (kept apart so the
 * catalogue fits in one read, en.ts).
 */
export const enSystem = {
  system: {
    heading: "System",
    contents: "On this page",
    server: "This server",
    build: "Orpheum build",
    commit: "from commit {{commit}}",
    committedAt: "of {{when}}",
    uncommittedChanges: "(built with uncommitted changes)",
    otherBuild:
      "This machine runs a different Orpheum build from the server's. They may not work together as they should: install the same build on both.",
    goVersion: "Go",
    distribution: "Distributor",
    description: "Description",
    release: "Release",
    codename: "Codename",
    kernel: "Kernel",
    platform: "Platform",
    startedAt: "Started",
    multipathTcpOn:
      "This machine's kernel has MPTCP (Multipath TCP) on. Orpheum opens no MPTCP connection, so the server is not affected; other programs on this machine may be, by a flaw reached through it (CVE-2026-98432). To turn it off, as root: sysctl net.mptcp.enabled=0.",
    certificate: {
      heading: "The certificate for {{name}}",
      expires: "Expires {{at}}; it is renewed a month before.",
      none: "Not obtained yet: the jobs list shows its renewal, and why it failed if it did.",
    },
    disks: "Disks",
    disk: "{{available}} free of {{total}}",
    diskUnmeasured: "could not be measured: {{why}}",
    stuck:
      "Waiting on a disk that does not answer: each ends when its disk answers, and the machine's other work goes on.",
    stuckWork: "{{program}}, reading {{path}}, since {{since}}",
    down: "Not answering",
    outside: "Outside services",
    providerRate:
      "asked at most {{rate}} times a second, at most {{background}} of them for background work;",
    credentials: {
      unknown: "its key or login not tried yet.",
      accepted: "its key or login works.",
      missing: "no key or login is set: an admin can set it in Settings.",
      refused: "it refused its key or login: an admin can change it in Settings.",
    },
    quota: "{{remaining}} of {{allowed}} left today.",
    quotaResets: "The count starts again at {{at}}.",
    waitUntil: "It asked us to wait until {{at}}.",
    stuckProgram: {
      ffmpeg: "FFmpeg",
      ffprobe: "FFprobe",
      node: "The scan",
    },
    findings: "What doctor finds",
    verdict: {
      ok: "OK",
      warn: "Check",
      fail: "Problem",
    },
    action: "What to do: {{action}}",
    node: "Machine: {{node}}",
    awaitingHello: "Not heard from since the server started.",
    notConnectedHint:
      "If it stops each time it starts, its log says why. See Setting up a server: problems seen, and their fixes.",
    unconfined: {
      locked:
        "This machine is locked: part of the sandbox FFmpeg runs in could not be put in force, so it reads no films and runs no FFmpeg.",
      risk: "FFmpeg reads every film this machine serves. A film made to attack FFmpeg could, without the sandbox, read or change anything this machine's Orpheum user can. The safe fix is the kernel's: see Setting up a server: problems seen, and their fixes. If you cannot fix it and trust every file in your libraries, you can run it without the sandbox.",
      allow: "Run without confinement…",
      consequence:
        "This machine then runs FFmpeg without the parts of its sandbox listed above. Your name and the time are recorded. To stop it, stop the machine.",
      allowConfirm: "Accept the risk and run it",
      given: "Runs without confinement: {{by}} accepted the risk on {{at}}.",
    },
    needsUpdate:
      "The media machine needs updating: it runs another version of Orpheum than the server, and its work waits until it is updated.",
    retireNode: "This machine is gone for good…",
    retireNodeConsequence:
      "Its folders leave every library and its waiting work is cancelled; what was watched is kept. If it comes back, it joins as a new machine.",
    retireNodeConfirm: "Retire the machine",
    queue: {
      heading: "Work waiting",
      waiting: "{{count}} waiting",
      oldest: "since {{at}}",
      failed: "{{count}} failed in the last day",
    },
    work: {
      running: "{{running}} of {{slots}} jobs it runs at once are running;",
      waiting: "{{count}} waiting.",
      lastMoved: "Last made progress {{at}}.",
      broken:
        "{{count}} times it held work the server had not given it or had ended: a fault, in it or in the server.",
    },
    devices: {
      heading: "Disks it reads",
      kind: {
        spinning: "spinning disk;",
        ssd: "solid-state disk;",
        network: "network share;",
        unknown: "disk of unknown kind;",
      },
      readersSet: "{{count}} may read it at once, set for this disk.",
      readersOfKind: "{{count}} may read it at once, as for its kind.",
      readersLabel: "At once:",
      set: "Set",
      useKinds: "Use its kind's",
    },
    backups: {
      heading: "Backups",
      what: "A backup keeps everything that cannot be made again: the database, with what everyone watched and chose, and the server's settings. The media is not in it. The server makes one on its own each day, without the server's keys, and keeps the newest; the backup settings say when and how many.",
      asked: "A backup is on its way: the jobs list shows it, and it appears here once it is made.",
      includeKey: "Put the server's keys in it, to move the server to another machine",
      includeKeyWarning:
        "Whoever has a backup with the keys can read the passwords kept on the server and pass for the server itself. Keep it somewhere safe.",
      make: "Back up now",
      create: "Create backup",
      where:
        "Backups are kept in {{directory}} on the server's machine. A backup on the same disk does not survive that disk: copy them elsewhere.",
      none: "There is no backup yet.",
      list: "The backups kept ({{count}})",
      backup: "{{name}}, {{size}}, made {{at}}",
    },
    resourceUse: {
      heading: "Resource use",
      what: "What the server and each media node used of their machines, measured each minute and kept for a month, beside what they were doing. A node's use counts the FFmpeg it starts. CPU is a share of one core, so a node using four cores shows 400%.",
      none: "Nothing is measured yet: the first figures come a minute after the server starts.",
      hours: "Its hours of the last day ({{count}})",
      core: "The server",
      node: "Media node {{name}}",
      summary: "Since {{since}}: CPU {{cpu}}; memory {{memory}}.",
      hour: "Hour from",
      cpu: "CPU",
      memory: "Memory",
      doing: "Doing",
      cpuUse: "{{percent}}% (busiest minute {{busiest}}%)",
      cpuUnknown: "not yet known",
      memoryUse: "{{held}} held, {{cache}} cached",
      withPeak: "{{use}}, {{peak}} at most",
      plays: "{{count}} playing",
      idle: "idle",
      running: "{{kind}} × {{count}}",
    },
    videos: {
      heading: "The server's own videos",
      where: "Kept on {{node}} in {{folder}}.",
      nowhere: "Not made yet: the first use makes them on a connected node.",
      ready: "{{name}}: made, {{size}}",
      notReady: "{{name}}: not made yet",
      rebuild: "Make them again",
    },
    unusedSettings: {
      heading: "Settings this release does not read",
      what: "These were left by an earlier release, or mistyped. The server ignores them, so deleting one changes nothing it does.",
      retired: "{{key}} = {{value}}: retired in {{release}}, {{reason}}",
      typo: "{{key}} = {{value}}: no release has read it, so probably a typo",
      delete: "Delete",
      deleteFor: "Delete {{key}}",
      consequence: "{{key}} is deleted for good.",
      confirm: "Delete it",
    },
    discovery: {
      heading: "Remote access",
      what: "A discovery tells your devices where to find this server, so you reach it from outside the house without a VPS of your own in front of it. It passes on addresses, never your films.",
      issuer: "The discovery's name",
      connect: "Connect",
      wrongPasswordToConnect: "That isn't your password. The server is not connected.",
      notAnIssuer:
        "That isn't a discovery's name. Type the name it gave you, as in discovery.example.",
      suggestedUnknown:
        "This version of the server doesn't know the keys of {{issuer}} yet, so it can't trust it. Update the server, or connect to another discovery.",
      at: "Connected to {{name}}, at {{issuer}}.",
      atService: "Connected to the Orpheum service, at {{issuer}}.",
      identity: "Its identity is {{identity}}.",
      compareIdentity:
        "{{name}}, at {{issuer}}, shows this identity. Compare it with the one its operator shows you, and connect only if they are the same:",
      identityMatches: "They are the same: connect",
      identityCancel: "Cancel",
      notTheIdentity:
        "That discovery's identity is not the one confirmed. Nothing was connected; ask its operator.",
      rootKeys: "Trusted by the discovery's keys {{keys}} (version {{version}}).",
      documentStale:
        "The discovery's addresses expired on {{at}}: this server cannot confirm they are current until the discovery signs them again.",
      documentProblem:
        "The discovery's addresses couldn't be checked for an update; the server keeps the last ones it trusted. Its log says why.",
      realm:
        "People sign in to this server at the discovery's page by the realm {{realm}}, with their name and password.",
      state: {
        pending: "Waiting to be claimed.",
        claimed: "Claimed: clients can find this server.",
        grace:
          "The subscription has lapsed; clients can still find this server for a few more days.",
        frozen:
          "The subscription has ended: clients cannot find this server through the discovery until it is renewed.",
        released:
          "The discovery has let this server go. Leave it, and connect again to start over.",
        disabled: "The discovery has turned this server off. Its operator can say why.",
      },
      claim:
        "To claim this server, sign in at the discovery and enter the code {{code}}. It works until {{until}}.",
      key: "Your account's key",
      keyHint:
        "Sign in to your account at the discovery, choose Make a key under Your server, and type the key here, with your own password on this server: it lets you sign in by your realm from anywhere. The key works for an hour.",
      yourPassword: "Your password",
      claimByKey: "Claim",
      wrongPassword: "That isn't your password. Nothing was claimed, and the key still works.",
      wrongKey:
        "The discovery didn't accept that key. Check it, or make a new one on your account.",
      notPending: "This server isn't waiting to be claimed.",
      account: "Enrolled with the account {{account}}.",
      removed:
        "Another server was enrolled with this server's account at {{at}}, in its place. Clients no longer find this one through the discovery.",
      reachable: {
        publicIpv4: "Clients can reach it from anywhere, at its public IPv4 address.",
        publicIpv4OtherPort:
          "Clients reach it at its public IPv4 address, on a port some networks block; there, they connect over IPv6 or WebRTC.",
        ipv6: "Clients with IPv6 reach it directly; the rest connect over WebRTC.",
        nowhere:
          "The discovery could not reach it from outside. Clients connect over WebRTC, and at home directly.",
      },
      published: "Published as {{names}}.",
      deadEnds:
        "Times in the last week a device found no way to reach this server: {{count}}. Most often the device was on a VPN or a travel eSIM, which can't reach a home server. Giving this machine a public IPv6 address helps the most: devices with IPv6 then reach it directly. Turning on UPnP, NAT-PMP or PCP on the home router helps devices without.",
      deadEndCauses: "Why, as each device judged it: {{causes}}.",
      deadEndCause: {
        noIpv6:
          "{{count}} on a network that seemed to have no IPv6, while this server had no IPv4 way in",
        secureUnfinished:
          "{{count}} where the server answered but the secure connection never completed in the device's browser",
        unknown: "{{count}} where the device couldn't tell why",
        none: "{{count}} from a device too old to say why",
        other: "{{count}} for another reason",
      },
      heartbeat: "Last told the discovery its addresses at {{at}}.",
      copyRefused:
        "A copy of this server tried to start from {{from}} at {{at}}, and was refused: this one serves.",
      stoodAside:
        "Another copy of this server serves. This one stopped asking at {{at}} and serves only at home. Asking again takes over only once the other copy has stopped.",
      askAgain: "Ask again",
      problem: "The last call to the discovery failed: {{problem}}",
      clientOrigin: "Links to this server open at {{origin}}.",
      homeAddressWaiting:
        "This server's home address is being set up: its certificate usually comes within a few minutes of the claim. Until it does, the discovery's page at home reaches the server the long way, and you can't administer through it; this server's own page, at its address on your home network, works meanwhile.",
      fingerprint:
        "This server's fingerprint is {{fingerprint}}. Its links carry it, so a client knows it reached this server and no other; to check by eye, compare the first and last groups.",
      deprecated:
        "The discovery stops speaking this server's protocol on {{date}}. Update the server before then.",
      portMapping: {
        opened: "The home router opened the server's port:",
        none: "The home router did not open the server's port. Turn on UPnP, NAT-PMP or PCP on it, or forward the port by hand.",
        mapping: "{{external}} reaches {{internal}}, by {{method}}.",
        method: { pcp: "PCP", natpmp: "NAT-PMP", upnp: "UPnP" },
        problems: "What the router said",
      },
      paths: {
        heading: "How clients' WebRTC connections reached the server since it started:",
        path: {
          directIpv6: "directly over IPv6: {{count}}",
          directIpv4: "directly over IPv4: {{count}}",
          punchedIpv6: "through the router's firewall over IPv6: {{count}}",
          punchedIpv4: "through the router's NAT over IPv4: {{count}}",
          hairpinnedIpv4:
            "out through the provider and back over IPv4, one address at both ends, not across the house's network: {{count}}",
          relayed: "through a relay: {{count}}",
          failed: "failed to connect: {{count}}",
        },
      },
      houseNetworks: {
        heading:
          "The discovery recognises devices at home by your home's IPv6 networks, and offers them this server's address at home:",
        learnt:
          "{{network}}, learnt from the device at {{device}}, which reached this server directly at home at {{at}}",
        configured:
          "{{network}}, named in the config. It goes stale when your provider moves the prefix; devices that reach this server directly at home teach it the new one.",
      },
      leave: "Leave the discovery",
      leaveConsequence: "Clients outside the house will no longer find this server through it.",
      leaveConfirm: "Leave",
    },
    reachability: {
      heading: "Reached from outside",
      what: "Checks the way a browser outside the house reaches the server: its public name, through the VPS and the tunnel, and the voice relay's port.",
      check: "Check now",
      checkedAt: "Checked {{at}}.",
      points: "{{name}} points at {{addresses}}.",
      hop: {
        https: "HTTPS to {{name}}",
        relayUdp: "The voice relay over UDP",
        relayTcp: "The voice relay over TCP",
      },
      outcome: {
        reached: "{{hop}}: reached this server.",
        notChecked: "{{hop}}: not set up in the config, so not checked.",
        nameUnresolved: "{{hop}}: the name has no address. Check its DNS records.",
        notAnswering:
          "{{hop}}: nothing answered. Check that the VPS is up, that it forwards the port, and that the tunnel is connected.",
        certificateRefused:
          "{{hop}}: something answered, but not with a certificate for the name. The certificate may not have been issued yet.",
        anotherServer:
          "{{hop}}: another server answered for the name. Check where its DNS records point.",
      },
      detail: "The cause: {{detail}}",
    },
    offline: "Offline",
    offlineRoot: "{{node}}: {{path}}, in {{library}}: offline since {{since}}",
    offlineDisk: "What was mounted at {{node}}: {{path}}, in {{library}}: missing since {{since}}",
    offlineHelp:
      "Nothing of theirs is removed until they come back or you say they are gone for good.",
    offlineWhere: "Libraries",
    waiting: "Waiting for you",
    waitingExplain:
      "A scan found files gone and holds back removing them until you say they are: a disk that is not mounted looks the same. Everything else in the folder plays meanwhile. If the files should be there, put them back and scan again.",
    waitingRoot:
      "{{node}}: {{path}}, in {{library}}: its scan of {{since}} would remove {{count}} files, because {{reason}}.",
    waitingMore: "and {{count}} more",
    waitingRemove: "Remove these {{count}} files",
    waitingRemoveConsequence: "They leave the library; a scan finds them again if they come back.",
    waitingRemoveConfirm: "Remove them",
    unread: "Could not be read",
    unreadWhere: "in {{library}}, on {{node}}:",
    unreadWhy: "{{text}}. It belongs to user {{owner}} and group {{group}}, with mode {{mode}}.",
    unreadFix:
      "The node reads as user {{uid}} and group {{gid}}. On the machine that holds it, give it to that user (chown -R {{uid}}:{{gid}}) or let everyone read it (chmod -R a+rX), then scan the library again.",
    unreadFixUnknownNode:
      "Make it readable to the user the node runs as, which its System entry shows once it connects, then scan the library again.",
    silentSince: "Not answering. Last heard from at {{at}}.",
    ffmpeg: "FFmpeg",
    runsAs: "Runs as",
    uidGid: "user {{uid}}, group {{gid}}",
    lastHeard: "Last heard from",
    hardware: "Hardware encoding",
    encodesWith: "Video is encoded on the GPU, with {{encoder}}.",
    noGPU: "No GPU was found, so video is encoded on the processor, with x264.",
    gpuUnused:
      "This machine has a GPU, but no hardware encoder works on it, so video is encoded on the processor, with x264. The trials below say why.",
    gpuDevices: "What it sees",
    seesNothing: "No GPU device.",
    driver: "(driver {{driver}})",
    canOpen: "— Orpheum on this machine may open it.",
    cannotOpen: "— Orpheum on this machine may not open it: {{why}}",
    trials:
      "What worked when Orpheum started on this machine (each tried in the sandbox a real encode runs in)",
    trialWorked: "works",
    trialFailed: "did not work: {{why}}",
    encoder: {
      h264Qsv: "Intel Quick Sync",
      h264Nvenc: "NVIDIA NVENC",
      h264Vaapi: "VAAPI (AMD, or another card with a VA driver)",
      x264: "x264, on the processor",
      copy: "copied, not encoded",
      unknown: "not recorded",
    },
    preparedUsage:
      "Films' chunks: {{used}} of a {{budget}} budget; the disk is {{percent}}% full, its limit {{limit}}%.",
  },
} as const;
