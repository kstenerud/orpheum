// SPDX-License-Identifier: AGPL-3.0-or-later

package nodeproto

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// wireShapeByVersion is the shape of the protocol's messages at each version the core and node have
// spoken: a hash of every field of every message, at any depth, with its type and wire tag. A version
// is added here with each raise of ProtocolVersion, and never changed after.
var wireShapeByVersion = map[int]string{
	4:  "f3ad787eff46694996512e112edea5efbffbd5af6f568ca8b301a856ec5b4ffd",
	5:  "6db76ce5e79b3e0198f65281b6384627bfde1c59772dd3306749907130fd07b4",
	6:  "01f8607683c4c01ccda9f92659d65429e79b898e321847dcbaa353fd5ddfba12",
	7:  "e45e4d83b8896df59a1e8d76782b1fd047513641876688e9a2fe14e44c0e0a1f",
	8:  "45d351f69b783bd84c637d2cd83a55d231d2ef3f1f430b5188c6546e7ec710c4",
	9:  "d39d0c7c40958ec5b027fe17884514971ad2d8bc71de79ed4bc76cd1188100d5",
	10: "2f3cd200ae0779b316951054166c285a70bcf19390d59b2f5de523f39b548f70",
	11: "0ea1056ab237bcc19e294458af0fbb9eead973f7aabbb580abfc6f5da703470d",
	12: "25647d5faba4f24b0c1beeab4ee49f9087895d6f5192418ea1fd9968bacc5b0d",
	// 13 adds the reprioritize command, which a node of 12 would refuse; the fields are unchanged.
	13: "25647d5faba4f24b0c1beeab4ee49f9087895d6f5192418ea1fd9968bacc5b0d",
	// 14 adds the list and examine commands, their results and the listed record (D240 §2, §4).
	14: "2a89ff73d174a3e775f283925b9c5d8873e5bdf29c6e3c4c43537468564ca5b0",
	// 15 drops the scan command and the scan's records from the report, which carries the status alone,
	// and the status's activity (D240 §2).
	15: "31e1c26d35d69851f52a022f653d214c2e2e6d51cbb97af4254d424430c6ce1c",
	// 16 posts the status on its own (PathStatus), and the report stream, its records and the hello
	// reply's report sequence are gone (the jobs rebuild's 4e.4).
	16: "47fd403d76a5339300ab5ee965b6d80a4f01e7be57d651caba02532794805719",
	// 17 adds a stall's condition and whether its process was left running, and the status's stuck
	// processes (D233 §5, §10).
	17: "fe3a6d405ae55bec7709edec6df30a7b3e0f5c1a04e89a034049bb677b03e82a",
	// 18 adds how long each held attempt has not moved, and a listing's and an examining's stall
	// (D233 §5, §10).
	18: "c53b7595df6102deff942ada4eb498b1b3fb7eb995e1556731f69a7b4dada729",
	// 19 adds where the node's byte socket is to its hello (D242 §7).
	19: "821d42fb4c9c3edb01da3df1ac6eb2d61ee05a4a4a45b6d100addb3b3feba80c",
	// 20 adds, to a stall waiting on the disk, the file the run was blocked on (D244 §6).
	20: "7ef9e0f76c3122e4e8bdeb3c4ce62a5be3f9d80d632f756a0a56e5b73910df73",
	// 21 names a listed entry's filesystem device number for what it is (the pre-M4 audit).
	21: "a9b483b035696bb1912c31aebc850e6012cb9f5edfa9529972c014c264dd1fbb",
	// 22 tells a measurement the film's length, so its watchdog allows decoding a long film's sound.
	22: "be406c8890f5721b73757d3aa6cc346f54f36bc69bdefc6168d9eb2f61964f83",
	// 23 replaces the loudness measurement with a read-through, which measures the loudness and reads
	// the picture's timeline, its keyframes and its bytes in each second, from one read (D247 §10).
	23: "b81a47ac1d4856a018cdffb9ba6ba28a4fdaa850508d46dbea3e15620bd1462a",
	24: "baaf26c7c78792f6a4cdedeeb6631cad0f13a1ecabedaf18461ceb5613e162e3",
	25: "a0a19e06ed2b10f3d482fec058ed12947fc52bc04715cc2400020bbcc68ca410",
	26: "618c75d2060446306e9d2f35978547360de7593cf7734f8f0cfba88a0f986366",
	27: "da2013ff5f23f714b20736e23777b8bc2b789e3081c303b1f8dabf696d258b93",
	28: "6bdbf1b3f005722262d083b7f09eb588726d77d8bb5d84f956c3ae27cc338a74",
	29: "f66a79df079cd7e04cdbacee5f2969d3b6402bac42928c3130732c3c9f4f5ce3",
	// 30 renames the encoding prepare profile to "h264" (retired-names.md): the shape is the same, the
	// value is not, and a node of 29 refuses it (D270 §2).
	30: "f66a79df079cd7e04cdbacee5f2969d3b6402bac42928c3130732c3c9f4f5ce3",
	// 31 adds the watcher's notice of a change (PathChanged) and each root's watch in the status
	// (D272 §9); the notice joins the hashed messages from here.
	31: "030d5c2639c2f1ea6e9ad4e81141f5a33f12c52587ea0661578a90c54a4f7d29",
	// 32 adds each listed and observed file's inode, and an observed file's device, which tell a
	// file moved on its disk (D304 §5).
	32: "6193ac0e641a4dbaefbf2f1d37a06e958fdf23d2415aafa5b0303ca151862e4c",
	33: "8a094d78ee941cd24b40719b1ab30d899d9465b61ec6524b43094e1ddd77e03e",
	// 34 adds the core's Tuning of the node, in the hello reply and each poll's answer: first how often
	// the node posts its status, in place of its own config (the constants sweep, 2026-10-21).
	34: "97484e9114c9f3556ae2bc64055042b31149b03b622ff057df198ed07b274a87",
	// 35 has a listing say its error limit, the core's scan_root_error_limit, in place of the node's own
	// config (the constants sweep, 2026-10-21).
	35: "494b119bde4383d8dd578766fdcec651dc2759938fbb394eb745c1f0f0f9d36c",
	// 36 adds how often a prepare's progress is said to the Tuning (the constants sweep, 2026-10-21).
	36: "9370144c4db938cbce07c6c4d2976ccd9ad033ad1deec4c4e038488f677ee268",
	37: "727f6701763679738cf8331a8cda1b2959374d01bf3e570946c8e9d920e19348",
	38: "caf44b4145f192a52a5b4e20128850afdf39ae3df07df587ec5cf0353163cd36",
	// 39 adds whether a status's disk is not answering (the dead-share tests, 2026-10-21).
	39: "eea95fc59020a791583ec1fe5af5d18a9bb257651e30bcf0d2e558961e883ae6",
	// 40 adds a root's disk's device and whether it is a mount point (the dead-share tests, 2026-10-21).
	40: "7866b0da4ab3bbaf08275dc1e008da4f4b58bc843d3324212885654f8322f333",
	// 41 adds the probe of a root on the byte socket, PlaceProbe (D327).
	41: "1514f7e736a75243c304c27e9370072da046d898c94ce7c11dbbee4ffa4fc29d",
	// 42 probes any place, a root or a disk inside one, PlaceProbe naming its Path (D328 §6).
	42: "c0d627168692620ce61b4247709be8bc076034e0259bc812fbdc17dba2143994",
	// 43 says in the hello whether the node's FFmpeg reads films through it (Hardware, D330).
	43: "0613182317b5b4bd95c62024b19b1ee4f36ce08491d6d1d173b92813b464a979",
	44: "b21c5b100bbb082b3ce7496eb22f03e2bb09b8e4bab2b06f024956f1a710fe0e",
	// 45 extracts a video's text tracks together: the command names them all, and the answer carries
	// each one's text or failure (D340).
	45: "281baf20e25334ac786b9fdf670f975728f014b6575523f15665987b442beca4",
	// 46 adds a probed file's recorded time, RecordedAtMs, an other video's date (D349 §4).
	46: "62b38ced485d4bf7b82d89f02e9eae5cb1b1424e1fc3a47c19c3189a626d2c84",
	// 47 says a disc image's probe is of its main title (ProbeFormat.Disc), and a disc's folder is
	// skipped as disc_folder_not_supported_yet, an image being read (D350).
	47: "fef4610c120047fd82fb706c6cdf70d01b3ccfa72930f2260cfa09ff901050bf",
	// 48 adds the folders in a folder on the byte socket, FolderListing, and names a library folder
	// inside a root as a command's root (D359): a node of 47 would refuse it.
	48: "8b869c817d2f83afa184b486739a0b5d4efa856693582e0bead30daf2ec22981",
	// 49 names the library folders the node watches in the hello's reply and each poll's answer,
	// WatchFolders, and a folder's watch in the status is its own, not its configured root's (D359's
	// amendment of 2026-10-27).
	49: "a64273941d3a9cbf33f8df156926b57359a3bcc8c640dba23d808f34e28e9f70",
	// 50 has an artwork command name a video to take a frame still from, its picture stream and
	// where, and its answer say the video has no frame that is not black (D362): a node of 49 would
	// read no image.
	50: "1d260c54ce86fb96af1f06e595f53bb362aba9c8104f367c7fb69c6536c57c33",
}

