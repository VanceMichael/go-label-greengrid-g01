package telemetry

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/VanceMichael/greengrid/internal/domain"
)

type Quality string

const (
	QualityComplete Quality = "complete"
	QualityMissing  Quality = "missing"
	QualityInvalid  Quality = "invalid"
	QualityOutlier  Quality = "outlier"
)

type SampleAssessment struct {
	Sequence int64
	Quality  Quality
	Reason   string
}

func AssessReadings(readings []domain.TelemetryReading, expectedStart, expectedEnd time.Time, expectedStep time.Duration) ([]SampleAssessment, error) {
	if expectedStep <= 0 || !expectedEnd.After(expectedStart) {
		return nil, fmt.Errorf("%w: telemetry expectation", domain.ErrInvalid)
	}
	ordered := append([]domain.TelemetryReading(nil), readings...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].MeasuredAt.Equal(ordered[j].MeasuredAt) {
			return ordered[i].Sequence < ordered[j].Sequence
		}
		return ordered[i].MeasuredAt.Before(ordered[j].MeasuredAt)
	})
	result := make([]SampleAssessment, 0, len(ordered))
	last := int64(-1)
	for _, reading := range ordered {
		assessment := SampleAssessment{Sequence: reading.Sequence, Quality: QualityComplete}
		if reading.Sequence <= last {
			assessment.Quality = QualityInvalid
			assessment.Reason = "sequence is not increasing"
		}
		if reading.MeasuredAt.Before(expectedStart) || !reading.MeasuredAt.Before(expectedEnd) {
			assessment.Quality = QualityInvalid
			assessment.Reason = "reading is outside requested window"
		}
		if reading.PowerWatts < 0 || reading.RenewableShare < 0 || reading.RenewableShare > 1 {
			assessment.Quality = QualityInvalid
			assessment.Reason = "measurement value outside physical range"
		}
		if reading.PowerWatts > 1000000 {
			assessment.Quality = QualityOutlier
			assessment.Reason = "power exceeds configured accelerator envelope"
		}
		result = append(result, assessment)
		last = reading.Sequence
	}
	return result, nil
}

func MissingExpectedSlots(readings []domain.TelemetryReading, start, end time.Time, step time.Duration) ([]time.Time, error) {
	if step <= 0 || !end.After(start) {
		return nil, fmt.Errorf("%w: telemetry slots", domain.ErrInvalid)
	}
	known := make(map[int64]bool, len(readings))
	for _, reading := range readings {
		if !reading.MeasuredAt.Before(start) && reading.MeasuredAt.Before(end) {
			slot := int64(reading.MeasuredAt.Sub(start) / step)
			known[slot] = true
		}
	}
	var missing []time.Time
	for at, slot := start, int64(0); at.Before(end); at, slot = at.Add(step), slot+1 {
		if !known[slot] {
			missing = append(missing, at)
		}
	}
	return missing, nil
}

func IsStable(readings []domain.TelemetryReading, tolerance float64) bool {
	if len(readings) < 2 || tolerance < 0 {
		return false
	}
	mean := 0.0
	for _, reading := range readings {
		mean += reading.PowerWatts
	}
	mean /= float64(len(readings))
	if mean == 0 {
		return true
	}
	variance := 0.0
	for _, reading := range readings {
		delta := reading.PowerWatts - mean
		variance += delta * delta
	}
	return math.Sqrt(variance/float64(len(readings))) <= mean*tolerance
}
