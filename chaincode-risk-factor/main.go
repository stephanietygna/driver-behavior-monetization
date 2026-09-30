package main

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/hyperledger/fabric-chaincode-go/shim"
	"github.com/hyperledger/fabric-contract-api-go/contractapi"
)

// Calibration reúne os parâmetros fixados pelo protocolo do estudo.
// As durações usam minutos para facilitar serialização e auditoria.
type Calibration struct {
	// Aceleração/desaceleração: |Δv| >= 30 km/h em uma janela de até 10 s,
	// comparando a leitura atual com referências anteriores nessa janela.
	AnomalousSpeedChangeThresholdKmh float64 `json:"anomalousSpeedChangeThresholdKmh"`
	MaxSampleGapSeconds              int64   `json:"maxSampleGapSeconds"`
	// Curvas: variação angular bruta entre bearings consecutivos em radianos.
	SharpTurnAngleThresholdRad float64 `json:"sharpTurnAngleThresholdRad"`
	SharpTurnSpeedThresholdKmh float64 `json:"sharpTurnSpeedThresholdKmh"`
	FatigueThresholdMinutes    int64   `json:"fatigueThresholdMinutes"`
	StoppedSpeedThreshold      float64 `json:"stoppedSpeedThreshold"`
	MinValidPauseMinutes       int64   `json:"minValidPauseMinutes"`
	WeightAnomalousAccel       float64 `json:"weightAnomalousAccel"`
	WeightSharpTurn            float64 `json:"weightSharpTurn"`
	WeightFatigue              float64 `json:"weightFatigue"`
}

// Reading representa uma amostra de telemetria enviada pelo cliente.
// O timestamp deve usar RFC 3339, por exemplo: 2026-09-14T10:00:00Z.
type Reading struct {
	Timestamp time.Time `json:"timestamp"`
	Lat       float64   `json:"lat"`
	Lon       float64   `json:"lon"`
	SpeedKmh  float64   `json:"vehicleSpeed"`
}

// BufferedReading conserva os campos textuais recebidos do OBD. Eles são
// convertidos e validados no momento da entrada, mas permanecem auditáveis no
// ledger exatamente como foram enviados pelo cliente.
type BufferedReading struct {
	Sequence     int    `json:"sequence"`
	Timestamp    string `json:"timestamp"`
	Latitude     string `json:"latitude"`
	Longitude    string `json:"longitude"`
	VehicleSpeed string `json:"vehicleSpeed"`
}

// TripBuffer controla o recebimento sequencial de um trajeto. A última data
// impede que uma leitura fora de ordem altere as métricas posteriormente.
type TripBuffer struct {
	TripID        string    `json:"tripId"`
	ReadingCount  int       `json:"readingCount"`
	LastTimestamp time.Time `json:"lastTimestamp"`
	Finalized     bool      `json:"finalized"`
}

type FatigueMetrics struct {
	// Metric é a parcela do tempo total conduzida além do limiar, calculada
	// separadamente para cada período contínuo delimitado por pausa válida.
	Metric float64 `json:"metric"`
	// Os campos abaixo tornam a métrica auditável em unidades de tempo.
	TotalDrivingMinutes      float64 `json:"totalDrivingMinutes"`
	LongestContinuousMinutes float64 `json:"longestContinuousMinutes"`
	ExcessMinutes            float64 `json:"excessMinutes"`
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
	RiskFactor          float64        `json:"riskFactor"`
	Calibration         Calibration    `json:"calibration"`
}

// RiskContract expõe o cálculo de risco como transações Fabric.
type RiskContract struct {
	contractapi.Contract
}

// serverConfig contém os valores que o peer entrega ao contêiner no modo CCAS.
// O PACKAGE_ID calculado na instalação torna-se o CHAINCODE_ID.
type serverConfig struct {
	CCID    string
	Address string
}

