# Catalogue entries

Orpheum's compose file packaged for the app catalogues of NAS and Docker panels (D449 §5). Submitting
each is the owner's; nothing here is published by itself. Every route's file, the catalogues' among
them, is generated from `internal/deployment/templates/household.yaml` by `make generate` (D462
§10.2), and `TestCatalogue_…` in `internal/deployment/catalogue_test.go` fails when an entry falls out
of step with its file's variables.

No entry asks for the machine's address at home, its HTTPS port or the router: the server learns the
address, and its System page sets the rest (D452 §5, D462 §2).

## Portainer: `portainer/templates.json`

An app template list in Portainer's format version 3, with one template of type 3 (a Compose stack)
that installs `deploy/portainer/compose.yaml` from the repository and asks for each of its variables.
Portainer's template names no branch, so it takes the file from the repository's default branch,
`main`, which may be ahead of the newest release: a flag that release does not take is skipped, said
in the log (D462 §10.10).

- **Use it now:** in Portainer, **Settings > App Templates > URL**, give the raw URL of this file
  (`https://raw.githubusercontent.com/kstenerud/orpheum/release/deploy/catalogue/portainer/templates.json`).
- **Submit:** Portainer's own list (`portainer/templates`) takes few community apps; the usual route
  is a community template list, such as Lissy93's `portainer-templates`, by a pull request adding the
  template to its sources, following that repository's contributing notes.

## CasaOS and ZimaOS: `casaos/docker-compose.yml`

The compose file with CasaOS's folders and user (`/DATA/AppData`, `/DATA/Media`, `$PUID`, `$PGID`,
`$TZ`) and the store's `x-casaos` block (title, icon, the core as the app's page on port 8440, and a
tip saying where the setup code is).

- **Use it now:** in CasaOS, **App Store > Custom Install > Import**, paste the file.
- **Submit:** a pull request to `IceWhaleTech/CasaOS-AppStore` adding `Apps/Orpheum/` with this file
  as `docker-compose.yml`, an icon, and screenshots, as its `CONTRIBUTING.md` asks. Its reviewers
  expect an app to install with no value typed, which it now does.

## OpenMediaVault

The compose plugin's **Add from example** lists examples the plugin's maintainers keep
(`OpenMediaVault-Plugin-Developers/openmediavault-compose` and omv-extras' wiki). `deploy/omv/compose.yaml`
already follows their conventions (`PATH_TO_APPDATA`, `APPUSER_PUID`, `APPUSER_PGID`,
`TIME_ZONE_VALUE`), so it is offered as it is: ask on the omv-extras forum, or by an issue on that
repository, how they take a new example.

## TrueNAS

Not built. TrueNAS SCALE's community apps (`truenas/apps`, under `ix-dev/community/`) are not compose
files: an app is an `app.yaml`, a `questions.yaml` form and Jinja templates in the project's own
library. Worth writing now that no address is asked: the form would ask only for folders.
