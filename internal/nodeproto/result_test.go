// SPDX-License-Identifier: GPL-3.0-or-later

package nodeproto

import (
	"reflect"
	"strings"
	"testing"
)

// TestEnvelope_AJobIsNamedByTheEnvelopeAloneNeverByWhatItCarries holds that a command and a result
// name their job in the envelope (Command.JobID, Result.JobID) and nowhere else: no payload either
// carries, at any depth, has a job id of its own to disagree with it (D234 §2).
func TestEnvelope_AJobIsNamedByTheEnvelopeAloneNeverByWhatItCarries(t *testing.T) {
	t.Parallel()
	for _, envelope := range []reflect.Type{reflect.TypeFor[Command](), reflect.TypeFor[Result]()} {
		if _, hasJobID := envelope.FieldByName("JobID"); !hasJobID {
			t.Errorf("%s has no JobID: the envelope names the job", envelope.Name())
		}
		for field := range envelope.Fields() {
			if field.Name == "JobID" {
				continue
			}
			for _, where := range jobIDsIn(field.Type, envelope.Name()+"."+field.Name, map[reflect.Type]bool{}) {
				t.Errorf("%s carries a job id of its own; the envelope alone names the job", where)
			}
		}
	}
}

// jobIDsIn is where in a type, at any depth, a field names a job: a JobID, or a jobId on the wire.
func jobIDsIn(typ reflect.Type, path string, seen map[reflect.Type]bool) []string {
	for typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Slice || typ.Kind() == reflect.Array {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct || seen[typ] {
		return nil
	}
	seen[typ] = true
	var found []string
	for field := range typ.Fields() {
		wireName, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		if field.Name == "JobID" || wireName == "jobId" {
			found = append(found, path+"."+field.Name)
		}
		found = append(found, jobIDsIn(field.Type, path+"."+field.Name, seen)...)
	}
	return found
}
