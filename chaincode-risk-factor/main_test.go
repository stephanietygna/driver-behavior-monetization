package main

import (
	"math"
	"testing"
	"time"
)

func TestDefaultCalibrationIsValid(t *testing.T) {
	if err := validateCalibration(defaultCalibration()); err != nil {
		t.Fatalf("calibração padrão inválida: %v", err)
	}
}

func TestFatigueRiskRemainsNormalized(t *testing.T) {
	calibration := defaultCalibration()
	limit := time.Duration(calibration.FatigueThresholdMinutes) * time.Minute

	tests := []struct {
		name       string
		duration   time.Duration
		wantBase   float64
		wantExcess float64
	}{
		{"no limite", limit, 0, 0},
		{"um minuto acima do limite", limit + time.Minute, 1, float64(time.Minute) / float64(limit+time.Minute)},
		{"dobro do limite", 2 * limit, 1, 0.5},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assessment, err := calculateAssessment("teste", straightTrip(test.duration), calibration)
			if err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}

			assertClose(t, "B_i", assessment.Fatigue.Base, test.wantBase)
			assertClose(t, "E_i", assessment.Fatigue.Excess, test.wantExcess)
			if assessment.RiskFactor < 0 || assessment.RiskFactor > 1 {
				t.Fatalf("R_i deve pertencer a [0, 1]; obtido %.10f", assessment.RiskFactor)
			}
		})
	}
}

func TestValidPauseRestartsFatigueClock(t *testing.T) {
	calibration := defaultCalibration()
	limit := time.Duration(calibration.FatigueThresholdMinutes) * time.Minute
	pause := time.Duration(calibration.MinValidPauseMinutes) * time.Minute
	start := time.Date(2026, time.January, 1, 8, 0, 0, 0, time.UTC)

	readings := []Reading{
		{Timestamp: start, SpeedKmh: 50},
		{Timestamp: start.Add(limit - time.Minute), SpeedKmh: 0},
		{Timestamp: start.Add(limit - time.Minute + pause), SpeedKmh: 50},
		{Timestamp: start.Add(2*limit - 2*time.Minute + pause), SpeedKmh: 50},
	}

	assessment, err := calculateAssessment("teste", readings, calibration)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	assertClose(t, "B_i", assessment.Fatigue.Base, 0)
	assertClose(t, "E_i", assessment.Fatigue.Excess, 0)
}

func straightTrip(duration time.Duration) []Reading {
	start := time.Date(2026, time.January, 1, 8, 0, 0, 0, time.UTC)
	return []Reading{
		{Timestamp: start, Lat: -22.9, Lon: -43.2, SpeedKmh: 50},
		{Timestamp: start.Add(duration), Lat: -22.9, Lon: -43.2, SpeedKmh: 50},
	}
}

func assertClose(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("%s: obtido %.10f; esperado %.10f", name, got, want)
	}
}
