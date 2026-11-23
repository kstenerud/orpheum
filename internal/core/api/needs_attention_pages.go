// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/kstenerud/orpheum/internal/core/discovery"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/core/providers"
	"github.com/kstenerud/orpheum/internal/core/settings"
)

// AttentionPagesBody is what needs an admin by the page it belongs to and is fixed on, the table of
// D538 §12 (pages-spec "What counts as needing attention"): the menu marks each page that has any,
// the Needs attention page lists every item under its page, and each page's Needs you shows its
// own. A page with nothing is absent; a later server may add pages, which a client skips.
type AttentionPagesBody struct {
	// LibrarySetup: library folders and disks offline, scans waiting before removing files, what
	// scans could not read, rescans that gave up.
	LibrarySetup *AttentionPageBody `json:"librarySetup,omitempty"`
	// TitlesToCheck: films and series waiting for a match, series read wrongly, films without sound.
	TitlesToCheck *AttentionPageBody `json:"titlesToCheck,omitempty"`
	// System: a doctor's problem or check, work failing, a destination not answering, the
	// certificate not obtained, failures of unknown cause, a machine locked, outdated or silent.
	System *AttentionPageBody `json:"system,omitempty"`
	// Settings: an outside service refusing its key or missing it, a secret to enter again.
	Settings *AttentionPageBody `json:"settings,omitempty"`
	// Update: a newer version, a security fix this one lacks, the check failing, a pinned tag, a
	// newer compose file.
	Update *AttentionPageBody `json:"update,omitempty"`
	// Network: this server stood aside as a copy or a copy of it refused, the addresses expired,
	// another server enrolled in its place, dead ends in the last week, the protocol ending.
	Network *AttentionPageBody `json:"network,omitempty"`
}

// AttentionPageBody is one page's items: its problems first, then its notices.
type AttentionPageBody struct {
	IsProblem bool                `doc:"Whether any of its items is a problem, not only a notice" json:"isProblem"`
	Items     []AttentionItemBody `json:"items"                                                    maxItems:"100"`
	ItemCount int                 `doc:"How many items it has in all; items lists the first 100" json:"itemCount"`
}

// AttentionItemBody is one thing that needs an admin, with what its line says and links to: the
// fields its kind names, the rest absent.
type AttentionItemBody struct {
	// Kind is what it is; a later server may add kinds, which a client skips (D432 §2).
	Kind      string `enum:"folderOffline,diskOffline,folderWaiting,unread,rescanGaveUp,needsMatch,seriesReadWrongly,filmsWithoutSound,finding,workFailing,destinationDown,diskFull,certificateMissing,unknownFailures,machineLocked,machineOutdated,machineSilent,keyRefused,keyMissing,secretNeedsReentry,updateAvailable,securityFix,updateCheckFailed,composePinned,composeNewer,stoodAside,copyRefused,addressesExpired,removed,deadEnds,protocolEnding" json:"kind"` //nolint:lll // one struct tag
	IsProblem bool   `doc:"A problem, shown first and in the problem style; otherwise a notice" json:"isProblem"`
	// Count is how many: files waiting removal (folderWaiting), files and folders unread (unread),
	// titles (needsMatch, seriesReadWrongly, filmsWithoutSound), jobs failed (workFailing), kinds
	// of work alerting (unknownFailures), dead ends (deadEnds).
	Count int64 `json:"count,omitempty"`
	// Since is when it began, in Unix milliseconds: offline, waiting, given up, down, last heard,
	// stood aside, refused, expired, removed.
	Since int64 `json:"since,omitempty"`
	// LibraryID and LibraryName name its library; NodeName its machine; RootPath its library
	// folder; MountPath the disk inside it (diskOffline). A folder's anchor is its machine and path.
	LibraryID   int64  `json:"libraryId,omitempty"`
	LibraryName string `json:"libraryName,omitempty"`
	NodeName    string `json:"nodeName,omitempty"`
	RootPath    string `json:"rootPath,omitempty"`
	MountPath   string `json:"mountPath,omitempty"`
	// Finding is a doctor's finding not ok (finding): a fail is a problem, a warn a check.
	Finding *FindingBody `json:"finding,omitempty"`
	// Destination is the destination down (destinationDown), as the System page lists it.
	Destination *DestinationBody `json:"destination,omitempty"`
	// Prepared is the prepared directory of the machine whose disk is too full to prepare videos
	// (diskFull), as its last status said it; absent when it has not said.
	Prepared *PreparedBody `json:"prepared,omitempty"`
	// Name is the outside service (keyRefused, keyMissing), the secret (secretNeedsReentry), the
	// public name (certificateMissing), or where a copy tried to start (copyRefused).
	Name string `json:"name,omitempty"`
	// Version is the newest release (updateAvailable), the first with the fix (securityFix), or
	// the tag pinned (composePinned).
	Version string `json:"version,omitempty"`
	// Detail is what went wrong, for the admin: the fix lacking (securityFix), the reading's
	// problem (updateCheckFailed), why a scan waits (folderWaiting).
	Detail string `json:"detail,omitempty"`
	// WindowHours is how far back Count looks (workFailing): jobHealthWindowHours.
	WindowHours int64 `json:"windowHours,omitempty"`
	// Until is the date the discovery stops answering this protocol (protocolEnding).
	Until string `json:"until,omitempty"`
}

