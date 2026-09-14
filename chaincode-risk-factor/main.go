package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/hyperledger/fabric-contract-api-go/contractapi"
)

// Calibration reúne os parâmetros fixados pelo protocolo do estudo.
// As durações usam minutos para facilitar serialização e auditoria.
type Calibration struct {
	AnomalousAccelThreshold float64 `json:"anomalousAccelThreshold"`
	SharpTurnAngleThreshold float64 `json:"sharpTurnAngleThreshold"`
	SharpTurnSpeedThreshold float64 `json:"sharpTurnSpeedThreshold"`
	FatigueThresholdMinutes int64   `json:"fatigueThresholdMinutes"`
	StoppedSpeedThreshold   float64 `json:"stoppedSpeedThreshold"`
	MinValidPauseMinutes    int64   `json:"minValidPauseMinutes"`
	WeightAnomalousAccel    float64 `json:"weightAnomalousAccel"`
	WeightSharpTurn         float64 `json:"weightSharpTurn"`
	WeightFatigue           float64 `json:"weightFatigue"`
}

// Reading representa uma amostra de telemetria enviada pelo cliente.
// O timestamp deve usar RFC 3339, por exemplo: 2026-09-14T10:00:00Z.
type Reading struct {
	Timestamp time.Time `json:"timestamp"`
	Lat       float64   `json:"lat"`
	Lon       float64   `json:"lon"`
	SpeedKmh  float64   `json:"vehicleSpeed"`
}

type FatigueMetrics struct {
	Base   float64 `json:"base"`
	Excess float64 `json:"excess"`
}

// RiskAssessment é gravado no ledger após uma transação bem-sucedida.
// A calibração acompanha o resultado para tornar o cálculo auditável.
type RiskAssessment struct {
	TripID              string         `json:"tripId"`
	StartedAt           time.Time      `json:"startedAt"`
	EndedAt             time.Time      `json:"endedAt"`
	ReadingCount        int            `json:"readingCount"`
	AnomalousAccelCount int            `json:"anomalousAccelCount"`
	SharpTurnCount      int            `json:"sharpTurnCount"`
	AccelerationMetric  float64        `json:"accelerationMetric"`
	TurnMetric          float64        `json:"turnMetric"`
	Fatigue             FatigueMetrics `json:"fatigue"`
	ScoreWithoutExcess  float64        `json:"scoreWithoutExcess"`
	RiskFactor          float64        `json:"riskFactor"`
	Calibration         Calibration    `json:"calibration"`
}

// RiskContract expõe o cálculo de risco como transações Fabric.
type RiskContract struct {
	contractapi.Contract
}

func defaultCalibration() Calibration {
	return Calibration{
		AnomalousAccelThreshold: 3.0,
		SharpTurnAngleThreshold: 0.7,
		SharpTurnSpeedThreshold: 30.0,
		FatigueThresholdMinutes: 80,
		StoppedSpeedThreshold:   3.0,
		MinValidPauseMinutes:    5,
		WeightAnomalousAccel:    0.2025,
		WeightSharpTurn:         0.0253,
		WeightFatigue:           0.7722,
	}
}

// EvaluateTripRisk calcula o fator sem gravar nada no ledger.
// Use-a quando o cliente quiser apenas pré-visualizar o resultado.
func (c *RiskContract) EvaluateTripRisk(
	ctx contractapi.TransactionContextInterface,
	readingsJSON string,
) (*RiskAssessment, error) {
	_ = ctx // O contexto também é recebido nas transações somente de consulta.

	readings, err := parseAndValidateReadings(readingsJSON)
	if err != nil {
		return nil, err
	}

	return calculateAssessment("", readings, defaultCalibration())
}

// CreateRiskAssessment calcula o fator e registra um resultado imutável para tripID.
// Um identificador já existente é rejeitado para evitar sobrescrever um trajeto.
func (c *RiskContract) CreateRiskAssessment(
	ctx contractapi.TransactionContextInterface,
	tripID string,
	readingsJSON string,
) (*RiskAssessment, error) {
	if tripID == "" {
		return nil, errors.New("tripID é obrigatório")
	}

	key, err := ctx.GetStub().CreateCompositeKey("riskAssessment", []string{tripID})
	if err != nil {
		return nil, fmt.Errorf("erro ao criar chave do trajeto: %w", err)
	}

	existing, err := ctx.GetStub().GetState(key)
	if err != nil {
		return nil, fmt.Errorf("erro ao consultar o ledger: %w", err)
	}
	if existing != nil {
		return nil, fmt.Errorf("já existe avaliação para o trajeto %q", tripID)
	}

	readings, err := parseAndValidateReadings(readingsJSON)
	if err != nil {
		return nil, err
	}

	assessment, err := calculateAssessment(tripID, readings, defaultCalibration())
	if err != nil {
		return nil, err
	}

	payload, err := json.Marshal(assessment)
	if err != nil {
		return nil, fmt.Errorf("erro ao serializar avaliação: %w", err)
	}
	if err := ctx.GetStub().PutState(key, payload); err != nil {
		return nil, fmt.Errorf("erro ao gravar avaliação no ledger: %w", err)
	}

	return assessment, nil
}

