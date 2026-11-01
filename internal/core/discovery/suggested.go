// SPDX-License-Identifier: AGPL-3.0-or-later

package discovery

import "github.com/kstenerud/orpheum/discovery/trust"

// This file is the one place Orpheum names its paid discovery and its publisher root (D391 §7, D393
// §2): every other shipped source takes them from here, through the core
// (TestSuggested_IsTheOnlyShippedSourceThatNamesTheOwnersDomains).

// Suggestion is the discovery a core's admin is offered first: its issuer, and the genesis of its
// root chain, whose hash is its identity. A core enrolling with the suggested issuer takes its chain
// only from this genesis, never from one the issuer shows (the root key audit's §3.8). A fork or a
// household that wants none offers none: the System page then asks for an issuer with no default.
type Suggestion struct {
	Issuer  string
	Genesis trust.Signed
}

// Suggested is the paid service's suggestion (D390 §1, D391 §7).
//
// Its genesis is not made yet: the owner makes it at the day-one ceremony (the audit's §4) and puts
// the statement and its signature here, the identity in the commit's message. Until then enrolling
// with the suggested issuer fails, saying so, rather than trusting whatever chain the issuer shows.
func Suggested() Suggestion {
	return Suggestion{
		Issuer: "https://orpheum.cc",
		// The discovery chain's genesis statement and its signature file go here, verbatim.
		Genesis: trust.Signed{
			Payload: `orpheum-signed root 1
chain discovery
version 1
issued 2026-11-01T07:22:49Z
key recovery uv sk-ssh-ed25519@openssh.com AAAAGnNrLXNzaC1lZDI1NTE5QG9wZW5zc2guY29tAAAAIPTJCy1+O0GKq+A2Dex8lrs+whOiR4E0MYMJ7timRtqGAAAAGHNzaDpvcnBoZXVtLWRpc2NvdmVyeS1yMQ== discovery-r1
key recovery uv sk-ssh-ed25519@openssh.com AAAAGnNrLXNzaC1lZDI1NTE5QG9wZW5zc2guY29tAAAAIIfjdWUhe7puttEl68F0CD2LzwU9kNIQDOlRHKBteRjXAAAAGHNzaDpvcnBoZXVtLWRpc2NvdmVyeS1yMg== discovery-r2
key everyday uv sk-ssh-ed25519@openssh.com AAAAGnNrLXNzaC1lZDI1NTE5QG9wZW5zc2guY29tAAAAIPZGOvmSbEy4F+wDfZgU8z+YIH+1I4NYx9TN5iQ/wMvYAAAAGHNzaDpvcnBoZXVtLWRpc2NvdmVyeS1lMQ== discovery-e1
threshold recovery 1
threshold everyday 1
location https://orpheum.cc
`,
			Signature: `-----BEGIN SSH SIGNATURE-----
U1NIU0lHAAAAAQAAAF4AAAAac2stc3NoLWVkMjU1MTlAb3BlbnNzaC5jb20AAAAg9kY6+Z
JsTLgX7AN9mBTzP5ggf7Ujg1jH1M3mJD/Ay9gAAAAYc3NoOm9ycGhldW0tZGlzY292ZXJ5
LWUxAAAAGW9ycGhldW0tZGlzY292ZXJ5LXJvb3QtdjEAAAAAAAAABnNoYTUxMgAAAGcAAA
Aac2stc3NoLWVkMjU1MTlAb3BlbnNzaC5jb20AAABA4BeSOwET2la86wcp7tZJg5rcW83M
QE3fpnJvDDEYHWgIMt5GyIYQHNu4sg6sHPZzN0s0e8Ob1oqVvNq73a9qBwUAAAAJ
-----END SSH SIGNATURE-----
-----BEGIN SSH SIGNATURE-----
U1NIU0lHAAAAAQAAAF4AAAAac2stc3NoLWVkMjU1MTlAb3BlbnNzaC5jb20AAAAg9MkLLX
47QYqr4DYN7HyWuz7CE6JHgTQxgwnu2KZG2oYAAAAYc3NoOm9ycGhldW0tZGlzY292ZXJ5
LXIxAAAAGW9ycGhldW0tZGlzY292ZXJ5LXJvb3QtdjEAAAAAAAAABnNoYTUxMgAAAGcAAA
Aac2stc3NoLWVkMjU1MTlAb3BlbnNzaC5jb20AAABAf6MQcefr6f5P4EiohLcyW5hTgLyE
jSZqO+HDEmEpDCPH3lafQH2ziE8bQJq1J1urmmOw4UsbkMXIo6S6pi97BQUAAAAJ
-----END SSH SIGNATURE-----
-----BEGIN SSH SIGNATURE-----
U1NIU0lHAAAAAQAAAF4AAAAac2stc3NoLWVkMjU1MTlAb3BlbnNzaC5jb20AAAAgh+N1ZS
F7um620SXrwXQIPYvPBT2Q0hAM6VEcoG15GNcAAAAYc3NoOm9ycGhldW0tZGlzY292ZXJ5
LXIyAAAAGW9ycGhldW0tZGlzY292ZXJ5LXJvb3QtdjEAAAAAAAAABnNoYTUxMgAAAGcAAA
Aac2stc3NoLWVkMjU1MTlAb3BlbnNzaC5jb20AAABArPUDoZ5GKGLkTg/pBE1ZoNfv1ZVg
DSiGq31moEAT00Y+YMIg8bRZfK9Ai4ip2K3EVeGirUv1WqapgC8UYem7AgUAAAAI
-----END SSH SIGNATURE-----
`,
		},
	}
}

