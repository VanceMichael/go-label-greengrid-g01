package domain

import "fmt"

func ReservationTransition(from, to ReservationStatus) error {
	allowed := map[ReservationStatus]map[ReservationStatus]bool{ReservationDraft: {ReservationRequested: true, ReservationCancelled: true}, ReservationRequested: {ReservationApproved: true, ReservationCancelled: true}, ReservationApproved: {ReservationActive: true, ReservationCancelled: true}, ReservationActive: {ReservationReleased: true, ReservationCancelled: true}}
	if allowed[from][to] {
		return nil
	}
	return fmt.Errorf("%w: reservation %s -> %s", ErrState, from, to)
}
func JobTransition(from, to JobStatus) error {
	allowed := map[JobStatus]map[JobStatus]bool{JobQueued: {JobClaimed: true, JobCancelled: true}, JobClaimed: {JobRunning: true, JobQueued: true, JobCancelled: true}, JobRunning: {JobSucceeded: true, JobFailed: true, JobCancelled: true}, JobFailed: {JobQueued: true, JobCancelled: true}}
	if allowed[from][to] {
		return nil
	}
	return fmt.Errorf("%w: job %s -> %s", ErrState, from, to)
}
func ArtifactTransition(from, to ArtifactStatus) error {
	allowed := map[ArtifactStatus]map[ArtifactStatus]bool{ArtifactUploaded: {ArtifactScanned: true}, ArtifactScanned: {ArtifactPromoted: true, ArtifactRetired: true}, ArtifactPromoted: {ArtifactRetired: true}}
	if allowed[from][to] {
		return nil
	}
	return fmt.Errorf("%w: artifact %s -> %s", ErrState, from, to)
}
func IsReservationTerminal(s ReservationStatus) bool {
	return s == ReservationReleased || s == ReservationCancelled
}
func IsJobTerminal(s JobStatus) bool { return s == JobSucceeded || s == JobFailed || s == JobCancelled }
