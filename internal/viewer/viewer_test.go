package viewer

import "testing"

func TestNextStep(t *testing.T) {
	steps := []float64{0.90, 0.75, 0.55, 0.35}
	cases := map[float64]float64{0.90: 0.75, 0.35: 0.90, 0.80: 0.90, 1: 0.90, 0.7500001: 0.55}
	for cur, want := range cases {
		if got := nextStep(steps, cur); got != want {
			t.Errorf("nextStep(%v) = %v, want %v", cur, got, want)
		}
	}
}