func defaultCalibration() Calibration {
	return Calibration{
		AnomalousSpeedChangeThresholdKmh: 30.0,
		MaxSampleGapSeconds:              10,
		SharpTurnAngleThresholdRad:       0.7,
		SharpTurnSpeedThresholdKmh:       30.0,
		FatigueThresholdMinutes:          80,
		StoppedSpeedThreshold:            3.0,
		MinValidPauseMinutes:             5,
		// Pesos normalizados das porcentagens relativas de acidentes das três
		// funções do modelo: 16,12% (aceleração), 1,96% (curva) e 61,77%
		// (cansaço). A soma original é 79,85%; após normalização, os pesos
		// abaixo somam exatamente 1 e podem compor o score.
		WeightAnomalousAccel: 0.20187852222918,
		WeightSharpTurn:      0.0245460237946149,
		WeightFatigue:        0.773575453976205,
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
	return c.createRiskAssessment(ctx, tripID, readingsJSON)
}

// CreateRiskAssessmentCompressed recebe o mesmo JSON de leituras comprimido
// com gzip e codificado em base64. É útil para trajetos grandes enviados por
// linha de comando, pois reduz o tamanho do argumento sem mudar a equação.
func (c *RiskContract) CreateRiskAssessmentCompressed(
	ctx contractapi.TransactionContextInterface,
	tripID string,
	compressedReadings string,
) (*RiskAssessment, error) {
	readingsJSON, err := decompressReadings(compressedReadings)
	if err != nil {
		return nil, err
	}
	return c.createRiskAssessment(ctx, tripID, readingsJSON)
}

// AddReading recebe uma única telemetria OBD. Todos os parâmetros da leitura
// chegam como texto, compatível com dispositivos que transmitem campos CSV ou
// strings, mas são validados antes de serem persistidos.
func (c *RiskContract) AddReading(
	ctx contractapi.TransactionContextInterface,
	tripID string,
	timestamp string,
	latitude string,
	longitude string,
	vehicleSpeed string,
) (*TripBuffer, error) {
	if tripID == "" {
		return nil, errors.New("tripID é obrigatório")
	}
	reading, err := parseTextReading(timestamp, latitude, longitude, vehicleSpeed)
	if err != nil {
		return nil, err
	}

	bufferKey, err := ctx.GetStub().CreateCompositeKey("riskTrip", []string{tripID})
	if err != nil {
		return nil, fmt.Errorf("erro ao criar chave do trajeto: %w", err)
	}
	buffer, err := readTripBuffer(ctx, bufferKey, tripID)
	if err != nil {
		return nil, err
	}
	if buffer.Finalized {
		return nil, fmt.Errorf("o trajeto %q já foi finalizado", tripID)
	}
	if buffer.ReadingCount > 0 && !reading.Timestamp.After(buffer.LastTimestamp) {
		return nil, errors.New("a leitura deve possuir timestamp posterior ao da leitura anterior")
	}

	sequence := buffer.ReadingCount + 1
	readingKey, err := ctx.GetStub().CreateCompositeKey("riskReading", []string{tripID, fmt.Sprintf("%010d", sequence)})
	if err != nil {
		return nil, fmt.Errorf("erro ao criar chave da leitura: %w", err)
	}
	payload, err := json.Marshal(BufferedReading{
		Sequence:     sequence,
		Timestamp:    timestamp,
		Latitude:     latitude,
		Longitude:    longitude,
		VehicleSpeed: vehicleSpeed,
	})
	if err != nil {
		return nil, fmt.Errorf("erro ao serializar leitura: %w", err)
	}
	if err := ctx.GetStub().PutState(readingKey, payload); err != nil {
		return nil, fmt.Errorf("erro ao gravar leitura: %w", err)
	}

	buffer.ReadingCount = sequence
	buffer.LastTimestamp = reading.Timestamp
	if err := writeTripBuffer(ctx, bufferKey, buffer); err != nil {
		return nil, err
	}
	return buffer, nil
}

// FinalizeTrip recupera as leituras adicionadas por AddReading, calcula o
// risco uma única vez e torna o trajeto somente-leitura.
func (c *RiskContract) FinalizeTrip(
	ctx contractapi.TransactionContextInterface,
	tripID string,
) (*RiskAssessment, error) {
	if tripID == "" {
		return nil, errors.New("tripID é obrigatório")
	}
	assessmentKey, err := ctx.GetStub().CreateCompositeKey("riskAssessment", []string{tripID})
	if err != nil {
		return nil, fmt.Errorf("erro ao criar chave da avaliação: %w", err)
	}
	existing, err := ctx.GetStub().GetState(assessmentKey)
	if err != nil {
		return nil, fmt.Errorf("erro ao consultar avaliação existente: %w", err)
	}
	if existing != nil {
		return nil, fmt.Errorf("já existe avaliação para o trajeto %q", tripID)
	}

	bufferKey, err := ctx.GetStub().CreateCompositeKey("riskTrip", []string{tripID})
	if err != nil {
		return nil, fmt.Errorf("erro ao criar chave do trajeto: %w", err)
	}
	buffer, err := readTripBuffer(ctx, bufferKey, tripID)
	if err != nil {
		return nil, err
	}
	if buffer.Finalized {
		return nil, fmt.Errorf("o trajeto %q já foi finalizado", tripID)
	}
	if buffer.ReadingCount < 2 {
		return nil, errors.New("são necessárias ao menos 2 leituras para finalizar o trajeto")
	}

	readings, err := readBufferedReadings(ctx, tripID)
	if err != nil {
		return nil, err
	}
	if len(readings) != buffer.ReadingCount {
		return nil, errors.New("a quantidade de leituras armazenadas não confere com o trajeto")
	}
	assessment, err := calculateAssessment(tripID, readings, defaultCalibration())
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(assessment)
	if err != nil {
		return nil, fmt.Errorf("erro ao serializar avaliação: %w", err)
	}
	if err := ctx.GetStub().PutState(assessmentKey, payload); err != nil {
		return nil, fmt.Errorf("erro ao gravar avaliação: %w", err)
	}

	buffer.Finalized = true
	if err := writeTripBuffer(ctx, bufferKey, buffer); err != nil {
		return nil, err
	}
	return assessment, nil
}

// ReadTripBuffer permite acompanhar a quantidade de leituras recebidas antes
// de finalizar um trajeto.
func (c *RiskContract) ReadTripBuffer(
	ctx contractapi.TransactionContextInterface,
	tripID string,
) (*TripBuffer, error) {
	bufferKey, err := ctx.GetStub().CreateCompositeKey("riskTrip", []string{tripID})
	if err != nil {
		return nil, fmt.Errorf("erro ao criar chave do trajeto: %w", err)
	}
	return readTripBuffer(ctx, bufferKey, tripID)
}

func (c *RiskContract) createRiskAssessment(
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

func readTripBuffer(ctx contractapi.TransactionContextInterface, key, tripID string) (*TripBuffer, error) {
	payload, err := ctx.GetStub().GetState(key)
	if err != nil {
		return nil, fmt.Errorf("erro ao consultar trajeto: %w", err)
	}
	if payload == nil {
		return &TripBuffer{TripID: tripID}, nil
	}
	var buffer TripBuffer
	if err := json.Unmarshal(payload, &buffer); err != nil {
		return nil, fmt.Errorf("trajeto armazenado inválido: %w", err)
	}
	return &buffer, nil
}

func writeTripBuffer(ctx contractapi.TransactionContextInterface, key string, buffer *TripBuffer) error {
	payload, err := json.Marshal(buffer)
	if err != nil {
		return fmt.Errorf("erro ao serializar trajeto: %w", err)
	}
	if err := ctx.GetStub().PutState(key, payload); err != nil {
		return fmt.Errorf("erro ao gravar trajeto: %w", err)
	}
	return nil
}

func readBufferedReadings(ctx contractapi.TransactionContextInterface, tripID string) ([]Reading, error) {
	iterator, err := ctx.GetStub().GetStateByPartialCompositeKey("riskReading", []string{tripID})
	if err != nil {
		return nil, fmt.Errorf("erro ao consultar leituras do trajeto: %w", err)
	}
	defer iterator.Close()

	var readings []Reading
	for iterator.HasNext() {
		entry, err := iterator.Next()
		if err != nil {
			return nil, fmt.Errorf("erro ao percorrer leituras do trajeto: %w", err)
		}
		var buffered BufferedReading
		if err := json.Unmarshal(entry.Value, &buffered); err != nil {
			return nil, fmt.Errorf("leitura armazenada inválida: %w", err)
		}
		reading, err := parseTextReading(buffered.Timestamp, buffered.Latitude, buffered.Longitude, buffered.VehicleSpeed)
		if err != nil {
			return nil, fmt.Errorf("leitura %d inválida: %w", buffered.Sequence, err)
		}
		readings = append(readings, reading)
	}
	if err := validateReadings(readings); err != nil {
		return nil, err
	}
	return readings, nil
}

func decompressReadings(compressedReadings string) (string, error) {
	compressed, err := base64.StdEncoding.DecodeString(compressedReadings)
	if err != nil {
		return "", fmt.Errorf("leituras compactadas inválidas: %w", err)
	}

	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return "", fmt.Errorf("não foi possível descompactar leituras: %w", err)
	}
	defer reader.Close()

	// Limite defensivo: impede que um pequeno arquivo comprimido gere uma carga
	// excessiva no container do chaincode.
	const maxUncompressedBytes = 10 * 1024 * 1024
	jsonBytes, err := io.ReadAll(io.LimitReader(reader, maxUncompressedBytes+1))
	if err != nil {
		return "", fmt.Errorf("erro ao ler leituras descompactadas: %w", err)
	}
	if len(jsonBytes) > maxUncompressedBytes {
		return "", errors.New("leituras descompactadas excedem o limite de 10 MiB")
	}
	return string(jsonBytes), nil
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
	// Ā_i e D̄_i são taxas de ocorrências por minuto do trajeto,
	// limitadas a 1 para preservar a escala normalizada do modelo.
	accelerationMetric := math.Min(1, float64(accelerationCount)/tripMinutes)
	turnMetric := math.Min(1, float64(turnCount)/tripMinutes)
	fatigue := analyzeFatigue(readings, calibration)

	// O resultado é um escore relativo, não uma probabilidade de acidente.
	riskFactor := calibration.WeightAnomalousAccel*accelerationMetric +
		calibration.WeightSharpTurn*turnMetric +
		calibration.WeightFatigue*fatigue.Metric
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
		RiskFactor:          riskFactor,
		Calibration:         calibration,
	}, nil
}

