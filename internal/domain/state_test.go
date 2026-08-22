package domain

import "testing"

func TestReservationTransitions(t *testing.T) {
	cases := []struct {
		name     string
		from, to ReservationStatus
		wantErr  bool
	}{
		{"draft request", ReservationDraft, ReservationRequested, false},
		{"request approve", ReservationRequested, ReservationApproved, false},
		{"approve active", ReservationApproved, ReservationActive, false},
		{"active release", ReservationActive, ReservationReleased, false},
		{"draft cancel", ReservationDraft, ReservationCancelled, false},
		{"requested cancel", ReservationRequested, ReservationCancelled, false},
		{"approved cancel", ReservationApproved, ReservationCancelled, false},
		{"active cancel", ReservationActive, ReservationCancelled, false},
		{"released request", ReservationReleased, ReservationRequested, true},
		{"cancelled active", ReservationCancelled, ReservationActive, true},
		{"draft active", ReservationDraft, ReservationActive, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ReservationTransition(tc.from, tc.to)
			if (err != nil) != tc.wantErr {
				t.Fatalf("error=%v wantErr=%v", err, tc.wantErr)
			}
		})
	}
}

func TestJobTransitions(t *testing.T) {
	valid := [][2]JobStatus{{JobQueued, JobClaimed}, {JobQueued, JobCancelled}, {JobClaimed, JobRunning}, {JobClaimed, JobQueued}, {JobClaimed, JobCancelled}, {JobRunning, JobSucceeded}, {JobRunning, JobFailed}, {JobRunning, JobCancelled}, {JobFailed, JobQueued}, {JobFailed, JobCancelled}}
	for _, pair := range valid {
		if err := JobTransition(pair[0], pair[1]); err != nil {
			t.Errorf("%s -> %s: %v", pair[0], pair[1], err)
		}
	}
	invalid := [][2]JobStatus{{JobSucceeded, JobQueued}, {JobCancelled, JobRunning}, {JobQueued, JobRunning}, {JobRunning, JobQueued}, {JobClaimed, JobSucceeded}}
	for _, pair := range invalid {
		if err := JobTransition(pair[0], pair[1]); err == nil {
			t.Errorf("%s -> %s unexpectedly valid", pair[0], pair[1])
		}
	}
}

func TestArtifactTransitions(t *testing.T) {
	for _, pair := range [][2]ArtifactStatus{{ArtifactUploaded, ArtifactScanned}, {ArtifactScanned, ArtifactPromoted}, {ArtifactScanned, ArtifactRetired}, {ArtifactPromoted, ArtifactRetired}} {
		if err := ArtifactTransition(pair[0], pair[1]); err != nil {
			t.Fatal(err)
		}
	}
	for _, pair := range [][2]ArtifactStatus{{ArtifactPromoted, ArtifactUploaded}, {ArtifactRetired, ArtifactPromoted}, {ArtifactUploaded, ArtifactPromoted}} {
		if err := ArtifactTransition(pair[0], pair[1]); err == nil {
			t.Errorf("invalid transition accepted: %v", pair)
		}
	}
}

func TestRolesAndTerminalStates(t *testing.T) {
	for _, role := range []Role{RolePlatformAdmin, RoleClusterOps, RoleTenantAdmin, RoleScheduler, RoleAuditor} {
		if !role.Valid() {
			t.Errorf("role invalid: %s", role)
		}
	}
	if Role("visitor").Valid() {
		t.Fatal("unknown role accepted")
	}
	if !IsReservationTerminal(ReservationReleased) || !IsReservationTerminal(ReservationCancelled) {
		t.Fatal("reservation terminal state missing")
	}
	if !IsJobTerminal(JobSucceeded) || !IsJobTerminal(JobFailed) || !IsJobTerminal(JobCancelled) {
		t.Fatal("job terminal state missing")
	}
}
