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
		wantMetric float64
	}{
		{"no limite", limit, 0},
		{"um minuto acima do limite", limit + time.Minute, float64(time.Minute) / float64(limit+time.Minute)},
		{"dobro do limite", 2 * limit, 0.5},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assessment, err := calculateAssessment("teste", straightTrip(test.duration), calibration)
			if err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}

			assertClose(t, "M_T", assessment.Fatigue.Metric, test.wantMetric)
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
	assertClose(t, "M_T", assessment.Fatigue.Metric, 0)
}

func TestAccelerationUsesTenSecondWindowAndGroupsOneManeuver(t *testing.T) {
	calibration := defaultCalibration()
	start := time.Date(2026, time.January, 1, 8, 0, 0, 0, time.UTC)

	// 50 -> 80 km/h em 8 s deve ser identificado, ainda que existam
	// leituras intermediárias sem um salto individual de 30 km/h.
	validChange := []Reading{
		{Timestamp: start, SpeedKmh: 50},
		{Timestamp: start.Add(2 * time.Second), SpeedKmh: 55},
		{Timestamp: start.Add(4 * time.Second), SpeedKmh: 62},
		{Timestamp: start.Add(6 * time.Second), SpeedKmh: 70},
		{Timestamp: start.Add(8 * time.Second), SpeedKmh: 80},
	}
	if got := countAnomalousAccelerations(validChange, calibration); got != 1 {
		t.Fatalf("variação dentro do intervalo: obtido %d evento(s); esperado 1", got)
	}

	// A regra inclui exatamente 30 km/h: |Delta v| >= 30.
	exactThreshold := []Reading{
		{Timestamp: start, SpeedKmh: 10},
		{Timestamp: start.Add(8 * time.Second), SpeedKmh: 40},
	}
	if got := countAnomalousAccelerations(exactThreshold, calibration); got != 1 {
		t.Fatalf("variação no limite: obtido %d evento(s); esperado 1", got)
	}

	// Mesmo com |Delta v| >= 30, uma janela acima de 10 s não conta.
	lateChange := []Reading{
		{Timestamp: start, SpeedKmh: 20},
		{Timestamp: start.Add(11 * time.Second), SpeedKmh: 51},
	}
	if got := countAnomalousAccelerations(lateChange, calibration); got != 0 {
		t.Fatalf("variação fora do intervalo: obtido %d evento(s); esperado 0", got)
	}

}

func TestAngularWrapUsesSmallestDifference(t *testing.T) {
	got := angleDifference(359*math.Pi/180, 1*math.Pi/180)
	want := 2 * math.Pi / 180
	assertClose(t, "diferença angular circular", got, want)
}

func TestSharpTurnUsesAngleAndGroupsSamples(t *testing.T) {
	calibration := defaultCalibration()
	start := time.Date(2026, time.January, 1, 8, 0, 0, 0, time.UTC)
	readings := []Reading{
		{Timestamp: start, Lat: 0, Lon: 0, SpeedKmh: 50},
		{Timestamp: start.Add(time.Second), Lat: 0, Lon: 0.0001, SpeedKmh: 50},
		{Timestamp: start.Add(2 * time.Second), Lat: 0.0001, Lon: 0.0001, SpeedKmh: 50},
		{Timestamp: start.Add(3 * time.Second), Lat: 0.0001, Lon: 0, SpeedKmh: 50},
	}
	if got := countSharpTurns(readings, calibration); got != 1 {
		t.Fatalf("obtido %d evento(s); esperado 1", got)
	}
}

func TestRiskScoreIsWeightedCombination(t *testing.T) {
	calibration := defaultCalibration()
	duration := 2 * time.Duration(calibration.FatigueThresholdMinutes) * time.Minute
	assessment, err := calculateAssessment("teste", straightTrip(duration), calibration)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	want := calibration.WeightFatigue * 0.5
	assertClose(t, "R_i", assessment.RiskFactor, want)
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