// ReadRiskAssessment consulta um resultado já registrado no ledger.
func (c *RiskContract) ReadRiskAssessment(
	ctx contractapi.TransactionContextInterface,
	tripID string,
) (*RiskAssessment, error) {
	key, err := ctx.GetStub().CreateCompositeKey("riskAssessment", []string{tripID})
	if err != nil {
		return nil, fmt.Errorf("erro ao criar chave do trajeto: %w", err)
	}

	payload, err := ctx.GetStub().GetState(key)
	if err != nil {
		return nil, fmt.Errorf("erro ao consultar o ledger: %w", err)
	}
	if payload == nil {
		return nil, fmt.Errorf("avaliação não encontrada para o trajeto %q", tripID)
	}

	var assessment RiskAssessment
	if err := json.Unmarshal(payload, &assessment); err != nil {
		return nil, fmt.Errorf("erro ao desserializar avaliação: %w", err)
	}
	return &assessment, nil
}

func calculateAssessment(tripID string, readings []Reading, calibration Calibration) (*RiskAssessment, error) {
	if err := validateCalibration(calibration); err != nil {
		return nil, err
	}

	tripDuration := readings[len(readings)-1].Timestamp.Sub(readings[0].Timestamp)
	tripMinutes := tripDuration.Minutes()

	accelerationCount := countAnomalousAccelerations(readings, calibration)
	turnCount := countSharpTurns(readings, calibration)
	accelerationMetric := math.Min(1, float64(accelerationCount)/tripMinutes)
	turnMetric := math.Min(1, float64(turnCount)/tripMinutes)
	fatigue := analyzeFatigue(readings, calibration)

	// Primeiro, combinam-se as três métricas comportamentais ponderadas.
	scoreWithoutExcess := calibration.WeightAnomalousAccel*accelerationMetric +
		calibration.WeightSharpTurn*turnMetric +
		calibration.WeightFatigue*fatigue.Base

	// Depois, o excesso de fadiga ocupa apenas a parcela restante até 1.
	riskFactor := scoreWithoutExcess + (1-scoreWithoutExcess)*fatigue.Excess
	riskFactor = math.Min(1, math.Max(0, riskFactor))

	return &RiskAssessment{
		TripID:              tripID,
		StartedAt:           readings[0].Timestamp,
		EndedAt:             readings[len(readings)-1].Timestamp,
		ReadingCount:        len(readings),
		AnomalousAccelCount: accelerationCount,
		SharpTurnCount:      turnCount,
		AccelerationMetric:  accelerationMetric,
		TurnMetric:          turnMetric,
		Fatigue:             fatigue,
		ScoreWithoutExcess:  scoreWithoutExcess,
		RiskFactor:          riskFactor,
		Calibration:         calibration,
	}, nil
}

func parseAndValidateReadings(readingsJSON string) ([]Reading, error) {
	var readings []Reading
	if err := json.Unmarshal([]byte(readingsJSON), &readings); err != nil {
		return nil, fmt.Errorf("readingsJSON inválido: %w", err)
	}
	if len(readings) < 2 {
		return nil, errors.New("são necessárias ao menos 2 leituras")
	}

	for index, reading := range readings {
		if reading.Timestamp.IsZero() {
			return nil, fmt.Errorf("leitura %d não possui timestamp", index)
		}
		if reading.SpeedKmh < 0 || math.IsNaN(reading.SpeedKmh) || math.IsInf(reading.SpeedKmh, 0) {
			return nil, fmt.Errorf("velocidade inválida na leitura %d", index)
		}
		if reading.Lat < -90 || reading.Lat > 90 || reading.Lon < -180 || reading.Lon > 180 {
			return nil, fmt.Errorf("coordenada inválida na leitura %d", index)
		}
		if index > 0 && !reading.Timestamp.After(readings[index-1].Timestamp) {
			return nil, fmt.Errorf("timestamps devem estar em ordem crescente: leitura %d", index)
		}
	}

	return readings, nil
}

