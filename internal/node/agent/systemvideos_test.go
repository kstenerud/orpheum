// SPDX-License-Identifier: AGPL-3.0-or-later

package agent

import (
	"context"
	"testing"

	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// fakeSystemVideoMaker makes each video asked into /node/system-videos at once.
type fakeSystemVideoMaker struct{}

func (fakeSystemVideoMaker) Make(_ context.Context, command nodeproto.MakeSystemVideos) nodeproto.SystemVideosMade {
	made := nodeproto.SystemVideosMade{Folder: "/node/system-videos"}
	for _, video := range command.Videos {
		made.FileNames = append(made.FileNames, video.Purpose+"."+video.Codec+".mp4")
	}
	return made
}

func (fakeSystemVideoMaker) Folder() string { return "/node/system-videos" }

// makeSystemVideos is the command to make the sound timing clip, for job.
func makeSystemVideos(job int64) nodeproto.Command {
	return nodeproto.Command{CommandSeq: 1, JobID: job, Work: nodeproto.Work{
		Type: nodeproto.CommandMakeSystemVideos, MakeSystemVideos: &nodeproto.MakeSystemVideos{
			Videos: []nodeproto.SystemVideo{
				{Purpose: nodeproto.SystemVideoSoundTiming, Codec: nodeproto.SystemVideoH264AAC},
			},
		},
	}}
}

// The node makes the system videos it is asked for with its maker and tells the core what it made
// (D435 §4).
func TestAgent_MakesTheSystemVideosAndPostsWhatItMade(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	startAgentWithWorkers(t, socket, Workers{Scanner: fakeScanner{}, SystemVideoMaker: fakeSystemVideoMaker{}})
	testutil.Receive(t, core.helloHeard, "the hello")
	core.mutex.Lock()
	folder := core.hellos[0].SystemFolder
	core.mutex.Unlock()
	if folder != "/node/system-videos" {
		t.Errorf("the hello names the system folder %q; want the maker's", folder)
	}
	core.commands <- makeSystemVideos(14)
	result := testutil.Receive(t, core.systemVideos, "the system videos' answer")
	made := result.SystemVideos
	if result.JobID != 14 || made.Failure != "" || made.Folder != "/node/system-videos" ||
		len(made.FileNames) != 1 || made.FileNames[0] != "sound-timing.h264-aac.mp4" {
		t.Errorf("the core heard %+v, %+v; want the clip made", result, made)
	}
}

// A node with no system folder answers the ask with why, so the core does not wait on it.
func TestAgent_ANodeWithNoSystemFolderSaysSo(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	startAgentWithWorkers(t, socket, Workers{Scanner: fakeScanner{}})
	core.commands <- makeSystemVideos(15)
	result := testutil.Receive(t, core.systemVideos, "the system videos' answer")
	if result.SystemVideos.Failure != "this node has no system folder" || len(result.SystemVideos.FileNames) != 0 {
		t.Errorf("the core heard %+v; want the node to say it has no system folder", result.SystemVideos)
	}
}
