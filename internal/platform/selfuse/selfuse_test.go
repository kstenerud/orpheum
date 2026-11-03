// SPDX-License-Identifier: AGPL-3.0-or-later

package selfuse_test

import (
	"testing"
	"testing/fstest"

	"github.com/kstenerud/orpheum/internal/platform/selfuse"
)

// processFiles are /proc/self's files for a process that has used 250 ticks of CPU, holds 2048 KiB
// anonymous and 512 KiB of files, and has held 4096 KiB at most. Its name has a space in it.
func processFiles() fstest.MapFS {
	return fstest.MapFS{
		"proc/self/stat": {Data: []byte("4242 (orph eum) S 1 4242 4242 0 -1 4194560 100 0 0 0 " +
			"200 50 0 0 20 0 9 0 100 1000000 500 18446744073709551615\n")},
		"proc/self/status": {Data: []byte("Name:\torpheum\nVmHWM:\t    4096 kB\nVmRSS:\t    2560 kB\n" +
			"RssAnon:\t    2048 kB\nRssFile:\t     512 kB\nThreads:\t9\n")},
	}
}

func TestReadFrom_MeasuresTheProcesssCgroupWhenItHasOne(t *testing.T) {
	files := processFiles()
	files["proc/self/cgroup"] = &fstest.MapFile{Data: []byte("0::/system.slice/orpheum.service\n")}
	group := "sys/fs/cgroup/system.slice/orpheum.service/"
	files[group+"memory.current"] = &fstest.MapFile{Data: []byte("99999\n")}
	files[group+"cpu.stat"] = &fstest.MapFile{Data: []byte("usage_usec 7000000\nuser_usec 5000000\n")}
	files[group+"memory.stat"] = &fstest.MapFile{Data: []byte("anon 3000\nfile 5000\nkernel 10\n")}
	files[group+"memory.peak"] = &fstest.MapFile{Data: []byte("12000\n")}

	sample, err := selfuse.ReadFrom(files)
	if err != nil {
		t.Fatal(err)
	}
	want := selfuse.Sample{
		Scope: selfuse.ScopeGroup, CPUMicros: 7_000_000, HeldBytes: 3000, CacheBytes: 5000, PeakBytes: 12000,
	}
	if sample != want {
		t.Fatalf("sample = %+v, want %+v", sample, want)
	}
}

func TestReadFrom_MeasuresTheProcessAloneInTheRootGroup(t *testing.T) {
	files := processFiles()
	// The machine's root group: it has cpu.stat, but no memory.current, and counts every process.
	files["proc/self/cgroup"] = &fstest.MapFile{Data: []byte("0::/\n")}
	files["sys/fs/cgroup/cpu.stat"] = &fstest.MapFile{Data: []byte("usage_usec 999999999\n")}

	sample, err := selfuse.ReadFrom(files)
	if err != nil {
		t.Fatal(err)
	}
	want := selfuse.Sample{
		Scope: selfuse.ScopeProcess, CPUMicros: 2_500_000, HeldBytes: 2048 * 1024, CacheBytes: 512 * 1024,
		PeakBytes: 4096 * 1024,
	}
	if sample != want {
		t.Fatalf("sample = %+v, want %+v", sample, want)
	}
}

func TestReadFrom_AGroupWithoutAPeakSaysNone(t *testing.T) {
	files := fstest.MapFS{
		"proc/self/cgroup":                  {Data: []byte("0::/\n")},
		"sys/fs/cgroup/memory.current":      {Data: []byte("1\n")},
		"sys/fs/cgroup/cpu.stat":            {Data: []byte("usage_usec 10\n")},
		"sys/fs/cgroup/memory.stat":         {Data: []byte("anon 1\nfile 2\n")},
		"sys/fs/cgroup/some.other.thing":    {Data: []byte("x\n")},
		"sys/fs/cgroup/memory.swap.current": {Data: []byte("0\n")},
	}
	sample, err := selfuse.ReadFrom(files)
	if err != nil {
		t.Fatal(err)
	}
	if sample.Scope != selfuse.ScopeGroup || sample.PeakBytes != 0 || sample.CPUMicros != 10 {
		t.Fatalf("sample = %+v, want the group's, with no peak", sample)
	}
}

func TestReadFrom_FailsWithNeitherAGroupNorTheProcesssFiles(t *testing.T) {
	if _, err := selfuse.ReadFrom(fstest.MapFS{}); err == nil {
		t.Fatal("read nothing without an error")
	}
}
