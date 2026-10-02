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
		reflect.TypeFor[Result](), reflect.TypeFor[Progress](), reflect.TypeFor[Record](),
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
