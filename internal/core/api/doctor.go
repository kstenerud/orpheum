// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/kstenerud/orpheum/internal/core/rtcapi"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/diskspace"
)

// Verdict is a doctor finding's weight, the one vocabulary of the running server's doctor and the
// command's (architecture map F7).
type Verdict string

// The verdicts a finding gives.
const (
	VerdictOK      Verdict = "ok"
	VerdictWarning Verdict = "warn"
	VerdictFailure Verdict = "fail"
)

// FindingBody is one thing the running server checked, and for anything wrong, what to do.
type FindingBody struct {
	Check   string  `json:"check"`
	Verdict Verdict `enum:"ok,warn,fail" json:"verdict"`
	Detail  string  `json:"detail"`
	Action  string  `json:"action,omitempty"`
}

// collectFindings lists what the running server finds wrong with itself, for the System page.
func (server *Server) collectFindings(ctx context.Context, dependencies *Dependencies) ([]FindingBody, error) {
	findings := []FindingBody{artworkFinding(dependencies)}
	if server.rtc != nil {
		findings = append(findings, hairpinFinding(server.rtc.HairpinWarning()))
	}
	listed, err := dependencies.Libraries.NodeRoots(ctx)
	if err != nil {
		return nil, server.libraryError(ctx, "list nodes", err)
	}
	placesFindings, err := server.placesFindings(ctx, dependencies)
	if err != nil {
		return nil, err
	}
	findings = append(findings, placesFindings...)
	for _, node := range listed {
		facts, found := dependencies.NodeReports.Facts(node.Name)
		if !found {
			continue
		}
		findings = append(findings, nodeBuildFinding(node.Name, facts.Hello.Build, dependencies.System.Build),
			ConfinementFinding("node "+node.Name, facts.Hello.Confinement),
			hardwareFinding(node.Name, facts.Hello.Hardware), shareFinding(node.Name, facts.Hello.Devices),
			filmReadingFinding(node.Name, facts.Hello.Hardware))
		if facts.Status != nil && facts.Status.PreparedUsage != nil {
			findings = append(findings, preparedFinding(node.Name, *facts.Status.PreparedUsage))
		}
	}
	return findings, nil
}

// shareFinding is whether a node reads a network share that waits for its server for good: when the
// server goes away, every read of it hangs in the kernel and cannot be ended, and the node holds
// one such read at most (D233 §10; staging, 2026-10-21). A warning for a share mounted so, with how
// to mount it to give up; a note for a filesystem passed on from elsewhere, whose mounts the node
// cannot see.
func shareFinding(nodeName string, devices []nodeproto.Device) FindingBody {
	finding := FindingBody{
		Check: "network shares on node " + nodeName, Verdict: VerdictOK, Detail: "none waits for its server for good",
	}
	var waiting, passedOn []string
	for _, device := range devices {
		switch device.WhenGone {
		case nodeproto.ShareWaitsForever:
			waiting = append(waiting, strings.Join(device.MountPaths, ", "))
		case nodeproto.ShareMayWaitForever:
			passedOn = append(passedOn, strings.Join(device.MountPaths, ", ")+" ("+device.Filesystem+")")
		}
	}
	switch {
	case len(waiting) > 0:
		finding.Verdict = VerdictWarning
		finding.Detail = "mounted to wait for its server for good: " + strings.Join(waiting, "; ") +
			". If the server goes away, every read of it hangs until it is back, and its films cannot play"
		finding.Action = "mount the share with soft,timeo=100,retrans=3 (NFS) so its reads give up after " +
			"about 30 seconds, and restart the node"
	case len(passedOn) > 0:
		finding.Detail = "passed on from another machine or program, whose mount the node cannot see: " +
			strings.Join(passedOn, "; ") + ". If that is a network share mounted to wait for its server, mount " +
			"it with soft,timeo=100,retrans=3 (NFS) there"
	}
	return finding
}

