# Deployment for the least technical user (the owner, 2026-11-08)

The owner asked for an independent audit of how a server is installed and how its parts find each
other, aiming at *"as little technical skill as possible (without sacrificing security)"*. The audit
is [deployment-audit-2026-11-08.md](../research/deployment-audit-2026-11-08.md). The owner adopted
its proposals (*"I like all of those proposals. Let's get everything in place and also test the most
fundamental parts such that the happy path at least works."*): D448.

## What the audit found stops people, worst first

1. Away access fails behind carrier or symmetric NAT, with no relay (D314: failures counted).
2. The address at home must be typed, and kept by a router reservation; when it moves, admin is
   locked out silently.
3. Remote access is a chain: an account, a realm, a key pasted into the server, each person signing
   in once at home.
4. Media paths and permissions; a second folder means editing the compose file.
5. The setup code is only in the container's log.
6. The node will not start without Landlock (Synology, older kernels).

## The work (D448)

The owner (2026-11-08): *"lay the groundwork for everything, get the easy stuff done today (enough to
have something to show potential employers), and then we do the heavy stuff over the next few
days"*.

| Item | D448 § | Built as | When |
| --- | --- | --- | --- |
| The settings applied at every start | 2 | D449 | first: 2026-11-08 |
| The server's own machine's port in the compose file | 3 | D449 | first |
| The setup code in a file, the first-run page saying where | 4 | D450 | first |
| More media folders by variable, host names shown | 5 | D450 | first |
| No Landlock: the node locked until an admin accepts, in the UI | 6 | D450 | first |
| A release channel and an update notice | 7 | D450 | first |
| Catalogue entries (submission is the owner's) | 8 | D450 | first |
| The address at home learned at setup, learned again with a fresh code | 1 | D449 | then: the next days |
| Remote access approved from the account (both repositories) | 9 | D451 | then |

**Not built, recorded:** the household's relay, for the owner's own discovery and the paid one
(D448 §10, D438); remote nodes and joining one by code (D448 §11).

## The happy path, tested at each stage's end

First stage, on Docker from the image built from this branch: paste `deploy/omv/compose.yaml` with
its folders and address set, start it, read the setup code from the data folder, make the admin from
another machine on the network, see the node join and a library scan
its folder; restart and see the settings re-applied. After D449's address learning, the same with no address
typed anywhere; after D451, connect the server to staging's discovery
by approval.

## Owed by the owner

- Submitting the catalogue entries; making the image's package public; the arm64 image (a gate).
- Deploying the paid service's side of D451 to production.
