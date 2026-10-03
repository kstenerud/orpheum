// SPDX-License-Identifier: GPL-3.0-or-later

package library

import (
	"path"
	"slices"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// batchesOf cuts the files a scan needs read into batches (D240 §4, §8): a batch is one folder's,
// of at most perBatch videos, each with the files named for it ("Film.nfo", "Film.en.srt" beside
// "Film.mkv"); a file named for no video in the batch, a folder's "movie.nfo" or "tvshow.nfo", counts
// as a video of its own. Folders and files keep the listing's order.
func batchesOf(files []nodeproto.ExamineFile, perBatch int) [][]nodeproto.ExamineFile {
	var folders []string
	byFolder := map[string][]nodeproto.ExamineFile{}
	for _, file := range files {
		folder := path.Dir(string(file.Path.Bytes()))
		if _, isKnown := byFolder[folder]; !isKnown {
			folders = append(folders, folder)
		}
		byFolder[folder] = append(byFolder[folder], file)
	}
	var batches [][]nodeproto.ExamineFile
	for _, folder := range folders {
		units := unitsOf(byFolder[folder])
		for start := 0; start < len(units); start += perBatch {
			var batch []nodeproto.ExamineFile
			for _, unit := range units[start:min(start+perBatch, len(units))] {
				batch = append(batch, unit...)
			}
			batches = append(batches, batch)
		}
	}
	return batches
}

// unitsOf groups one folder's files: each video with the files named for it, then each file named
// for none of them alone.
func unitsOf(files []nodeproto.ExamineFile) [][]nodeproto.ExamineFile {
	var videoNames []string
	for _, file := range files {
		if file.Kind == nodeproto.ExamineVideo {
			videoNames = append(videoNames, path.Base(string(file.Path.Bytes())))
		}
	}
	units := make([][]nodeproto.ExamineFile, len(videoNames))
	var alone [][]nodeproto.ExamineFile
	videoIndex := 0
	for _, file := range files {
		if file.Kind == nodeproto.ExamineVideo {
			units[videoIndex] = append([]nodeproto.ExamineFile{file}, units[videoIndex]...)
			videoIndex++
			continue
		}
		if owner := ownerIndexOf(path.Base(string(file.Path.Bytes())), videoNames); owner >= 0 {
			units[owner] = append(units[owner], file)
			continue
		}
		alone = append(alone, []nodeproto.ExamineFile{file})
	}
	return slices.Concat(units, alone)
}