// filmReadingFinding is whether a node's FFmpeg reads each film through the node (D330): a warning
// for one that reads films by their paths, as the stock FFmpeg build does, which loses the
// dead-share protection, a run's read of a share whose server went away hanging in the kernel where
// the node cannot end it (audit 2026-10-25 N9).
func filmReadingFinding(nodeName string, hardware nodeproto.Hardware) FindingBody {
	finding := FindingBody{
		Check: "how FFmpeg reads films on node " + nodeName, Verdict: VerdictOK,
		Detail: "each run reads its film through the node (fdproxy), which a share that goes away cannot hang",
	}
	if !hardware.ReadsFilmsThroughNode {
		finding.Verdict = VerdictWarning
		finding.Detail = "this node's FFmpeg has no fdproxy protocol, so each run reads its film by its path: " +
			"if a network share's server goes away, a run reading it can hang where the node cannot end it"
		finding.Action = "run the node with Orpheum's own FFmpeg build, which has the fdproxy protocol, " +
			"and restart it"
	}
	return finding
}

// preparedFinding is what a node's prepared directory holds against its budget and its disk's
// limit (D217 §5): a warning when either is past, which eviction could not bring back.
func preparedFinding(nodeName string, usage nodeproto.PreparedUsage) FindingBody {
	finding := FindingBody{
		Check: "prepared copies on node " + nodeName, Verdict: VerdictOK,
		Detail: fmt.Sprintf("%.1f GiB of a %d GiB budget; the disk is %d%% full, its limit %d%%, with %d MiB "+
			"free, its floor %d MiB",
			float64(usage.UsedBytes)/diskspace.BytesPerGiB, usage.BudgetBytes/diskspace.BytesPerGiB,
			usage.DiskUsedPercent, usage.DiskUsedLimitPercent, usage.DiskAvailableBytes>>20,
			usage.DiskMinFreeBytes>>20),
	}
	if usage.DiskUsedPercent > usage.DiskUsedLimitPercent || usage.UsedBytes > usage.BudgetBytes ||
		usage.DiskAvailableBytes < usage.DiskMinFreeBytes {
		finding.Verdict = VerdictWarning
		finding.Action = "free space on the disk that holds the prepared directory, or raise " +
			"node.prepared_budget_gib; new copies are refused while nothing can be evicted"
	}
	return finding
}

// ConfinementFinding is FFmpeg's confinement where it runs, judged from the mechanisms a node
// reports, or would report, in its hello: a warning naming each mechanism not in force, with what
// would put it there, because a user whose safety depends on it deserves to know (D112). A report
// with no mechanism in it is a warning too. It is the one judgement of confinement: the System page
// uses it for each node, and the doctor command for this machine. The place,
// where, reads "node devnas" or "this machine".
func ConfinementFinding(where string, confinement []nodeproto.Confinement) FindingBody {
	finding := FindingBody{
		Check: "FFmpeg confinement on " + where, Verdict: VerdictOK,
		Detail: "every FFmpeg run is confined by Landlock and seccomp",
	}
	var missing, remedies []string
	for _, mechanism := range confinement {
		if !mechanism.IsInForce {
			missing = append(missing, mechanism.Mechanism+": "+mechanism.Reason)
			remedies = append(remedies, mechanism.Mechanism+": "+mechanism.Remedy)
		}
	}
	if len(confinement) == 0 {
		missing = []string{"no confinement was reported"}
	}
	if len(missing) > 0 {
		finding.Verdict = VerdictWarning
		finding.Detail = "FFmpeg runs without all its confinement: " + strings.Join(missing, "; ")
		finding.Action = strings.Join(remedies, "; ")
	}
	return finding
}

// hairpinFinding is whether devices reach the server through the NAT they share with it and back,
// not across the house's network (D354 §2): a warning naming the UDP port and the house's address a
// device in the house could not reach, while hairpin, isStanding, stands.
func hairpinFinding(hairpin rtcapi.Hairpin, isStanding bool) FindingBody {
	finding := FindingBody{
		Check: "WebRTC from devices at home", Verdict: VerdictOK,
		Detail: "no device has reached this server through your provider and back since it started, " +
			"or since the last time one at home reached it directly",
	}
	if !isStanding {
		return finding
	}
	finding.Verdict = VerdictWarning
	seen := fmt.Sprintf("%d connection(s) from %s, most recently %s, went out to the provider and back, "+
		"through the address this server shares with them", hairpin.Count, hairpin.Client,
		hairpin.LastAt.UTC().Format(time.RFC3339))
	if hairpin.UDPPort == 0 || len(hairpin.LANAddresses) == 0 {
		finding.Detail = "devices at home may reach this server through your provider, not your network: " +
			"it takes WebRTC on no port at the house's address. " + seen
		finding.Action = "set webrtc.udp_port and discovery.lan_addresses, publish that UDP port at the " +
			"house's address, and restart the server"
		return finding
	}
	addresses := make([]string, 0, len(hairpin.LANAddresses))
	for _, address := range hairpin.LANAddresses {
		addresses = append(addresses, address.String())
	}
	finding.Detail = fmt.Sprintf("devices at home reach this server through your provider, not your network: "+
		"UDP port %d at %s is not reachable. %s", hairpin.UDPPort, strings.Join(addresses, ", "), seen)
	finding.Action = fmt.Sprintf("publish UDP port %d from the server's machine (Docker: -p %d:%d/udp) and let "+
		"it through its firewall; this clears once a device at home reaches the server directly, or a week "+
		"after the last such connection", hairpin.UDPPort, hairpin.UDPPort, hairpin.UDPPort)
	return finding
}

