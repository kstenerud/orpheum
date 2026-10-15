// SPDX-License-Identifier: MPL-2.0

import { enDevices } from "./en-devices.ts";
import { enDiscovery } from "./en-discovery.ts";
import { enLibraries } from "./en-libraries.ts";
import { enPeople } from "./en-people.ts";
import { enStatus } from "./en-status.ts";
import { enTogether } from "./en-together.ts";
import { enWarning } from "./en-warning.ts";

/**
 * The English catalogue, bundled as the offline floor (D51). Other languages are served by the
 * server once translations exist. Keys name what the text is for, never what it says.
 */
export const en = {
  ...enTogether,
  ...enDevices,
  ...enDiscovery,
  ...enLibraries,
  ...enPeople,
  ...enStatus,
  ...enWarning,
  nav: {
    films: "Films",
    series: "Series",
    search: "Search",
    watchTogether: "Watch Together",
    identify: "Needs a match",
    libraries: "Libraries",
    settings: "Settings",
    jobs: "Jobs",
    system: "System",
    people: "People",
    thisDevice: "This device",
    account: "Your account",
    about: "About",
    signOut: "Sign out",
    manage: "Manage",
    togetherOn: "on now",
    menu: "Menu",
    signedInAs: "{{name}}",
    signedInAsOn: "{{name}} on {{device}}",
    realmTitle: "Signed in to the realm {{realm}}",
    label: "Main",
  },
  signIn: {
    heading: "Sign in",
    name: "Name",
    password: "Password",
    submit: "Sign in",
    wrongNameOrPassword: "The name or password is wrong.",
    tooManyAttempts: "Too many failed sign-ins. Wait a few minutes and try again.",
    noAdminYet:
      "No one can sign in until the first admin is created with the setup code from the server's log.",
    useCode: "Get a code",
  },
  problem: {
    signedOut: {
      sessionEnded:
        "This device was signed out: it was removed from your devices, or signed out from another device. Sign in again, or get a code and enter it on another device where you are signed in.",
      sessionExpired: "You were signed out after a long time away. Sign in again.",
      noSession: "You are not signed in. Sign in to continue.",
      signOutUnheard:
        "You are signed out on this device, but the server could not be told, so its session may still work until it expires. To end it now, remove this device from Your account on another device.",
    },
    forbidden: "Only an administrator can do that.",
    notFound: "That is no longer here: it may have been removed. Go back and try again.",
    playbackGone:
      "The server restarted, or ended this playback, so it stopped. Press play to carry on from where you were.",
    invalidRequest: "The server could not accept that. Check what was entered and try again.",
    invalidFields:
      "The server did not accept what was entered in {{fields, list}}. Correct what is there and try again.",
    conflict: "That clashes with something that already exists.",
    stale: "Someone changed this meanwhile. Reload and try again.",
    rateLimited: "Too many attempts. Wait a minute, then try again.",
    quotaExhausted: "Today's allowance for this is used up. Try again tomorrow.",
    providerError: "An outside service did not answer properly. Try again later.",
    nodeSilent:
      "The machine that holds this file is not answering. Check that it is on and connected.",
    fileUnavailable:
      "The file is not where it was. It may have been moved or its disk unplugged; scanning the library again finds it.",
    mediaUnreachable:
      "This cannot be played now: the machine, folder or mount that holds it is missing.",
    mediaReason: {
      nodeAway:
        "This cannot be played now: {{node}}, the machine that holds it, is not connected{{since}}.",
      rootMissing:
        "This cannot be played now: the folder {{path}} on {{node}} is missing{{since}}.",
      diskMissing:
        "This cannot be played now: what was mounted at {{path}} on {{node}} is missing{{since}}.",
    },
    sinceWhen: ", since {{since}}",
    wait: {
      stillWaiting: "Still waiting for {{provider}}…",
      checking: "Checking whether {{provider}} answers now…",
      askedToWait: "{{provider}} asked us to wait. Trying again in {{seconds}} s…",
    },
    nextCheck: "; it will be checked again at {{at}}",
    preparing: "It is still being prepared. Wait a moment.",
    prepareFailed: "This could not be prepared for this device.",
    noRoomToMake:
      "The server cannot make this quality as fast as it plays while it makes others for other viewers. Try again in a while.",
    migrating: "The server is starting up, or updating. It will answer in a moment.",
    setupRequired: "The server has not been set up yet.",
    internal:
      "Something went wrong on the server. Try again; if it keeps happening, the server's log says why.",
    unreachable: "The server could not be reached. Check the connection and try again.",
    notAnswering:
      "The server is not answering: it may be restarting, or the machine it runs on may be off.",
    askingAgain: "This page tries again by itself.",
    unknown:
      "Something went wrong that this app does not expect, which is likely a bug. Try again; the browser's console and the server's log say what happened.",
    providerReason: {
      noKey: "The outside service has no key set up. An admin can set it in Settings.",
      keyRefused: "The outside service refused its key. An admin can change it in Settings.",
      noLogin: "The outside service has no account set up. An admin can add one in Settings.",
      loginRefused:
        "The outside service refused the account's login. An admin can change it in Settings.",
      notConfigured: "This server does not use that outside service.",
      unavailable: "The outside service is not answering right now. Try again later.",
      unavailableSince: "The outside service has not been answering{{since}}{{next}}.",
      unreadable: "The outside service sent something that could not be read. Choose another.",
    },
  },
  codeSignIn: {
    heading: "Sign in with a code",
    explain:
      "On a phone or computer where you are signed in, open Your account and enter this code under Activate a device.",
    code: "Code",
    checkCode: "Check code",
    checkExplain:
      "Your other device now shows a check code too. If it is this same code, say so there.",
    renewed: "The last code ended, so here is a new one.",
    signingIn: "Signed in.",
    back: "Sign in with a name and password instead",
  },
  register: {
    heading: "Choose your password",
    explain: "This link is yours, and works once. Choose the password you will sign in with.",
    explainFor:
      "This link is {{name}}'s, and works once. Choose the password you will sign in with.",
    invitedHeading: "Make your account",
    invitedExplain:
      "This invitation is yours, and works once. Choose the name the others will see you by and sign in with, and your password.",
    name: "Your name",
    nameTaken: "Someone already has that name. Choose another.",
    password: "Password",
    again: "The same password again",
    submit: "Set password and sign in",
    noLink: "This address has no registration link in it. Open the link you were given.",
    mismatch: "The two passwords are not the same.",
    refused: "{{refusalText}}",
    badLink:
      "This link does not work: it was used, replaced by a newer one, or has expired. Ask the person who made it for a new one.",
  },
  account: {
    heading: "Your account",
    yourLink: "Your link",
    yourLinkText:
      "Open this link on another phone or computer, or scan the code with its camera, to find this server again. It signs no one in.",
    yourLinkQr: "Your link, as a QR code",
    deviceName: "The device's name",
    changePassword: "Change password",
    currentPassword: "Current password",
    newPassword: "New password",
    passwordChanged: "Your password is changed. Your other devices stay signed in.",
    wrongPassword: "That is not your current password. Check it and try again.",
    notOnThisDevice:
      "This device was activated by a code, so it can watch and host but not change your password, remove devices or activate others. Do that on a device you signed in to with your password.",
  },
  activate: {
    heading: "Activate a device",
    explain:
      "On the device you want to sign in, a television say, open {{origin}} and choose Get a code. Enter the code it shows here.",
    code: "Code",
    codeExample: "xxxx-xxxx",
    deviceName: "Its name, for a new device",
    deviceNameExample: "Living room TV",
    submit: "Activate",
    noSuchCode:
      "No device is showing that code. Check it, or choose Get a code on the device again: a code lasts ten minutes.",
    tooManyCodes: "Too many codes that did not work. Wait a few minutes and try again.",
    checkExplain: "{{device}} now shows a check code. This is the one it should show:",
    checkCode: "Check code",
    warningHeading: "Look at the device's screen yourself.",
    warning:
      "Only say they match if you see this same code on the device's own screen, in front of you. Never go on what someone tells you, on a call or in a message: a code someone else gives you lets their device into your account.",
    match: "Yes, they match",
    noMatch: "They don't match",
    activated: "{{device}} is activated and signed in.",
    notActivated: "Nothing was activated.",
  },
  nowPlaying: {
    heading: "Now playing",
    none: "Nothing is playing.",
    who: "{{person}} on {{device}}: {{name}}, since {{since}}.",
    noDevice: "a device with no name",
    direct: "Played as it is.",
    prepared: "A prepared copy, because of: {{reasons}}.",
    message: "Message",
    stop: "Stop it",
  },
  share: {
    link: "It works once, until {{expires}}.",
    togetherLink: "It works once, for the person you made it for, while the session lasts.",
  },
  about: {
    heading: "About",
    tmdbHeading: "Film and series information",
    tmdbLogo: "The Movie Database (TMDB)",
    tmdbNotice: "This product uses the TMDB API but is not endorsed or certified by TMDB.",
    thanksHeading: "Built on",
    thanksIntro: "{{product}} stands on the work of these projects, with thanks.",
  },
  browse: {
    films: "Films",
    series: "Series",
    sort: "Sort",
    byName: "By name",
    byAdded: "Newest first",
    empty: "Nothing here yet. Once a library is scanned, what it holds shows here.",
    noLibrary:
      "You have not been given any library yet, so there is nothing to watch. Ask whoever runs this server to share one with you.",
    year: "{{year}}",
  },
  title: {
    runtime: "{{minutes}} min",
    season: "Season {{season}}",
    episodeCount: "Episodes: {{count}}",
    allSeasons: "All seasons",
    place: "{{series}} · season {{season}}, episode {{number}}",
    versions: "Versions",
    version: "Version {{index}}",
    fileDetails: "File details",
    extras: "Extras",
    unidentified: "Not identified yet",
    identifiedIncorrectly: "Identified incorrectly",
    streamVideo: "Video: {{codec}}",
    streamAudio: "Audio: {{codec}} {{language}} {{channels}}",
    streamSubtitle: "Subtitles: {{codec}} {{language}}",
    channels: "{{count}} channels",
    episodeLabel: "{{number}}. {{name}}",
    notOnDisk: "not on disk",
    unavailableNow: "Cannot be played now",
    retireFile: "This file is gone for good…",
    retireFileConsequence:
      "It leaves the library; what was watched is kept, and if it comes back it is found again.",
    retireFileConfirm: "Retire the file",
    specials: "Specials",
    play: "Play",
  },
  play: {
    updateAfterFilm: "A new version of this app is ready: it loads as you leave the film.",
    direct: "Playing the original file, as it is.",
    directFailed: "This browser could not play the original file: {{reason}}.",
    prepared: "Playing a copy prepared for this browser, because {{reasons}}.",
    preparing: "Preparing — ready in a few seconds.",
    preparedFailed: "The prepared copy could not play: {{reason}}.",
    lowerQualityStarting:
      "Your connection could not keep up, so this moves to a lower quality: {{height}}p, {{rate}}.",
    ladderStarting: "Starting at {{height}}p, {{rate}}. The quality follows your connection.",
    ladderPlaying: "Playing at {{height}}p, {{rate}}. The quality follows your connection.",
    lowerQuality:
      "Playing a lower quality for your connection: {{height}}p, {{rate}}. It goes back up when the connection allows.",
    cannotKeepUpUnmeasured:
      "Even the lowest quality, {{lowest}}, is too much for this connection: it will keep pausing.",
    cannotKeepUp:
      "Your connection carries about {{link}}, and even the lowest quality needs {{lowest}}: it will keep pausing.",
    failure: {
      hlsUnsupported: "this browser cannot play streamed video",
      aborted: "loading it was stopped",
      network: "it could not be fetched from the server",
      decode: "this browser could not decode it",
      unsupported: "this browser does not play its format",
      tooBusy:
        "the server cannot make this quality as fast as it plays while it makes others for other viewers; try again in a while",
      unknown: "the browser gave no reason",
    },
    boost: "Dialogue boost",
    level: {
      switch: "Level loudness",
      raised: "Raised {{db}} dB, to the level of other films.",
      lowered: "Lowered {{db}} dB, to the level of other films.",
      atLevel: "Already at the level of other films.",
      asMixed: "Playing as it was mixed.",
      notMeasured: "Its loudness is not measured yet, so it plays as it is.",
      deviceVolume:
        "This device sets its own volume, so only a copy prepared after measuring is levelled here.",
    },
    startOver: "Start over",
    stoppedByAdmin: "The administrator stopped this: {{message}}",
    positionNotKept: "Where you are is no longer being kept for next time. {{reason}}",
    audio: {
      label: "Sound",
      unknownLanguage: "Unknown language",
      commentary: "commentary",
      described: "described for the blind and partially sighted",
    },
    subtitles: {
      label: "Subtitles",
      off: "Off",
      unknownLanguage: "Unknown language",
      forced: "forced",
      hearingImpaired: "for the deaf and hard of hearing",
      earlier: "Earlier",
      later: "Later",
      offset: "Subtitle offset: {{seconds}} s",
      failed: "The subtitles could not be shown: {{reason}}.",
      fetched: "downloaded, unverified",
    },
    findSubtitles: {
      open: "Find subtitles…",
      heading: "Find subtitles",
      language: "Language",
      search: "Search OpenSubtitles",
      allowance: "{{remaining}} of {{allowed}} downloads left today.",
      unverified:
        "Downloaded subtitles aren't checked against the sound, so one can run early or late, or be timed for a different cut. If it is, the Earlier and Later buttons shift it.",
      none: "OpenSubtitles has no {{language}} subtitles for this video.",
      noneForThisFile: "OpenSubtitles has no {{language}} subtitles made for this exact file.",
      fileHashOnly:
        "This video isn't identified yet, so only subtitles made for this exact file were searched for. Once it is identified, a search also finds those made for other copies of it.",
      madeForThisFile: "made for this very file",
      machineTranslated: "machine-translated",
      downloads: "downloads: {{count}}",
      fetch: "Download",
      quotaSpent: "Today's downloads are spent; they start again at {{when}}.",
      nothingToSearchBy:
        "This file is not identified yet, so there is nothing to ask OpenSubtitles by. Identify it first, or scan it again.",
      reason: {
        noLogin: "No OpenSubtitles account is set up. An admin can add one in Settings.",
        loginRefused:
          "OpenSubtitles refused the account's login. An admin can change it in Settings.",
        notConfigured: "This server does not download subtitles.",
        unavailable: "OpenSubtitles is not answering right now. Try again later.",
        unavailableSince: "OpenSubtitles has not been answering{{since}}{{next}}.",
        unreadable: "That subtitle could not be read, so it was not kept. Choose another.",
        unknown: "OpenSubtitles could not be reached.",
      },
    },
    reason: {
      container: "the browser cannot open its container",
      videoCodec: "the browser cannot decode its video",
      audioCodec: "the browser cannot decode its sound",
      bandwidth: "it is too large for the connection",
      dialogueBoost:
        "this device plays stereo, so its surround sound is mixed here and the dialogue boost can act",
      audioTrack: "the sound you chose is not the one the file plays first",
      userChoice: "you asked for it to be prepared",
    },
  },
  nodes: {
    confined:
      "{{node}}: FFmpeg runs in its sandbox, so a hostile media file can reach only the file being read.",
    unconfined:
      "{{node}}: FFmpeg runs without part of its sandbox, so a hostile media file could reach more of this machine than it should.",
    mechanism: {
      landlock: "Landlock (which files FFmpeg may open)",
      seccomp: "seccomp (which system calls FFmpeg may make)",
    },
    cause: {
      disabledAtBoot:
        "{{mechanism}} is in the kernel but not switched on at boot. Add landlock to the lsm= setting on the kernel command line, then restart the machine.",
      kernelWithout:
        "{{mechanism}} is not in this machine's kernel. A newer kernel has it (5.13 or later for Landlock).",
      blockedByRuntime:
        "The container this runs in blocks {{mechanism}}. Update the container software, or allow it in the container's settings.",
      notLinux: "{{mechanism}} exists only on Linux, and this machine runs another system.",
      architectureUnwritten:
        "{{mechanism}} is not yet written for this machine's kind of processor; it is for x86-64 and ARM64.",
      unexplained:
        "{{mechanism}} is not working here, for a reason the machine could not name. The server's log has its exact answer.",
    },
  },
  system: {
    heading: "System",
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
      where:
        "Backups are kept in {{directory}} on the server's machine. A backup on the same disk does not survive that disk: copy them elsewhere.",
      none: "There is no backup yet.",
      backup: "{{name}}, {{size}}, made {{at}}",
    },
    discovery: {
      heading: "Discovery",
      what: "A discovery tells your clients where to find this server, so it can be reached from outside the house without a VPS of your own in front of it. It passes on addresses, never your films.",
      address: "The discovery's address",
      connect: "Connect",
      at: "Connected to {{address}}.",
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
        "Times in the last week a client found no way to reach this server: {{count}}. The discovery's check of how it is reached says what may help.",
      heartbeat: "Last told the discovery its addresses at {{at}}.",
      problem: "The last call to the discovery failed: {{problem}}",
      clientOrigin: "Links to this server open at {{origin}}.",
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
          relayed: "through a relay: {{count}}",
          failed: "failed to connect: {{count}}",
        },
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
      x264: "x264, on the processor",
      copy: "copied, not encoded",
      unknown: "not recorded",
    },
    preparedUsage:
      "Prepared copies: {{used}} of a {{budget}} budget; the disk is {{percent}}% full, its limit {{limit}}%.",
    recentPrepares: "Recent prepared copies",
    noPrepares: "Nothing prepared since the copies' records were last cleared.",
    preparedFile: "File",
    preparedWith: "Video",
    realtimeFactor: "Speed",
    realtimeFactorTimes: "{{realtimeFactor}}× real time",
    finishedAt: "Finished",
    prepareFailed: "failed: {{why}}",
  },
  jobs: {
    heading: "Jobs",
    none: "No work has been done yet.",
    target: "{{kind}} #{{target}}",
    kind: {
      fetchMetadata: "Looking up",
      makeArtworkSizes: "Making artwork sizes",
      prepare: "Preparing a copy",
      extractSubtitle: "Extracting subtitles",
      readThrough: "Reading a video through",
      makeChunks: "Making a film's chunks",
      convertFetchedSubtitle: "Converting a downloaded subtitle",
      scan: "Listing a folder",
      examine: "Reading new files",
      backup: "Backing up",
      renewCertificate: "Renewing the server's certificate",
    },
    state: {
      queued: "Waiting",
      running: "Running",
      done: "Done",
      failed: "Failed",
      cancelled: "Cancelled",
    },
    progress: "How far {{job}} is",
    reason: "Why: {{reason}}",
    queuedAt: "Queued {{at}}",
    lastFailure: "Last error: {{failure}}",
    waitingFor: "Waiting for {{name}}: {{reason}} since {{since}}, next check {{next}}",
    waitingForSignal: "Waiting for {{name}}: {{reason}} since {{since}}",
    onNode: {
      waiting: "Waiting for a free slot on the media machine",
      paused: "Paused while someone watches",
    },
    cancel: "Cancel",
    cancelLabel: "Cancel {{job}}",
    alreadyEnded: "That job has already ended.",
  },
  sound: {
    question: "How does this device play sound?",
    why: "So that dialogue can be made clearer where it helps. You can change this later, under the video or in Your account.",
    builtInSpeakers: "Built-in speakers",
    builtInSpeakersHint:
      "The device's own speakers, a television's, a laptop's or a phone's: surround sound is mixed down to two channels here, and the dialogue boost can raise the voices.",
    receiver: "Soundbar or receiver",
    receiverHint:
      "Surround sound goes to it as it is when this browser can play the original file; a copy prepared for another reason is stereo, and the dialogue boost works on it.",
    headphones: "Headphones",
    headphonesHint:
      "Surround sound is mixed down to two channels here, and the dialogue boost can raise the voices.",
    current: "Sound: {{output}}",
    change: "Change",
    label: "Sound",
    notAsked: "Not asked yet",
  },
  continueWatching: {
    heading: "Continue watching",
    episode: "{{seriesName}} · S{{seasonNumber}}E{{episodeNumber}} · {{episodeName}}",
    watched: "{{percent}}% watched",
    hide: "Hide",
    hideLabel: "Hide {{name}} from Continue watching",
    hidden: "{{name}} is hidden until you watch it again.",
    undo: "Undo",
  },
  extraType: {
    trailer: "Trailer",
    teaser: "Teaser",
    featurette: "Featurette",
    behindTheScenes: "Behind the scenes",
    deletedScene: "Deleted scene",
    interview: "Interview",
    scene: "Scene",
    short: "Short",
    clip: "Clip",
    unknown: "Extra",
  },
  search: {
    heading: "Search",
    label: "Search for a film or a series",
    submit: "Search",
    none: "Nothing matches “{{query}}”.",
    kindMovie: "Film",
    kindSeries: "Series",
    kindEpisode: "Episode",
    kindExtra: "Extra",
  },
  files: {
    heading: "What the scanner saw",
    all: "Everything",
    group: "{{filing}} · {{reason}} ({{count}})",
    noReason: "no reason given",
    errorDetail: "Details: {{text}} (owner {{owner}}, group {{group}}, mode {{mode}})",
  },
  filing: {
    listed: "Listed",
    extra: "Extra",
    attached: "Beside a video",
    skipped: "Skipped",
    errored: "Could not be read",
    unidentified: "Not identified",
  },
  identify: {
    heading: "Needs a match",
    ambiguous: "Several matches",
    unidentified: "No match",
    none: "Nothing is waiting.",
    choose: "Choose its match",
    candidates: "Possible matches",
    pick: "This one",
    candidate: "{{name}} ({{year}})",
    originalName: "Originally “{{name}}”",
    refresh: "Look again",
    refreshed: "It will be looked up again shortly.",
    refreshedWaiting:
      "It will be looked up again once {{name}} answers: it has not been answering since {{since}}.",
    providerReason: {
      noKey: "No TMDB key is set up. An admin can set it in Settings.",
      keyRefused: "TMDB refused its key. An admin can change it in Settings.",
      unavailable: "TMDB is not answering right now. Try again later.",
      unavailableSince: "TMDB has not been answering{{since}}{{next}}.",
      unknown: "TMDB could not be reached.",
    },
  },
  settings: {
    heading: "Settings",
    secretsHeading: "Keys and passwords",
    secretsIntro: "What is stored here is never shown again, only whether it is set.",
    secretSet: "Set, ending in {{lastChars}}",
    secretSetNoLastChars: "Set",
    secretUnset: "Not set",
    secretReentry: "Must be entered again: this server no longer has the key it was sealed with.",
    newValue: "New value",
    save: "Save",
    remove: "Remove",
    removeConsequence: "The server can't use it until it's entered again.",
    removeConfirm: "Remove it",
    saved: "Saved.",
    removed: "Removed.",
    settingsHeading: "Server settings",
    changed: "Changed; the starting value is {{start}}",
    reset: "Reset to the starting value",
    on: "On",
  },
  shell: {
    notFound: "There is nothing at this address.",
  },
  common: {
    loading: "Loading…",
    show: "Show",
    showMore: "Show more",
    noPoster: "No poster",
    keepIt: "Keep it",
  },
} as const;

/** The catalogue's shape, which every other language's catalogue must have. */
export type Catalogue = typeof en;
