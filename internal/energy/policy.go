package energy

import (
	"fmt"
	"github.com/VanceMichael/greengrid/internal/domain"
	"math"
	"time"
)

type Policy struct {
	MinimumRenewableShare    float64
	MaximumCarbonGramsPerKWh float64
	RequireCompleteWindow    bool
}
type Decision struct {
	Allowed                         bool
	Reason                          string
	RenewableShare, CarbonIntensity float64
	Samples                         int
}

func DefaultPolicy() Policy {
	return Policy{MinimumRenewableShare: .35, MaximumCarbonGramsPerKWh: 420, RequireCompleteWindow: true}
}
func (p Policy) Validate() error {
	if p.MinimumRenewableShare < 0 || p.MinimumRenewableShare > 1 {
		return fmt.Errorf("%w: renewable threshold", domain.ErrInvalid)
	}
	if p.MaximumCarbonGramsPerKWh <= 0 {
		return fmt.Errorf("%w: carbon threshold", domain.ErrInvalid)
	}
	return nil
}
func (p Policy) Evaluate(summary Efficiency, expectedSamples int) Decision {
	if p.Validate() != nil {
		return Decision{Reason: "invalid policy"}
	}
	if p.RequireCompleteWindow && summary.Samples < expectedSamples {
		return Decision{Reason: "telemetry window incomplete", RenewableShare: summary.RenewableShare, CarbonIntensity: summary.CarbonIntensity(), Samples: summary.Samples}
	}
	intensity := summary.CarbonIntensity()
	if summary.RenewableShare < p.MinimumRenewableShare {
		return Decision{Reason: "renewable share below policy", RenewableShare: summary.RenewableShare, CarbonIntensity: intensity, Samples: summary.Samples}
	}
	if intensity > p.MaximumCarbonGramsPerKWh {
		return Decision{Reason: "carbon intensity above policy", RenewableShare: summary.RenewableShare, CarbonIntensity: intensity, Samples: summary.Samples}
	}
	return Decision{Allowed: true, Reason: "within green compute policy", RenewableShare: summary.RenewableShare, CarbonIntensity: intensity, Samples: summary.Samples}
}

type Slot struct {
	Start, End time.Time
	GPU        int
	Decision   Decision
}

func (p Policy) Admit(summary Efficiency, expected int, start, end time.Time, gpu int) (Slot, error) {
	if err := p.Validate(); err != nil {
		return Slot{}, err
	}
	if end.IsZero() || !end.After(start) || gpu <= 0 {
		return Slot{}, fmt.Errorf("%w: admission window", domain.ErrInvalid)
	}
	d := p.Evaluate(summary, expected)
	if !d.Allowed {
		return Slot{Start: start.UTC(), End: end.UTC(), GPU: gpu, Decision: d}, domain.ErrConflict
	}
	return Slot{Start: start.UTC(), End: end.UTC(), GPU: gpu, Decision: d}, nil
}
func RoundShare(value float64) float64 { return math.Round(value*1000) / 1000 }