// PublisherRoot is Orpheum's publisher root, which signs release records (D393 §2): the genesis of its
// chain, compiled into every binary, and the places its chain and record are served at.
type PublisherRoot struct {
	Genesis   trust.Signed
	Locations []string
}

// Publisher is the publisher root this build trusts. Its genesis is made at the day-one ceremony too;
// until it is here a release build checks no release record, and says so in its log.
func Publisher() PublisherRoot {
	return PublisherRoot{
		// The publisher chain's genesis statement and its signature file go here, verbatim.
		Genesis: trust.Signed{
			Payload: `orpheum-signed root 1
chain publisher
version 1
issued 2026-11-01T07:22:49Z
key recovery uv sk-ssh-ed25519@openssh.com AAAAGnNrLXNzaC1lZDI1NTE5QG9wZW5zc2guY29tAAAAIEiluylB6UgBPQmW37P15jrcfDb6DEGWei4EVDfNon2hAAAAGHNzaDpvcnBoZXVtLXB1Ymxpc2hlci1yMQ== publisher-r1
key recovery uv sk-ssh-ed25519@openssh.com AAAAGnNrLXNzaC1lZDI1NTE5QG9wZW5zc2guY29tAAAAIPzrv4G3J8LX9Gr7aCHnFFJ/1Vb4ZG86M2rSBRwvD7CMAAAAGHNzaDpvcnBoZXVtLXB1Ymxpc2hlci1yMg== publisher-r2
key everyday uv sk-ssh-ed25519@openssh.com AAAAGnNrLXNzaC1lZDI1NTE5QG9wZW5zc2guY29tAAAAIBh4svjVnlArynMAmvAKIBbB06F6l5uLvtq7mNQCF2vjAAAAGHNzaDpvcnBoZXVtLXB1Ymxpc2hlci1lMQ== publisher-e1
threshold recovery 1
threshold everyday 1
location https://orpheum.cc
`,
			Signature: `-----BEGIN SSH SIGNATURE-----
U1NIU0lHAAAAAQAAAF4AAAAac2stc3NoLWVkMjU1MTlAb3BlbnNzaC5jb20AAAAgGHiy+N
WeUCvKcwCa8AogFsHToXqXm4u+2ruY1AIXa+MAAAAYc3NoOm9ycGhldW0tcHVibGlzaGVy
LWUxAAAAGW9ycGhldW0tcHVibGlzaGVyLXJvb3QtdjEAAAAAAAAABnNoYTUxMgAAAGcAAA
Aac2stc3NoLWVkMjU1MTlAb3BlbnNzaC5jb20AAABAh560SQxaDw1CrPQjWFr77utYHqKF
VJwd5RWuEVCxKCmwi3MAlkUR90iUtRAqIJwdQBrKzOchLr6eEUOLqeggCgUAAAAJ
-----END SSH SIGNATURE-----
-----BEGIN SSH SIGNATURE-----
U1NIU0lHAAAAAQAAAF4AAAAac2stc3NoLWVkMjU1MTlAb3BlbnNzaC5jb20AAAAgSKW7KU
HpSAE9CZbfs/XmOtx8NvoMQZZ6LgRUN82ifaEAAAAYc3NoOm9ycGhldW0tcHVibGlzaGVy
LXIxAAAAGW9ycGhldW0tcHVibGlzaGVyLXJvb3QtdjEAAAAAAAAABnNoYTUxMgAAAGcAAA
Aac2stc3NoLWVkMjU1MTlAb3BlbnNzaC5jb20AAABA0tpfNcrgfrU0P3sFJpaeQ6PHgqjj
j2Y+82UIAja89k1JoQC1KemtJKyACoWG1DpFaB/UTcR5P44Sg6FogxHHBgUAAAAI
-----END SSH SIGNATURE-----
-----BEGIN SSH SIGNATURE-----
U1NIU0lHAAAAAQAAAF4AAAAac2stc3NoLWVkMjU1MTlAb3BlbnNzaC5jb20AAAAg/Ou/gb
cnwtf0avtoIecUUn/VVvhkbzozatIFHC8PsIwAAAAYc3NoOm9ycGhldW0tcHVibGlzaGVy
LXIyAAAAGW9ycGhldW0tcHVibGlzaGVyLXJvb3QtdjEAAAAAAAAABnNoYTUxMgAAAGcAAA
Aac2stc3NoLWVkMjU1MTlAb3BlbnNzaC5jb20AAABAIVRtoZfUGyy9tJcvbaggvy/apUly
pkBak1bXt+3PsZaHNwvUslpsXRgWdAHdqlC0I/lkqUUbHvA7eTdC/Le5CAUAAAAJ
-----END SSH SIGNATURE-----
`,
		},
		Locations: []string{"https://orpheum.cc"},
	}
}