func parseAndValidateReadings(readingsJSON string) ([]Reading, error) {
	var readings []Reading
	if err := json.Unmarshal([]byte(readingsJSON), &readings); err != nil {
		return nil, fmt.Errorf("readingsJSON inválido: %w", err)
	}
	if err := validateReadings(readings); err != nil {
		return nil, err
	}
	return readings, nil
}

func parseTextReading(timestampText, latitudeText, longitudeText, speedText string) (Reading, error) {
	const localLayout = "2006-01-02 15:04:05.000"
	location := time.FixedZone("America/Sao_Paulo", -3*60*60)

	timestampText = strings.TrimSpace(timestampText)
	var timestamp time.Time
	var err error
	for _, layout := range []string{time.RFC3339Nano, localLayout, "2006-01-02 15:04:05"} {
		if layout == time.RFC3339Nano {
			timestamp, err = time.Parse(layout, timestampText)
		} else {
			timestamp, err = time.ParseInLocation(layout, timestampText, location)
		}
		if err == nil {
			break
		}
	}
	if err != nil {
		return Reading{}, fmt.Errorf("timestamp inválido: %q", timestampText)
	}
	latitude, err := parseTextFloat(latitudeText, "latitude")
	if err != nil {
		return Reading{}, err
	}
	longitude, err := parseTextFloat(longitudeText, "longitude")
	if err != nil {
		return Reading{}, err
	}
	speed, err := parseTextFloat(speedText, "velocidade")
	if err != nil {
		return Reading{}, err
	}
	reading := Reading{Timestamp: timestamp, Lat: latitude, Lon: longitude, SpeedKmh: speed}
	if err := validateReading(reading); err != nil {
		return Reading{}, err
	}
	return reading, nil
}