// The kinds of attention items, as AttentionItemBody's enum says them.
const (
	attentionFolderOffline      = "folderOffline"
	attentionDiskOffline        = "diskOffline"
	attentionFolderWaiting      = "folderWaiting"
	attentionUnread             = "unread"
	attentionRescanGaveUp       = "rescanGaveUp"
	attentionNeedsMatch         = "needsMatch"
	attentionSeriesReadWrongly  = "seriesReadWrongly"
	attentionFilmsWithoutSound  = "filmsWithoutSound"
	attentionFinding            = "finding"
	attentionWorkFailing        = "workFailing"
	attentionDestinationDown    = "destinationDown"
	attentionDiskFull           = "diskFull"
	attentionCertificateMissing = "certificateMissing"
	attentionUnknownFailures    = "unknownFailures"
	attentionMachineLocked      = "machineLocked"
	attentionMachineOutdated    = "machineOutdated"
	attentionMachineSilent      = "machineSilent"
	attentionKeyRefused         = "keyRefused"
	attentionKeyMissing         = "keyMissing"
	attentionSecretNeedsReentry = "secretNeedsReentry"
	attentionUpdateAvailable    = "updateAvailable"
	attentionSecurityFix        = "securityFix"
	attentionUpdateCheckFailed  = "updateCheckFailed"
	attentionComposePinned      = "composePinned"
	attentionComposeNewer       = "composeNewer"
	attentionStoodAside         = "stoodAside"
	attentionCopyRefused        = "copyRefused"
	attentionAddressesExpired   = "addressesExpired"
	attentionRemoved            = "removed"
	attentionDeadEnds           = "deadEnds"
	attentionProtocolEnding     = "protocolEnding"
)

// attentionPage is one page's items as they are gathered.
type attentionPage []AttentionItemBody

// body is the page as the API says it, its problems first, or nil when it has no item.
func (page attentionPage) body() *AttentionPageBody {
	if len(page) == 0 {
		return nil
	}
	items := slices.Clone(page)
	slices.SortStableFunc(items, func(a, b AttentionItemBody) int {
		return cmp.Compare(boolRank(b.IsProblem), boolRank(a.IsProblem))
	})
	return &AttentionPageBody{
		IsProblem: items[0].IsProblem, Items: items[:min(len(items), attentionListedAtMost)], ItemCount: len(items),
	}
}

// boolRank is 1 for true, so a sort puts the trues first by comparing b with a.
func boolRank(is bool) int {
	if is {
		return 1
	}
	return 0
}

