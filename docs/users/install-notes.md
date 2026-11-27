# Watching an install: what to note

For whoever sits beside a person installing Orpheum for the first time, and for an AI agent helping
one. An install by someone who has never seen the system shows what its makers cannot see: a step
whose words assume knowledge, a setting their own machines never had, a moment where the person
does not know what comes next. This sheet is how those reach the project.

## If you are watching

- **Do not help.** Give the person the invitation and the [install guide](install.md), nothing
  else. When they are stuck, wait. Step in only when they would otherwise give up, and note the
  moment you did: it is the most useful line on the sheet.
- **Note what they do, not what they should have done.** Where they look, what they read twice,
  what they search for, what they skip.
- **Keep the time.** When each stage below starts and ends, to the minute.

## If you are an AI agent

Follow [the agents' page](agent-setup.md); its rules stand. Keep these notes as you go, from what
the person tells you and what you see. At the end, show the person the filled sheet and ask whether
they will send it. It is theirs to send or not.

## The machine

- Make and model, and what runs on it (OpenMediaVault and its version, DSM and its version, another
  system); `uname -m`'s answer; how much memory.
- Docker: already there, or installed for this; its version.
- Where the films are: the same machine, or another NAS, and how it is mounted. Where the server's
  data went.
- The home's connection, as far as the person knows it: the provider, and whether the installer
  said yes to a public IPv4 and a public IPv6 address.

## The stages

For each: the time it took, where the person hesitated, what they asked or searched for, what went
wrong, and what fixed it.

1. **Before starting**: finding the guide, choosing the route for their machine, the checks under
   "Before you start".
2. **Installing**: Docker where it was missing, the compose file or the install script, the folders
   and variables, the first **Up**.
3. **The first admin**: opening the server from a browser at home, making the account.
4. **The first library**: adding it, the scan, the first posters arriving.
5. **The first film at home**: on which device and browser; how long from pressing play to a
   picture; whether the sound and subtitles were as expected.
6. **The account**: opening the invitation, making the account, the confirmation mail and how long
   it took to arrive (and whether it went to spam).
7. **Linking the server** to the account, from the server's Remote access page.
8. **The first film away from home**: a phone on mobile data, not the home's Wi-Fi; how long to
   connect; whether it played.
9. **Later, an update**: how they learned of it, what they did, whether everything came back.

## For each hesitation or fault

- The step: the guide's heading, or the page's name and the button.
- The words on the screen, exactly, or a screenshot. An error's whole text.
- What the person expected to happen, in their words.
- What they tried, and what worked in the end.
- Whether [setup-problems.md](setup-problems.md) had it, and whether they found it there alone.

## What to leave out

- Passwords, an invitation link, a link from **Invite someone**, a recovery code, any key or token.
- The titles of the films and series, the names of people in the household, and the home's
  addresses. "A film" and "the second account" are enough.
- A problem report made with `unsafe`: it names everything. One made by the System page, or with
  `safe` from a terminal ([how](setup-problems.md#asked-for-a-problem-report-and-the-core-does-not-start-so-system-cannot-make-one)),
  replaces each title, path and name by a tag and may go with the sheet. Read it first.

## Where it goes

During the closed beta, to the person who sent the invitation. Add the server's version (the System
page shows it) and, where something failed, a problem report from **Report a problem** in the menu.
