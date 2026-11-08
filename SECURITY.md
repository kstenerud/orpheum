# Security policy

Orpheum is in a closed beta. It handles accounts, passwords and passkeys, and it can be reached
from the internet, so a security problem matters even at this stage.

## Reporting a vulnerability

**Please do not open a public issue for a security problem.** Report it privately through
GitHub: on this repository's **Security** tab, choose **Report a vulnerability**. Only the
maintainer sees the report.

Please include what you found, how to reproduce it, and the version (the commit or the release
tag). A proof of concept helps; one that touches someone else's server does not.

You will get an answer within a week. Once the problem is fixed and released, it is disclosed
with credit to you, unless you would rather not be named.

## Supported versions

Only the latest release and `main` receive security fixes.

## Scope

In scope: the server and media node (`cmd/`, `internal/`), the web client and cast receiver
(`web/`), and the container images built from `deploy/`.

Out of scope: a deployment's own configuration (a reverse proxy, a firewall, a password chosen
weakly), and denial of service by sheer volume.