// artworkFinding is the artwork cache against its ceiling: at high water the admin is told what
// the libraries need and the ceiling to set (D212 §7).
func artworkFinding(dependencies *Dependencies) FindingBody {
	pressure := dependencies.Artwork.Pressure()
	finding := FindingBody{
		Check: "artwork cache", Verdict: VerdictOK,
		Detail: fmt.Sprintf("%.1f GiB of a %d GiB ceiling",
			float64(pressure.TotalBytes)/diskspace.BytesPerGiB, pressure.CeilingBytes/diskspace.BytesPerGiB),
	}
	if pressure.IsHighWater {
		finding.Verdict = VerdictWarning
		finding.Detail = fmt.Sprintf("the libraries need %.1f GiB of artwork to be shown at once, more than the "+
			"%d GiB ceiling; artwork beyond it is fetched again when it is opened, which is slower",
			float64(pressure.RequiredBytes)/diskspace.BytesPerGiB, pressure.CeilingBytes/diskspace.BytesPerGiB)
		finding.Action = fmt.Sprintf("set artwork_cache_ceiling_gib to at least %d",
			pressure.RecommendedBytes/diskspace.BytesPerGiB)
	}
	return finding
}

// namedAtMost is how many files or items a doctor's finding names, the rest counted: a justified
// constant, what one line of a page can show.
const namedAtMost = 20

// placesFindings are the files found at more than one place, an admin seeing every place of each, and
// the items whose places' NFO files disagree, where the one beside the place first found speaks
// (D363 §8, §9).
func (server *Server) placesFindings(ctx context.Context, dependencies *Dependencies) ([]FindingBody, error) {
	files, isMore, err := dependencies.Libraries.FilesAtSeveralPlaces(ctx)
	if err != nil {
		return nil, server.libraryError(ctx, "list the files found at more than one place", err)
	}
	several := FindingBody{
		Check: "files found at more than one place", Verdict: VerdictOK, Detail: "none: each file is at one place",
	}
	if len(files) > 0 {
		named := make([]string, 0, min(len(files), namedAtMost))
		for _, file := range files[:min(len(files), namedAtMost)] {
			named = append(named, fmt.Sprintf("file %d at %s", file.FileID, strings.Join(file.Paths, " and ")))
		}
		count := fmt.Sprintf("%d", len(files))
		if isMore {
			count = "at least " + count
		}
		several.Detail = fmt.Sprintf("%s, each one file, byte for byte the same, at every place: %s", count,
			strings.Join(named, "; "))
	}
	disagreeing, err := dependencies.Libraries.ItemsWhoseNFOsDisagree(ctx)
	if err != nil {
		return nil, server.libraryError(ctx, "compare the NFO files of the files found at more than one place", err)
	}
	nfos := FindingBody{
		Check: "NFO files beside the places of one file", Verdict: VerdictOK,
		Detail: "none disagree: a file at several places is described the same at each",
	}
	if len(disagreeing) > 0 {
		named := make([]string, 0, min(len(disagreeing), namedAtMost))
		for _, item := range disagreeing[:min(len(disagreeing), namedAtMost)] {
			named = append(named, fmt.Sprintf("%q (item %d) at %s", item.Name, item.TitleID,
				strings.Join(item.Paths, " and ")))
		}
		nfos.Verdict = VerdictWarning
		nfos.Detail = fmt.Sprintf("%d with NFO files that disagree, the one beside the place found first "+
			"speaking for each: %s", len(disagreeing), strings.Join(named, "; "))
		nfos.Action = "make the NFO files beside each copy the same, or choose the match by hand"
	}
	return []FindingBody{several, nfos}, nil
}
