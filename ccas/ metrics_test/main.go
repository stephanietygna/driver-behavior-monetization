// Programa standalone para testar as 3 métricas de comportamento de condução
// (aceleração anômala, curva brusca, fadiga por condução contínua) antes de
// integrar a lógica ao chaincode Fabric.
//
// Uso:
//
//	go run . -csv=dados.csv
//
// O CSV deve ter cabeçalho com (pelo menos) as colunas:
//
//	timestamp,lat,lon,vehicle_speed,accel_x
//
// - timestamp no formato "2006-01-02 15:04:05.000" (igual ao da sua planilha)
// - vehicle_speed em km/h
// - accel_x em m/s² (AJUSTE se a unidade real do seu sensor for diferente)
package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"math"
	"os"
	"strconv"
	"time"
)

// ============================================================================
// LIMIARES CONFIGURÁVEIS — ajuste aqui conforme os valores confirmados no estudo
// ============================================================================
const (
	// --- Métrica 1: Aceleração/desaceleração anômala ---
	// |Δa| > A  e  Δt <= T
	// A = 0.833 m/s² equivale a "variação de 30 km/h em 10s" (30/3.6/10).
	// TODO: confirmar se accel_x do seu sensor está mesmo em m/s².
	AnomalousAccelThreshold  = 0.833 // A, em m/s²
	AnomalousAccelMaxWindow  = 10 * time.Second // T

	// --- Métrica 2: Mudança brusca de direção ---
	// Δθ > θ_limite  e  v >= v_limite
	SharpTurnAngleThreshold = 0.7 // θ_limite, em radianos
	SharpTurnSpeedThreshold = 30.0 // v_limite, em km/h

	// --- Métrica 3: Fadiga por condução contínua ---
	// F(t) = 0 se t < T, 1 se t >= T ; T_i = floor(t/T)
	FatigueThreshold = 80 * time.Minute // T

	// Parâmetros do detector de pausa (usados para saber quando "zerar" t)
	// TODO: confirmar duração mínima de pausa válida com o orientador/estudo
	StoppedSpeedThreshold = 3.0            // km/h abaixo disso conta como "parado"
	MinValidPauseDuration = 5 * time.Minute // tempo parado mínimo para contar como pausa real
)

// Reading representa uma linha de telemetria do veículo.
type Reading struct {
	Timestamp time.Time
	Lat       float64
	Lon       float64
	SpeedKmh  float64
	AccelX    float64
}

// Event representa uma ocorrência detectada de uma métrica.
type Event struct {
	Timestamp time.Time
	Metric    string
	Detail    string
}

func main() {
	csvPath := flag.String("csv", "sample_data.csv", "caminho para o arquivo CSV de telemetria")
	flag.Parse()

	readings, err := loadReadings(*csvPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "erro ao carregar CSV: %v\n", err)
		os.Exit(1)
	}

	if len(readings) < 2 {
		fmt.Fprintf(os.Stderr, "dados insuficientes: são necessárias ao menos 2 leituras\n")
		os.Exit(1)
	}

	fmt.Printf("Carregadas %d leituras de %s\n", len(readings), *csvPath)
	fmt.Println("============================================================")

	var allEvents []Event

	allEvents = append(allEvents, DetectAnomalousAcceleration(readings)...)
	allEvents = append(allEvents, DetectSharpTurns(readings)...)
	allEvents = append(allEvents, DetectFatigue(readings)...)

	if len(allEvents) == 0 {
		fmt.Println("Nenhum evento de risco detectado com os limiares atuais.")
		return
	}

	fmt.Printf("%d evento(s) detectado(s):\n\n", len(allEvents))
	for _, e := range allEvents {
		fmt.Printf("[%s] %-14s %s\n", e.Timestamp.Format("2006-01-02 15:04:05.000"), e.Metric, e.Detail)
	}
}

// ============================================================================
// Métrica 1: Aceleração/desaceleração anômala
// Δa = a(tf) - a(ti) ; anômala se |Δa| > A e Δt <= T
// ============================================================================
func DetectAnomalousAcceleration(readings []Reading) []Event {
	var events []Event

	for i := 0; i < len(readings); i++ {
		for j := i + 1; j < len(readings); j++ {
			deltaT := readings[j].Timestamp.Sub(readings[i].Timestamp)
			if deltaT > AnomalousAccelMaxWindow {
				break // janela de tempo excedida, não adianta olhar mais à frente a partir de i
			}
			deltaA := readings[j].AccelX - readings[i].AccelX
			if math.Abs(deltaA) > AnomalousAccelThreshold {
				events = append(events, Event{
					Timestamp: readings[j].Timestamp,
					Metric:    "ACEL_ANOMALA",
					Detail: fmt.Sprintf("Δa=%.3f m/s² em Δt=%.1fs (de %.3f para %.3f)",
						deltaA, deltaT.Seconds(), readings[i].AccelX, readings[j].AccelX),
				})
			}
		}
	}

	return events
}