func validateCalibration(calibration Calibration) error {
	if calibration.FatigueThresholdMinutes <= 0 || calibration.MinValidPauseMinutes <= 0 {
		return errors.New("os limiares de duração devem ser positivos")
	}
	if calibration.AnomalousAccelThreshold < 0 || calibration.SharpTurnAngleThreshold < 0 ||
		calibration.SharpTurnSpeedThreshold < 0 || calibration.StoppedSpeedThreshold < 0 {
		return errors.New("os limiares de velocidade e aceleração não podem ser negativos")
	}

	weightSum := calibration.WeightAnomalousAccel + calibration.WeightSharpTurn + calibration.WeightFatigue
	if calibration.WeightAnomalousAccel < 0 || calibration.WeightSharpTurn < 0 || calibration.WeightFatigue < 0 ||
		math.Abs(weightSum-1) > 1e-9 {
		return errors.New("os pesos devem ser não negativos e somar 1")
	}
	return nil
}

func countAnomalousAccelerations(readings []Reading, calibration Calibration) int {
	count := 0
	for i := 1; i < len(readings); i++ {
		deltaSeconds := readings[i].Timestamp.Sub(readings[i-1].Timestamp).Seconds()
		acceleration := (readings[i].SpeedKmh - readings[i-1].SpeedKmh) / deltaSeconds
		if math.Abs(acceleration) > calibration.AnomalousAccelThreshold {
			count++
		}
	}
	return count
}

func countSharpTurns(readings []Reading, calibration Calibration) int {
	if len(readings) < 3 {
		return 0
	}

	bearings := make([]float64, len(readings))
	bearings[0] = math.NaN()
	for i := 1; i < len(readings); i++ {
		bearings[i] = calculateBearing(readings[i-1].Lat, readings[i-1].Lon, readings[i].Lat, readings[i].Lon)
	}

	count := 0
	for i := 2; i < len(readings); i++ {
		turnAngle := angleDifference(bearings[i], bearings[i-1])
		if turnAngle > calibration.SharpTurnAngleThreshold && readings[i].SpeedKmh >= calibration.SharpTurnSpeedThreshold {
			count++
		}
	}
	return count
}

func calculateBearing(lat1, lon1, lat2, lon2 float64) float64 {
	deltaLon := lon2 - lon1
	x := math.Cos(lat2*math.Pi/180) * math.Sin(deltaLon*math.Pi/180)
	y := math.Cos(lat1*math.Pi/180)*math.Sin(lat2*math.Pi/180) -
		math.Sin(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*math.Cos(deltaLon*math.Pi/180)

	bearing := math.Atan2(x, y)
	if bearing < 0 {
		bearing += 2 * math.Pi
	}
	return bearing
}

func angleDifference(a, b float64) float64 {
	difference := math.Abs(a - b)
	if difference > math.Pi {
		return 2*math.Pi - difference
	}
	return difference
}

func analyzeFatigue(readings []Reading, calibration Calibration) FatigueMetrics {
	periods := continuousDrivingPeriods(readings, calibration)
	fatigueLimit := time.Duration(calibration.FatigueThresholdMinutes) * time.Minute

	var totalDuration, totalExcess time.Duration
	for _, period := range periods {
		totalDuration += period
		if period > fatigueLimit {
			totalExcess += period - fatigueLimit
		}
	}
	if totalDuration == 0 {
		return FatigueMetrics{}
	}

	metrics := FatigueMetrics{Excess: float64(totalExcess) / float64(totalDuration)}
	if totalExcess > 0 {
		metrics.Base = 1
	}
	return metrics
}

func continuousDrivingPeriods(readings []Reading, calibration Calibration) []time.Duration {
	var periods []time.Duration
	drivingStart := readings[0].Timestamp
	isStopped := false
	var stoppedSince time.Time
	minimumPause := time.Duration(calibration.MinValidPauseMinutes) * time.Minute

	closePeriod := func(end time.Time) {
		if duration := end.Sub(drivingStart); duration > 0 {
			periods = append(periods, duration)
		}
	}

	for i := 1; i < len(readings); i++ {
		reading := readings[i]
		if reading.SpeedKmh > calibration.StoppedSpeedThreshold {
			if isStopped && reading.Timestamp.Sub(stoppedSince) >= minimumPause {
				closePeriod(stoppedSince)
				drivingStart = reading.Timestamp
			}
			isStopped = false
			continue
		}
		if !isStopped {
			stoppedSince = reading.Timestamp
			isStopped = true
		}
	}

	periodEnd := readings[len(readings)-1].Timestamp
	if isStopped {
		periodEnd = stoppedSince
	}
	closePeriod(periodEnd)
	return periods
}

func main() {
	chaincode, err := contractapi.NewChaincode(&RiskContract{})
	if err != nil {
		panic(fmt.Errorf("erro ao criar chaincode: %w", err))
	}
	if err := chaincode.Start(); err != nil {
		panic(fmt.Errorf("erro ao iniciar chaincode: %w", err))
	}
}
