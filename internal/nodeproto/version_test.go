// SPDX-License-Identifier: GPL-3.0-or-later

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
		reflect.TypeFor[Result](), reflect.TypeFor[Progress](), reflect.TypeFor[Status](),
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