// attentionPages is what needs an admin by page (D538 §12), from the counts body already holds and
// the reads the System, Update and Network pages make. It reads more than the tray's counts, so a
// part that cannot be read leaves the pages out, logged, and the counts still answer as before.
func (server *Server) attentionPages(
	ctx context.Context, dependencies *Dependencies, body *NeedsAttentionBody, places attentionPlaces,
) *AttentionPagesBody {
	pages := &AttentionPagesBody{}
	pages.LibrarySetup = libraryAttention(body, places).body()
	pages.TitlesToCheck = titlesAttention(body).body()
	system, err := server.systemAttention(ctx, dependencies, body)
	pages.System = system.body()
	if err == nil {
		var keys attentionPage
		keys, err = settingsAttention(ctx, dependencies)
		pages.Settings = keys.body()
	}
	if err != nil {
		server.logger.WarnContext(ctx, "what needs an admin by page was left out; the counts still answer",
			"err", err)
		return nil
	}
	pages.Update = updateAttention(dependencies).body()
	if dependencies.Discovery != nil {
		pages.Network = networkAttention(dependencies.Discovery.Status()).body()
	}
	return pages
}

// attentionPlaces are the library places the tray's counts were read from, listed again by page.
type attentionPlaces struct {
	offline []library.Offline
	waiting []library.WaitingRemovals
	unread  []library.UnreadPath
}

// libraryAttention is Library setup's items: each folder and disk offline, each folder whose scan
// waits, what could not be read by library, and each rescan given up.
func libraryAttention(body *NeedsAttentionBody, places attentionPlaces) attentionPage {
	var page attentionPage
	for _, missing := range places.offline {
		kind := attentionFolderOffline
		if missing.MountPath != "" {
			kind = attentionDiskOffline
		}
		page = append(page, AttentionItemBody{
			Kind: kind, IsProblem: true, Since: missing.Since.UnixMilli(), LibraryID: missing.LibraryID,
			LibraryName: missing.LibraryName, NodeName: missing.NodeName, RootPath: missing.RootPath,
			MountPath: missing.MountPath,
		})
	}
	for _, root := range places.waiting {
		page = append(page, AttentionItemBody{
			Kind: attentionFolderWaiting, IsProblem: true, Count: int64(root.FileCount),
			Since: root.Since.UnixMilli(), LibraryID: root.LibraryID, LibraryName: root.LibraryName,
			NodeName: root.NodeName, RootPath: root.RootPath, Detail: root.Reason,
		})
	}
	page = append(page, unreadByLibrary(places.unread)...)
	for _, given := range body.RescansGivenUp {
		page = append(page, AttentionItemBody{
			Kind: attentionRescanGaveUp, IsProblem: true, Since: given.GivenUpAt, LibraryID: given.LibraryID,
			LibraryName: given.Name,
		})
	}
	return page
}

// unreadByLibrary is one item a library and machine for what scans could not read, counted: the
// Files page lists them one by one.
func unreadByLibrary(unread []library.UnreadPath) attentionPage {
	type place struct {
		libraryID int64
		nodeName  string
	}
	var page attentionPage
	found := map[place]int{}
	for _, path := range unread {
		at := place{path.LibraryID, path.NodeName}
		index, isFound := found[at]
		if !isFound {
			index = len(page)
			found[at] = index
			page = append(page, AttentionItemBody{
				Kind: attentionUnread, IsProblem: true, Since: path.Since.UnixMilli(), LibraryID: path.LibraryID,
				LibraryName: path.LibraryName, NodeName: path.NodeName,
			})
		}
		page[index].Count++
		page[index].Since = min(page[index].Since, path.Since.UnixMilli())
	}
	return page
}

// titlesAttention is Titles to check's items, counted as the tray counts them: notices, each title
// still playing as it is.
func titlesAttention(body *NeedsAttentionBody) attentionPage {
	var page attentionPage
	for _, counted := range []struct {
		kind  string
		count int
	}{
		{attentionNeedsMatch, body.MatchCount},
		{attentionSeriesReadWrongly, body.SeriesReadWronglyCount},
		{attentionFilmsWithoutSound, body.FilmsWithoutSoundCount},
	} {
		if counted.count > 0 {
			page = append(page, AttentionItemBody{Kind: counted.kind, Count: int64(counted.count)})
		}
	}
	return page
}