// TestProtocolVersion_IsRaisedWithAnyChangeToTheWire holds that the messages' shape is the one this
// ProtocolVersion was raised for (D233 §9): a field added, removed, retyped or renamed on the wire
// without raising it, or a raise without its shape recorded, fails.
func TestProtocolVersion_IsRaisedWithAnyChangeToTheWire(t *testing.T) {
	t.Parallel()
	got := wireShape()
	if want, recorded := wireShapeByVersion[ProtocolVersion]; !recorded || want != got {
		t.Errorf("the wire's shape is %s; protocol %d records %q. Changing the wire raises ProtocolVersion, "+
			"and the new version's shape is recorded in wireShapeByVersion", got, ProtocolVersion, want)
	}
}

// wireShape is a hash of every field of every message, at any depth.
func wireShape() string {
	seen := map[reflect.Type]bool{}
	var fields []string
	for _, message := range []reflect.Type{
		reflect.TypeFor[Hello](), reflect.TypeFor[HelloReply](), reflect.TypeFor[Commands](),
		reflect.TypeFor[Result](), reflect.TypeFor[Progress](), reflect.TypeFor[Status](), reflect.TypeFor[Changed](),
		reflect.TypeFor[PlaceProbe](), reflect.TypeFor[FolderListing](),
	} {
		fields = append(fields, fieldsOf(message, seen)...)
	}
	slices.Sort(fields)
	sum := sha256.Sum256([]byte(strings.Join(fields, "\n")))
	return hex.EncodeToString(sum[:])
}

func fieldsOf(typ reflect.Type, seen map[reflect.Type]bool) []string {
	for typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Slice || typ.Kind() == reflect.Array ||
		typ.Kind() == reflect.Map {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct || seen[typ] {
		return nil
	}
	seen[typ] = true
	var fields []string
	for field := range typ.Fields() {
		fields = append(fields, fmt.Sprintf("%s.%s %s %q", typ.String(), field.Name, field.Type.String(),
			field.Tag.Get("json")))
		fields = append(fields, fieldsOf(field.Type, seen)...)
	}
	return fields
}
