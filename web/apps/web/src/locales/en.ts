// SPDX-License-Identifier: MPL-2.0

import { enCast } from "./en-cast.ts";
import { enDevices } from "./en-devices.ts";
import { enDiscovery } from "./en-discovery.ts";
import { enLibraries } from "./en-libraries.ts";
import { enPeople } from "./en-people.ts";
import { enPlay } from "./en-play.ts";
import { enSettingsCatalogue } from "./en-settings.ts";
import { enGallery } from "./en-gallery.ts";
import { enLab } from "./en-lab.ts";
import { enStatus } from "./en-status.ts";
import { enSystem } from "./en-system.ts";
import { enTogether } from "./en-together.ts";
import { enWarning } from "./en-warning.ts";
import { enWatched } from "./en-watched.ts";
import { enOrderings } from "./en-orderings.ts";
import { enNavigation } from "./en-navigation.ts";
import { enUpdate } from "./en-update.ts";

/**
 * The English catalogue, bundled as the offline floor (D51). Other languages are served by the
 * server once translations exist. Keys name what the text is for, never what it says.
 */
export const en = {
  ...enTogether,
  ...enPlay,
  ...enDevices,
  ...enCast,
  ...enDiscovery,
  ...enLibraries,
  ...enPeople,
  ...enStatus,
  ...enGallery,
  ...enWarning,
  ...enWatched,
  ...enOrderings,
  ...enLab,
  ...enNavigation,
  ...enSystem,
  ...enUpdate,
  waits: {
    underAMinute: "less than a minute",
    aMinute: "about a minute",
    minutes: "about {{count}} minutes",
    hours: "about {{count}} hours",
  },
  nav: {
    administerFromHome:
      "You administer this server from home, on its own page or the discovery's page at home.",
    administerFromHomeAt: "You administer this server from home, at",
    films: "Films",
    series: "Series",
    deviceTests: "Device tests",
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
    signOutAsked: "Sign out of this device?",
    signOutCancel: "Cancel",
    switchHousehold: "Switch household",
    manage: "Manage",
    develop: "Development",
    uiGallery: "UI gallery",
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
    leaving: "Signed out. Taking you to sign in…",
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
      "The server restarted, or ended this playback, so it stopped. Reload the page to carry on from where you were.",
    invalidRequest: "The server could not accept that. Check what was entered and try again.",
    invalidFields:
      "The server did not accept what was entered in {{fields, list}}. Correct what is there and try again.",
    conflict: "That clashes with something that already exists.",
    stale: "Someone changed this meanwhile. Reload and try again.",
    receivingACast: "That device is showing another cast. Cast to it once that cast ends.",
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
        "This cannot be played now: the server cannot reach {{node}}, the computer the film is on{{since}}. Check that it is on and connected.",
      rootMissing:
        "This cannot be played now: the folder {{path}} on {{node}} is missing{{since}}.",
      diskMissing:
        "This cannot be played now: what was mounted at {{path}} on {{node}} is missing{{since}}.",
      stillReading:
        "This is still being read: {{node}} is reading it now, and it plays as soon as that is done.",
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
    cannotConvert:
      "This file can't be converted for this device: it has no picture, or nothing to cut it into pieces by.",
    noRoomToMake:
      "The server cannot make this quality as fast as it plays while it makes others for other viewers. Try again in a while.",
    migrating: "The server is starting up, or updating. It will answer in a moment.",
    inAnotherSession: "You are in another Watch Together session. Leave it to join this one.",
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
  screenNotLoaded: {
    said: "This page could not load.",
    noWorker:
      "No service worker answers this page, and only it brings the server's pages here. Chrome's developer tools may be set to bypass the service worker: untick “Bypass for network” under Application → Service workers, then reload.",
    reload: "Reload",
  },
  whatHappened: {
    copy: "Copy what happened",
    copied: "Copied. Send it to whoever runs your server.",
    refused: "This browser wouldn't copy it: select the text below instead.",
    show: "What happened",
  },
  homeAddress: {
    heading: "Your server at home",
    text: "At home, this server is at",
    qr: "The server's home address, as a QR code",
  },
  account: {
    heading: "Your account",
    signedInAs: "Signed in as {{name}}",
    yourLink: "Your link",
    yourLinkText:
      "Open this link on another phone or computer, or scan the code with its camera, to find this server again. It signs no one in.",
    yourLinkQr: "Your link, as a QR code",
    adminAway: "Administering from away from home",
    adminAwayText:
      "Away from home you see this server as anyone else does. Allowed, anyone with your name and password can administer it from anywhere, so use a strong password.",
    adminAwayAlways: "Let me administer from away from home",
    adminAwayAtHome: "This can be changed only at home, on the server's own network.",
    deviceName: "The device's name",
    changePassword: "Change password",
    currentPassword: "Current password",
    newPassword: "New password",
    passwordChanged: "Your password is changed. Your other devices stay signed in.",
    wrongPassword: "That is not your current password. Check it and try again.",
    notOnThisDevice:
      "This device was activated by a code, so it can watch and host but not change your password, remove devices or activate others. Do that on a device you signed in to with your password.",
  },
  debugTools: {
    heading: "Testing",
    show: "Show debug tools",
    hint: "Shows extra controls and readouts for testing.",
    linkCap: "Cap this tab's download rate",
    linkCapOff: "No cap",
    linkCapRate: "{{rate}} Mb/s",
    drop: "Drop the connection for {{seconds}} s",
    dropped: "Connection dropped for testing: back in {{seconds}} s",
    throttled: "Throttled to {{rate}} Mb/s",
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
    entryWarning:
      "Only enter a code shown on a screen in front of you. A code someone sent you, in a message or on a call, lets their device into your account.",
    checkExplain: "{{device}} now shows a check code. This is the one it should show:",
    asked: "It says it is {{platform}}, and asked for its code {{age}}.",
    justNow: "just now",
    secondsAgo: "{{count}} s ago",
    minutesAgo: "{{count}} min ago",
    askedLongAgo:
      "That was a while ago. Unless you asked for this code on the device in front of you a while ago yourself, say they don't match.",
    checkCode: "Check code",
    warningHeading: "Look at the device's screen yourself.",
    warning:
      "Only say they match if you see this same code on the device's own screen, in front of you. Never go on what someone tells you, on a call or in a message: a code someone else gives you lets their device into your account.",
    match: "Yes, they match",
    noMatch: "They don't match",
    activated: "{{device}} is activated and signed in.",
    notActivated: "Nothing was activated.",
    scannedQuestion: "Activate {{device}}? It will be signed in as you.",
    scannedWarning:
      "Only activate a device you can see in front of you, whose QR code you scanned yourself. A QR code someone sent you lets their device into your account.",
    scannedActivate: "Activate",
    scannedCancel: "Cancel",
  },
  nowPlaying: {
    heading: "Now playing",
    none: "Nothing is playing.",
    who: "{{person}} on {{device}}: {{name}}, since {{since}}.",
    noDevice: "a device with no name",
    direct: "Played as it is.",
    converted: "Converted, because of: {{reasons}}.",
    message: "Message",
    stop: "Stop it",
  },
  share: {
    link: "It works once, until {{expires}}.",
    togetherLink: "It works once, for the person you made it for, while the session lasts.",
    copy: "Copy",
    copied: "Copied.",
    copyRefused: "This browser would not copy it; select the link above instead.",
  },
  about: {
    heading: "About",
    tmdbHeading: "Film and series information",
    tmdbLogo: "The Movie Database (TMDB)",
    tmdbNotice: "This product uses the TMDB API but is not endorsed or certified by TMDB.",
    thanksHeading: "Built on",
    thanksIntro: "{{product}} stands on the work of these projects, with thanks.",
  },
  home: {
    heading: "Home",
    newestFilms: "New films",
    newestSeries: "New series",
    noLibrariesAdmin: "Home fills once this server has a library of your films or series.",
    makeLibrary: "Make a library",
  },
  browse: {
    films: "All Films",
    series: "All Series",
    libraries: "Libraries",
    sort: "Sort",
    fieldName: "Name",
    fieldFileName: "File name",
    fieldAdded: "Added",
    fieldRecorded: "Recorded",
    fieldReleased: "Released",
    byNameAToZ: "Name, A–Z",
    byNameZToA: "Name, Z–A",
    byFileNameAToZ: "File name, A–Z",
    byFileNameZToA: "File name, Z–A",
    byAddedNewest: "Newest added",
    byAddedOldest: "Oldest added",
    byRecordedNewest: "Newest recorded",
    byRecordedOldest: "Oldest recorded",
    byReleasedNewest: "Newest released",
    byReleasedOldest: "Oldest released",
    alphabet: "Jump to a letter",
    empty: "Nothing here yet. Once a library is scanned, what it holds shows here.",
    noLibrary:
      "You have not been given any library yet, so there is nothing to watch. Ask whoever runs this server to share one with you.",
    noLibrariesAtAll: "No libraries have been made yet. Ask whoever runs this server to make one.",
    year: "{{year}}",
    library: "Library",
    allLibraries: "All Libraries",
    view: "View",
    viewAll: "All",
    viewFolders: "Folders",
    folderEmpty: "This folder holds no videos.",
    searchLabel: "Search the library chosen, or every one",
    searchSubmit: "Search",
    searchOpen: "Search",
    showAll: "Show all",
  },
  filingReport: {
    write: "Report this",
    savedAt:
      "Saved on the server as {{path}}. Edit anything you would rather not post, then post it as an issue.",
    text: "The report, in Markdown",
    copy: "Copy",
    copied: "Copied.",
    heading: "Reports of what was filed wrongly",
    what: "Each is a Markdown file on the server, written when an admin asked on a film's or series' page. Nothing is sent anywhere: read one, edit what you would rather not post, and post it as an issue.",
    none: "No report yet.",
  },
  title: {
    runtime: "{{minutes}} min",
    season: "Season {{season}}",
    seasons: "Seasons",
    seasonEpisodeCount: "episodes: {{count}}",
    placeBetween: " · ",
    placeSeason: "season {{season}}",
    placeEpisode: ", episode {{number}}",
    versions: "Versions",
    version: "Version {{index}}",
    fileDetails: "File details",
    extras: "Extras",
    unidentified: "Not identified yet",
    identifiedIncorrectly: "Identified incorrectly",
    identifiedCorrectly: "Identified correctly",
    identifiedCorrectlyExplain:
      "It was identified automatically, as the likeliest. Confirm it is right, and it stays so: no longer marked, and no later scan changes it.",
    confirmMatch: "Confirm",
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
    episodeCode: "S{{season}}E{{number}}",
    continueEpisode: "Continue {{episode}}",
    playEpisode: "Play {{episode}}",
    nextUpUnavailable: "{{episode}}: {{why}}",
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
  continueWatching: {
    heading: "Continue watching",
    episode: "{{seriesName}} · S{{seasonNumber}}E{{episodeNumber}} · {{episodeName}}",
    episodeInOrdering: "{{seriesName}} · {{place}} · {{episodeName}}",
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
    none: "Nothing matches “{{query}}”.",
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
    ambiguous: "Several matches ({{count}})",
    unidentified: "No match ({{count}})",
    none: "Nothing is waiting.",
    choose: "Choose its match",
    candidates: "Possible matches",
    pick: "This one",
    candidate: "{{name}} ({{year}})",
    originalName: "Originally “{{name}}”",
    refresh: "Look again",
    noneFound: "The search found nothing.",
    notListed: "Not here? Search for another name, or paste its page at TMDB or IMDb.",
    searchName: "Name",
    searchYear: "Year",
    search: "Search",
    enterManually: "Enter manually",
    link: "TMDB or IMDb page",
    linkHint: "https://www.themoviedb.org/… or https://www.imdb.com/title/tt…",
    useLink: "Use this match",
    linkRefused:
      "That is not a TMDB or IMDb page or id of this kind of work, or TMDB has no such work. Check what was pasted and try again.",
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
    secretSet: "Set, ending in {{lastChars}}",
    secretSetNoLastChars: "Set",
    secretUnset: "Not set",
    secretReentry: "Must be entered again: this server no longer has the key it was sealed with.",
    removeSecret: "Remove {{key}}",
    removeConsequence: "The server can't use it until it's entered again.",
    removeConfirm: "Remove it",
    advanced: "Show advanced settings",
    search: "Search",
    searchHint: "A key or words of its description",
    noneFound: "No setting matches.",
    about: "What {{key}} is for",
    reset: "Return {{key}} to its starting value, {{start}}",
    isStart: "{{key}} is at its starting value",
    unanswered: "unanswered",
    on: "on",
    off: "off",
    nothing: "nothing",
    tipAbout: "What it is for",
    tipReset: "Return to default value ({{start}})",
    tipAtDefault: "Already at default",
    tipResetting: "Returning to default",
    tipRemove: "Remove it",
    tipNotSet: "Not set, so nothing to remove",
    ...enSettingsCatalogue,
  },
  shell: {
    notFound: "There is nothing at this address.",
  },
  notice: {
    ok: "OK",
  },
  moreActions: {
    label: "More actions",
    close: "Close",
  },
  common: {
    loading: "Loading…",
    back: "Back",
    show: "Show",
    noPoster: "No poster",
    keepIt: "Keep it",
  },
} as const;

/** The catalogue's shape, which every other language's catalogue must have. */
export type Catalogue = typeof en;