// systemAttention is System's items: the doctor's findings not ok, the jobs failed within the job
// health window, each destination down, each machine whose disk is too full to prepare videos, the
// certificate not obtained, the failures of unknown cause
// alerting, and each machine locked, outdated or silent.
func (server *Server) systemAttention(
	ctx context.Context, dependencies *Dependencies, body *NeedsAttentionBody,
) (attentionPage, error) {
	var page attentionPage
	findings, err := server.collectFindings(ctx, dependencies)
	if err != nil {
		return nil, fmt.Errorf("collect the doctor's findings: %w", err)
	}
	for _, finding := range findings {
		if finding.Verdict != VerdictOK {
			page = append(page, AttentionItemBody{
				Kind: attentionFinding, IsProblem: finding.Verdict == VerdictFailure,
				Finding: &finding,
			})
		}
	}
	window, err := dependencies.Settings.Duration(ctx, settings.JobHealthWindowHours)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", settings.JobHealthWindowHours, err)
	}
	health, err := dependencies.Jobs.Health(ctx)
	if err != nil {
		return nil, fmt.Errorf("read the queue's health: %w", err)
	}
	var failed int64
	for _, kind := range health {
		failed += kind.FailedCount
	}
	if failed > 0 {
		page = append(page, AttentionItemBody{
			Kind: attentionWorkFailing, IsProblem: true, Count: failed, WindowHours: int64(window / time.Hour),
		})
	}
	nodeName := server.nodeNamer(ctx, dependencies)
	for _, state := range dependencies.Jobs.ParkedDestinations() {
		if parts := jobs.PartsOf(state.Destination); parts.IsPreparedDirectory {
			page = append(page, diskFullItem(dependencies, nodeName(parts.NodeID), state))
			continue
		}
		down := destinationBody(state, nodeName)
		page = append(page, AttentionItemBody{
			Kind: attentionDestinationDown, IsProblem: true, Since: down.DownAt,
			Destination: &down,
		})
	}
	if system := dependencies.System; system.PublicName != "" && system.Certificate != nil {
		if _, isObtained := system.Certificate.ExpiresAt(); !isObtained {
			page = append(page, AttentionItemBody{
				Kind: attentionCertificateMissing, IsProblem: true, Name: system.PublicName,
			})
		}
	}
	if body.UnknownFailureAlertCount > 0 {
		page = append(page, AttentionItemBody{
			Kind: attentionUnknownFailures, IsProblem: true, Count: int64(body.UnknownFailureAlertCount),
		})
	}
	machines, err := machinesAttention(ctx, dependencies)
	if err != nil {
		return nil, err
	}
	return append(page, machines...), nil
}

// machineAttentionKind is the item a machine's connection makes, and false for one that needs no
// admin: connected, or waiting for the hello it sends on connecting.
func machineAttentionKind(connection nodes.Connection) (string, bool) {
	switch connection {
	case nodes.Locked:
		return attentionMachineLocked, true
	case nodes.NeedsUpdate:
		return attentionMachineOutdated, true
	case nodes.Silent:
		return attentionMachineSilent, true
	case nodes.Connected, nodes.AwaitingHello:
	}
	return "", false
}

// machinesAttention is an item for each media machine locked, outdated or silent, since it was last
// heard from.
func machinesAttention(ctx context.Context, dependencies *Dependencies) (attentionPage, error) {
	listed, err := dependencies.Libraries.NodeRoots(ctx)
	if err != nil {
		return nil, fmt.Errorf("list the nodes: %w", err)
	}
	var page attentionPage
	for _, node := range listed {
		connection, lastHeard := dependencies.NodeReports.NodeConnection(ctx, node.ID)
		kind, isAttention := machineAttentionKind(connection)
		if !isAttention {
			continue
		}
		item := AttentionItemBody{Kind: kind, IsProblem: true, NodeName: node.Name}
		if !lastHeard.IsZero() {
			item.Since = lastHeard.UnixMilli()
		}
		page = append(page, item)
	}
	return page, nil
}