func parseTextFloat(value, field string) (float64, error) {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return 0, fmt.Errorf("%s inválida: %q", field, value)
	}
	return parsed, nil
}

func validateReadings(readings []Reading) error {
	if len(readings) < 2 {
		return errors.New("são necessárias ao menos 2 leituras")
	}
	for index, reading := range readings {
		if err := validateReading(reading); err != nil {
			return fmt.Errorf("leitura %d: %w", index, err)
		}
		if index > 0 && !reading.Timestamp.After(readings[index-1].Timestamp) {
			return fmt.Errorf("timestamps devem estar em ordem crescente: leitura %d", index)
		}
	}
	return nil
}

func validateReading(reading Reading) error {
	if reading.Timestamp.IsZero() {
		return errors.New("não possui timestamp")
	}
	if reading.SpeedKmh < 0 || math.IsNaN(reading.SpeedKmh) || math.IsInf(reading.SpeedKmh, 0) {
		return errors.New("velocidade inválida")
	}
	if reading.Lat < -90 || reading.Lat > 90 || reading.Lon < -180 || reading.Lon > 180 {
		return errors.New("coordenada inválida")
	}
	return nil
}

func validateCalibration(calibration Calibration) error {
	if calibration.FatigueThresholdMinutes <= 0 || calibration.MinValidPauseMinutes <= 0 {
		return errors.New("os limiares de duração devem ser positivos")
	}
	if calibration.AnomalousSpeedChangeThresholdKmh <= 0 || calibration.MaxSampleGapSeconds <= 0 ||
		calibration.SharpTurnAngleThresholdRad <= 0 ||
		calibration.SharpTurnSpeedThresholdKmh < 0 ||
		calibration.StoppedSpeedThreshold < 0 {
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
	inAnomalousEvent := false
	lastDirection := 0

	for i := 1; i < len(readings); i++ {
		foundAnomalousChange := false
		direction := 0

		// A leitura j é comparada a referências anteriores k dentro da
		// janela de até 10 s. Assim, 50 -> 80 km/h em 10 s é identificado
		// mesmo quando há leituras intermediárias entre os dois pontos.
		for k := i - 1; k >= 0; k-- {
			deltaSeconds := readings[i].Timestamp.Sub(readings[k].Timestamp).Seconds()
			if deltaSeconds <= 0 {
				continue
			}
			if deltaSeconds > float64(calibration.MaxSampleGapSeconds) {
				break
			}

			deltaSpeed := readings[i].SpeedKmh - readings[k].SpeedKmh
			// a(i,j,k) = (v(i,j) - v(i,k)) / (t(i,j) - t(i,k)).
			// O evento ocorre quando |Delta v| e de pelo menos 30 km/h.
			acceleration := deltaSpeed / deltaSeconds
			if math.Abs(acceleration*deltaSeconds) >= calibration.AnomalousSpeedChangeThresholdKmh {
				foundAnomalousChange = true
				if deltaSpeed > 0 {
					direction = 1
				} else {
					direction = -1
				}
				break
			}
		}

		// Várias leituras do mesmo aumento/redução de velocidade equivalem
		// a uma única ocorrência. Uma mudança de sentido inicia outra.
		if foundAnomalousChange {
			if !inAnomalousEvent || direction != lastDirection {
				count++
			}
			inAnomalousEvent = true
			lastDirection = direction
		} else {
			inAnomalousEvent = false
			lastDirection = 0
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
	inSharpTurnEvent := false
	for i := 2; i < len(readings); i++ {
		if !readings[i].Timestamp.After(readings[i-1].Timestamp) {
			inSharpTurnEvent = false
			continue
		}
		turnAngle := angleDifference(bearings[i], bearings[i-1])
		isSharpTurn := turnAngle > calibration.SharpTurnAngleThresholdRad &&
			readings[i].SpeedKmh >= calibration.SharpTurnSpeedThresholdKmh
		if isSharpTurn && !inSharpTurnEvent {
			count++
		}
		inSharpTurnEvent = isSharpTurn
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

	var totalDuration, totalExcess, longestPeriod time.Duration
	for _, period := range periods {
		totalDuration += period
		if period > longestPeriod {
			longestPeriod = period
		}
		if period > fatigueLimit {
			totalExcess += period - fatigueLimit
		}
	}
	if totalDuration == 0 {
		return FatigueMetrics{}
	}

	metrics := FatigueMetrics{
		Metric:                   float64(totalExcess) / float64(totalDuration),
		TotalDrivingMinutes:      totalDuration.Minutes(),
		LongestContinuousMinutes: longestPeriod.Minutes(),
		ExcessMinutes:            totalExcess.Minutes(),
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
		log.Panicf("erro ao criar chaincode: %v", err)
	}

	// Este servidor substitui chaincode.Start(): em Chaincode as a Service,
	// o peer Fabric conecta-se ao contêiner usando estas variáveis de ambiente.
	server := &shim.ChaincodeServer{
		CCID:     os.Getenv("CHAINCODE_ID"),
		Address:  os.Getenv("CHAINCODE_SERVER_ADDRESS"),
		CC:       chaincode,
		TLSProps: getTLSProperties(),
	}
	if server.CCID == "" || server.Address == "" {
		log.Panic("CHAINCODE_ID e CHAINCODE_SERVER_ADDRESS são obrigatórios")
	}
	if err := server.Start(); err != nil {
		log.Panicf("erro ao iniciar servidor do chaincode: %v", err)
	}
}

// getTLSProperties permite habilitar TLS depois, sem modificar o código do
// contrato. Para a rede local de teste, CHAINCODE_TLS_DISABLED=true é usado.
func getTLSProperties() shim.TLSProperties {
	tlsDisabled := getBoolOrDefault(os.Getenv("CHAINCODE_TLS_DISABLED"), true)
	properties := shim.TLSProperties{Disabled: tlsDisabled}
	if tlsDisabled {
		return properties
	}

	var err error
	if properties.Key, err = os.ReadFile(os.Getenv("CHAINCODE_TLS_KEY")); err != nil {
		log.Panicf("erro ao ler chave TLS: %v", err)
	}
	if properties.Cert, err = os.ReadFile(os.Getenv("CHAINCODE_TLS_CERT")); err != nil {
		log.Panicf("erro ao ler certificado TLS: %v", err)
	}
	if clientCA := os.Getenv("CHAINCODE_CLIENT_CA_CERT"); clientCA != "" {
		if properties.ClientCACerts, err = os.ReadFile(clientCA); err != nil {
			log.Panicf("erro ao ler CA do cliente TLS: %v", err)
		}
	}
	return properties
}

func getBoolOrDefault(value string, defaultValue bool) bool {
	if value == "" {
		return defaultValue
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return defaultValue
	}
	return parsed
}