// ============================================================================
// Métrica 2: Mudança brusca de direção
// Δθ = |θ(tf) - θ(ti)| ; brusca se Δθ > θ_limite e v >= v_limite
// θ é calculado como o bearing (rumo) entre duas posições GPS consecutivas.
// ============================================================================
func DetectSharpTurns(readings []Reading) []Event {
	var events []Event

	if len(readings) < 3 {
		return events
	}

	// Calcula o bearing entre cada par consecutivo de pontos
	bearings := make([]float64, len(readings))
	bearings[0] = math.NaN() // não há bearing anterior para o primeiro ponto
	for i := 1; i < len(readings); i++ {
		bearings[i] = CalculateBearing(
			readings[i-1].Lat, readings[i-1].Lon,
			readings[i].Lat, readings[i].Lon,
		)
	}

	// Compara bearings consecutivos (Δθ), a partir do segundo bearing calculável
	for i := 2; i < len(readings); i++ {
		if math.IsNaN(bearings[i-1]) || math.IsNaN(bearings[i]) {
			continue
		}
		deltaTheta := angleDiff(bearings[i], bearings[i-1])
		if deltaTheta > SharpTurnAngleThreshold && readings[i].SpeedKmh >= SharpTurnSpeedThreshold {
			events = append(events, Event{
				Timestamp: readings[i].Timestamp,
				Metric:    "CURVA_BRUSCA",
				Detail: fmt.Sprintf("Δθ=%.3f rad, v=%.1f km/h", deltaTheta, readings[i].SpeedKmh),
			})
		}
	}

	return events
}

// CalculateBearing calcula a direção (rumo) entre dois pontos geográficos, em radianos (0 a 2π).
func CalculateBearing(lat1, lon1, lat2, lon2 float64) float64 {
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

// angleDiff retorna a menor diferença angular entre dois ângulos (0 a π), tratando o "wrap-around" de 0/2π.
func angleDiff(a, b float64) float64 {
	diff := math.Abs(a - b)
	if diff > math.Pi {
		diff = 2*math.Pi - diff
	}
	return diff
}

// ============================================================================
// Métrica 3: Fadiga por condução contínua
// F(t) = 0 se t < T, 1 se t >= T ; T_i = floor(t/T)
// Pausa válida: velocidade < StoppedSpeedThreshold por >= MinValidPauseDuration
// ============================================================================
func DetectFatigue(readings []Reading) []Event {
	var events []Event

	drivingStart := readings[0].Timestamp
	isStopped := false
	var stoppedSince time.Time

	lastReportedTi := 0 // evita reportar o mesmo T_i repetidamente a cada leitura

	for i := 0; i < len(readings); i++ {
		r := readings[i]

		if r.SpeedKmh > StoppedSpeedThreshold {
			// Carro em movimento
			if isStopped {
				stoppedDuration := r.Timestamp.Sub(stoppedSince)
				if stoppedDuration >= MinValidPauseDuration {
					// Pausa válida: reinicia o relógio de condução contínua
					drivingStart = r.Timestamp
					lastReportedTi = 0
				}
				isStopped = false
			}
		} else {
			// Carro parado (ou quase)
			if !isStopped {
				stoppedSince = r.Timestamp
				isStopped = true
			}
		}

		t := r.Timestamp.Sub(drivingStart)
		Ti := int(t / FatigueThreshold)

		if t >= FatigueThreshold && Ti > lastReportedTi {
			events = append(events, Event{
				Timestamp: r.Timestamp,
				Metric:    "FADIGA",
				Detail: fmt.Sprintf("condução contínua de %.1f min (violação nº %d do limite de %v)",
					t.Minutes(), Ti, FatigueThreshold),
			})
			lastReportedTi = Ti
		}
	}

	return events
}

// ============================================================================
// Leitura do CSV
// ============================================================================
func loadReadings(path string) ([]Reading, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	rows, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("falha ao ler CSV: %w", err)
	}
	if len(rows) < 2 {
		return nil, fmt.Errorf("CSV vazio ou sem dados")
	}

	header := rows[0]
	colIndex := map[string]int{}
	for i, name := range header {
		colIndex[name] = i
	}

	required := []string{"timestamp", "lat", "lon", "vehicle_speed", "accel_x"}
	for _, col := range required {
		if _, ok := colIndex[col]; !ok {
			return nil, fmt.Errorf("coluna obrigatória ausente no CSV: %s", col)
		}
	}

	var readings []Reading
	const layout = "2006-01-02 15:04:05.000"

	for lineNum, row := range rows[1:] {
		ts, err := time.Parse(layout, row[colIndex["timestamp"]])
		if err != nil {
			return nil, fmt.Errorf("linha %d: erro ao converter timestamp %q: %w", lineNum+2, row[colIndex["timestamp"]], err)
		}
		lat, err := strconv.ParseFloat(row[colIndex["lat"]], 64)
		if err != nil {
			return nil, fmt.Errorf("linha %d: erro ao converter lat: %w", lineNum+2, err)
		}
		lon, err := strconv.ParseFloat(row[colIndex["lon"]], 64)
		if err != nil {
			return nil, fmt.Errorf("linha %d: erro ao converter lon: %w", lineNum+2, err)
		}
		speed, err := strconv.ParseFloat(row[colIndex["vehicle_speed"]], 64)
		if err != nil {
			return nil, fmt.Errorf("linha %d: erro ao converter vehicle_speed: %w", lineNum+2, err)
		}
		accelX, err := strconv.ParseFloat(row[colIndex["accel_x"]], 64)
		if err != nil {
			return nil, fmt.Errorf("linha %d: erro ao converter accel_x: %w", lineNum+2, err)
		}

		readings = append(readings, Reading{
			Timestamp: ts,
			Lat:       lat,
			Lon:       lon,
			SpeedKmh:  speed,
			AccelX:    accelX,
		})
	}

	return readings, nil
}