// settingsAttention is Keys and passwords' items: each outside service whose key was refused, or
// missing when a request needed it, and each secret sealed under a key this server no longer has.
func settingsAttention(ctx context.Context, dependencies *Dependencies) (attentionPage, error) {
	var page attentionPage
	for _, provider := range dependencies.Providers {
		state := provider.State(ctx)
		switch state.Credentials {
		case providers.CredentialsRefused:
			page = append(page, AttentionItemBody{Kind: attentionKeyRefused, IsProblem: true, Name: state.Name})
		case providers.CredentialsMissing:
			page = append(page, AttentionItemBody{Kind: attentionKeyMissing, IsProblem: true, Name: state.Name})
		case providers.CredentialsUnknown, providers.CredentialsAccepted:
		}
	}
	if dependencies.Secrets == nil {
		return page, nil
	}
	statuses, err := dependencies.Secrets.Statuses(ctx)
	if err != nil {
		return nil, fmt.Errorf("list the secrets: %w", err)
	}
	for _, status := range statuses {
		if status.NeedsReentry {
			page = append(page, AttentionItemBody{
				Kind: attentionSecretNeedsReentry, IsProblem: true, Name: saidValue(string(status.Name)),
			})
		}
	}
	return page, nil
}

// updateAttention is Update's items, from what the Update page reads: a newer version, a notice; the
// security fix this version lacks and the last check failing, problems; a pinned tag and a newer
// compose file, notices.
func updateAttention(dependencies *Dependencies) attentionPage {
	update := knownUpdateOutput(dependencies).Body
	var page attentionPage
	if update.Latest != "" {
		page = append(page, AttentionItemBody{Kind: attentionUpdateAvailable, Version: update.Latest})
	}
	if fix := update.Security; fix != nil {
		page = append(page, AttentionItemBody{
			Kind: attentionSecurityFix, IsProblem: true, Version: fix.FixedIn, Detail: fix.Summary,
		})
	}
	if update.Problem != "" || update.StaleAt != 0 {
		page = append(page, AttentionItemBody{
			Kind: attentionUpdateCheckFailed, IsProblem: true, Since: update.StaleAt, Detail: update.Problem,
		})
	}
	if update.PinnedTag != "" {
		page = append(page, AttentionItemBody{Kind: attentionComposePinned, Version: update.PinnedTag})
	}
	if update.ComposeFile != nil {
		page = append(page, AttentionItemBody{Kind: attentionComposeNewer})
	}
	return page
}

// networkAttention is Network's items, from the discovery's status as the Network page reads it.
func networkAttention(status discovery.Status) attentionPage {
	standing := discoveryStatusBody(status)
	var page attentionPage
	if standing.StoodAsideAt != 0 {
		page = append(page, AttentionItemBody{Kind: attentionStoodAside, IsProblem: true, Since: standing.StoodAsideAt})
	}
	if standing.CopyRefusedAt != 0 {
		page = append(page, AttentionItemBody{
			Kind: attentionCopyRefused, Since: standing.CopyRefusedAt, Name: standing.CopyRefusedFrom,
		})
	}
	if !status.DocumentStaleSince.IsZero() {
		page = append(page, AttentionItemBody{
			Kind: attentionAddressesExpired, IsProblem: true, Since: status.DocumentStaleSince.UnixMilli(),
		})
	}
	if !status.RemovedAt.IsZero() {
		page = append(page, AttentionItemBody{
			Kind: attentionRemoved, IsProblem: true, Since: status.RemovedAt.UnixMilli(),
		})
	}
	if status.DeadEndsLastWeek > 0 {
		page = append(page, AttentionItemBody{Kind: attentionDeadEnds, Count: int64(status.DeadEndsLastWeek)})
	}
	if status.DeprecatedUntil != "" {
		page = append(page, AttentionItemBody{Kind: attentionProtocolEnding, Until: status.DeprecatedUntil})
	}
	return page
}

// diskFullItem is a machine whose prepared directory is parked for want of room (D543): a node
// refused a run for its disk, and it clears when the machine's status shows room again, which
// unparks it (nodes.keepStatus). Its usage is the machine's last status, when it has said.
func diskFullItem(dependencies *Dependencies, nodeName string, state jobs.DestinationState) AttentionItemBody {
	item := AttentionItemBody{
		Kind: attentionDiskFull, IsProblem: true, Since: state.Since.UnixMilli(), NodeName: nodeName,
	}
	if facts, found := dependencies.NodeReports.Facts(nodeName); found {
		item.Prepared = preparedBody(facts.Status.PreparedUsage)
	}
	return item
}
