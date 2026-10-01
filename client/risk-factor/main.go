// Programa cliente para testar o RiskContract a partir de um CSV OBD.
//
// Execute este arquivo na VM, e não o main.go da pasta chaincode-risk-factor:
//
//	cd ~/driver-behavior-monetization/client/risk-factor
//	go run main.go -trip-id obd-15-spin-trajeto-t2
//
// O cliente aceita dois formatos de entrada:
//   1. CSV padronizado: timestamp, lat, lon, vehicle_speed e id_route;
//   2. CSV Logger: Time (sec), Latitude (deg), Longitude (deg) e
//      Velocidade do veículo (km/h). Neste caso, o StartTime da primeira
//      linha é combinado com Time (sec) para formar cada timestamp.
package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Reading é o formato que o chaincode aceita. A velocidade é mantida em km/h.
type Reading struct {
	Timestamp time.Time `json:"timestamp"`
	Lat       float64   `json:"lat"`
	Lon       float64   `json:"lon"`
	SpeedKmh  float64   `json:"vehicleSpeed"`
}

// Assessment contém os resultados devolvidos pelo contrato após o cálculo.
// Estes campos já foram calculados e registrados no ledger pelo chaincode.
type Assessment struct {
	TripID              string  `json:"tripId"`
	ReadingCount        int     `json:"readingCount"`
	AnomalousAccelCount int     `json:"anomalousAccelCount"`
	SharpTurnCount      int     `json:"sharpTurnCount"`
	AccelerationMetric  float64 `json:"accelerationMetric"`
	TurnMetric          float64 `json:"turnMetric"`
	Fatigue             struct {
		Metric                   float64 `json:"metric"`
		TotalDrivingMinutes      float64 `json:"totalDrivingMinutes"`
		LongestContinuousMinutes float64 `json:"longestContinuousMinutes"`
		ExcessMinutes            float64 `json:"excessMinutes"`
	} `json:"fatigue"`
	Calibration struct {
		FatigueThresholdMinutes int64   `json:"fatigueThresholdMinutes"`
		WeightAnomalousAccel    float64 `json:"weightAnomalousAccel"`
		WeightSharpTurn         float64 `json:"weightSharpTurn"`
		WeightFatigue           float64 `json:"weightFatigue"`
	} `json:"calibration"`
	RiskFactor float64 `json:"riskFactor"`
}

func main() {
	inputPath := flag.String("input", "../../data/obd_clean.csv", "caminho do CSV OBD")
	routeID := flag.String("route", "obd-15-spin-trajeto-t1", "valor de id_route a processar no CSV padronizado")
	tripID := flag.String("trip-id", "", "identificador novo e único do trajeto no ledger")
	configPath := flag.String("config", "../../resources/inmetro.yaml", "arquivo de configuração da rede")
	verbose := flag.Bool("verbose", true, "mostrar cada leitura do CSV no terminal")
	stream := flag.Bool("stream", false, "enviar cada leitura como uma transação antes de finalizar o trajeto")
	flag.Parse()

	if *tripID == "" {
		fatal("informe -trip-id, por exemplo: -trip-id obd-15-spin-trajeto-t2")
	}

	readings := readCSV(*inputPath, *routeID, *verbose)
	validateTimeline(readings)
	if *stream {
		fmt.Printf("\nEnviando %d leituras individuais para o trajeto %q...\n", len(readings), *tripID)
		assessment := invokeStream(*configPath, *tripID, readings)
		printAssessment(assessment)
		return
	}

	// O JSON é compactado somente para caber no limite de argumentos do terminal.
	// O chaincode descompacta e calcula o mesmo R_i que calcularia com JSON puro.
	payload, err := json.Marshal(readings)
	if err != nil {
		fatal("não foi possível gerar JSON das leituras: %v", err)
	}
	compressed, err := gzipBase64(payload)
	if err != nil {
		fatal("não foi possível compactar leituras: %v", err)
	}

	fmt.Printf("\n%d leituras prontas para o trajeto %q.\n", len(readings), *tripID)
	fmt.Println("Enviando uma única avaliação do trajeto à blockchain...\n")
	assessment := invokeCompressed(*configPath, *tripID, compressed)
	printAssessment(assessment)
}

// readCSV aceita o CSV padronizado do projeto e o CSV do Logger.
func readCSV(path, routeID string, verbose bool) []Reading {
	file, err := os.Open(path)
	if err != nil {
		fatal("não foi possível abrir o CSV %q: %v", path, err)
	}
	defer file.Close()

	reader, header, startTime, firstDataLine := openCSV(file)
	columns := indexes(header)

	standardFormat := hasColumns(columns, "timestamp", "lat", "lon", "vehicle_speed", "id_route")
	loggerFormat := hasColumns(columns, "time (sec)", "latitude (deg)", "longitude (deg)", "velocidade do veículo (km/h)")
	if !standardFormat && !loggerFormat {
		fatal("formato de CSV não reconhecido: use timestamp/lat/lon/vehicle_speed/id_route ou Time (sec)/Latitude (deg)/Longitude (deg)/Velocidade do veículo (km/h)")
	}
	if loggerFormat && startTime.IsZero() {
		fatal("CSV Logger sem StartTime na primeira linha")
	}

	var readings []Reading
	for line := firstDataLine; ; line++ {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			fatal("erro na linha %d do CSV: %v", line, err)
		}

		var reading Reading
		if standardFormat {
			if routeID != "" && value(record, columns, "id_route") != routeID {
				continue
			}
			reading = Reading{
				Timestamp: parseTimestamp(value(record, columns, "timestamp"), line),
				Lat:       parseFloat(value(record, columns, "lat"), "latitude", line),
				Lon:       parseFloat(value(record, columns, "lon"), "longitude", line),
				SpeedKmh:  parseFloat(value(record, columns, "vehicle_speed"), "velocidade", line),
			}
		} else {
			seconds := parseFloat(value(record, columns, "time (sec)"), "tempo", line)
			reading = Reading{
				Timestamp: startTime.Add(time.Duration(seconds * float64(time.Second))),
				Lat:       parseFloat(value(record, columns, "latitude (deg)"), "latitude", line),
				Lon:       parseFloat(value(record, columns, "longitude (deg)"), "longitude", line),
				SpeedKmh:  parseFloat(value(record, columns, "velocidade do veículo (km/h)"), "velocidade", line),
			}
		}

		readings = append(readings, reading)
		if verbose {
			fmt.Printf("Leitura %d | %s | lat %.6f | lon %.6f | velocidade %.2f km/h\n",
				len(readings), reading.Timestamp.Format(time.RFC3339Nano), reading.Lat, reading.Lon, reading.SpeedKmh)
		}
	}
	if len(readings) < 2 {
		fatal("o trajeto precisa conter ao menos duas leituras")
	}
	return readings
}

// openCSV identifica o delimitador e, nos arquivos Logger, lê o StartTime.
func openCSV(file *os.File) (*csv.Reader, []string, time.Time, int) {
	buffered := bufio.NewReader(file)
	firstLine, err := buffered.ReadString('\n')
	if err != nil && err != io.EOF {
		fatal("não foi possível ler o CSV: %v", err)
	}
	if firstLine == "" {
		fatal("CSV vazio")
	}

	firstLine = strings.TrimPrefix(firstLine, "\ufeff")
	startTime := time.Time{}
	headerLine := firstLine
	firstDataLine := 2
	if strings.HasPrefix(strings.TrimSpace(firstLine), "#") {
		startTime = parseLoggerStartTime(firstLine)
		headerLine, err = buffered.ReadString('\n')
		if err != nil && err != io.EOF {
			fatal("não foi possível ler o cabeçalho: %v", err)
		}
		if headerLine == "" {
			fatal("CSV sem cabeçalho")
		}
		firstDataLine = 3
	}

	separator := detectSeparator(headerLine)
	headerReader := csv.NewReader(strings.NewReader(headerLine))
	headerReader.Comma = separator
	headerReader.FieldsPerRecord = -1
	header, err := headerReader.Read()
	if err != nil {
		fatal("não foi possível ler o cabeçalho: %v", err)
	}

	reader := csv.NewReader(buffered)
	reader.Comma = separator
	reader.FieldsPerRecord = -1
	return reader, header, startTime, firstDataLine
}

func parseLoggerStartTime(line string) time.Time {
	parts := strings.SplitN(line, "=", 2)
	if len(parts) != 2 {
		fatal("StartTime inválido: %q", strings.TrimSpace(line))
	}
	text := strings.TrimSpace(parts[1])
	location := time.FixedZone("America/Sao_Paulo", -3*60*60)
	for _, layout := range []string{"01/02/2006 03:04:05.0000 PM", "01/02/2006 03:04:05 PM"} {
		if timestamp, err := time.ParseInLocation(layout, text, location); err == nil {
			return timestamp
		}
	}
	fatal("StartTime inválido: %q", text)
	return time.Time{}
}

func detectSeparator(header string) rune {
	if strings.Count(header, ";") > strings.Count(header, ",") {
		return ';'
	}
	return ','
}

func hasColumns(columns map[string]int, required ...string) bool {
	for _, column := range required {
		if _, ok := columns[normalizeColumnName(column)]; !ok {
			return false
		}
	}
	return true
}

func invokeCompressed(configPath, tripID, compressedReadings string) Assessment {
	output := invokeOutput(configPath, "CreateRiskAssessmentCompressed", tripID, compressedReadings)
	return parseAssessment(output)
}

// invokeStream reproduz um fluxo OBD: cada leitura é registrada como texto no
// ledger; ao fim, FinalizeTrip calcula as métricas sobre o conjunto completo.
func invokeStream(configPath, tripID string, readings []Reading) Assessment {
	for index, reading := range readings {
		invokeOutput(
			configPath,
			"AddReading",
			tripID,
			reading.Timestamp.Format(time.RFC3339Nano),
			strconv.FormatFloat(reading.Lat, 'f', -1, 64),
			strconv.FormatFloat(reading.Lon, 'f', -1, 64),
			strconv.FormatFloat(reading.SpeedKmh, 'f', -1, 64),
		)
		if (index+1)%100 == 0 || index+1 == len(readings) {
			fmt.Printf("   %d/%d leituras registradas\n", index+1, len(readings))
		}
	}
	fmt.Println("Finalizando o trajeto e calculando o fator de risco...\n")
	return parseAssessment(invokeOutput(configPath, "FinalizeTrip", tripID))
}

func invokeOutput(configPath, function string, arguments ...string) []byte {
	configPath, err := filepath.Abs(configPath)
	if err != nil {
		fatal("não foi possível localizar o arquivo de configuração: %v", err)
	}

	commandArguments := []string{"hlf", "chaincode", "invoke",
		"--config=" + configPath,
		"--user=inmetro-admin-default",
		"--peer=inmetro-peer0.default",
		"--channel=demo",
		"--chaincode=risk-factor",
		"--fcn=" + function,
	}
	for _, argument := range arguments {
		commandArguments = append(commandArguments, "--args="+argument)
	}
	command := exec.Command("kubectl", commandArguments...)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		fatal("a transação não foi concluída: %v\n%s", err, stderr.String())
	}
	return output
}

func parseAssessment(output []byte) Assessment {
	jsonOutput, err := firstJSONObject(output)
	if err != nil {
		fatal("a blockchain respondeu, mas o resultado não pôde ser lido: %v\n%s", err, output)
	}
	var assessment Assessment
	if err := json.Unmarshal(jsonOutput, &assessment); err != nil {
		fatal("resultado inválido recebido da blockchain: %v", err)
	}
	return assessment
}

// firstJSONObject isola o JSON do resultado, mesmo que o kubectl escreva
// mensagens informativas depois da transação.
func firstJSONObject(output []byte) ([]byte, error) {
	start := bytes.IndexByte(output, '{')
	if start == -1 {
		return nil, fmt.Errorf("nenhum JSON encontrado")
	}
	depth := 0
	for index := start; index < len(output); index++ {
		switch output[index] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return output[start : index+1], nil
			}
		}
	}
	return nil, fmt.Errorf("JSON incompleto")
}

// printAssessment organiza o retorno técnico do contrato em um resumo de fácil
// leitura para a análise do experimento.
func printAssessment(assessment Assessment) {
	fmt.Println("\n========================================")
	fmt.Println("        RESULTADO DO TRAJETO")
	fmt.Println("========================================")
	fmt.Printf("Trajeto: %s\n", assessment.TripID)
	fmt.Printf("Leituras analisadas: %d\n", assessment.ReadingCount)

	fmt.Println("\n1. ACELERAÇÕES ANÔMALAS")
	fmt.Printf("   Ocorrências: %d\n", assessment.AnomalousAccelCount)
	fmt.Printf("   Métrica normalizada (A_i): %.4f\n", assessment.AccelerationMetric)

	fmt.Println("\n2. CURVAS BRUSCAS")
	fmt.Printf("   Ocorrências: %d\n", assessment.SharpTurnCount)
	fmt.Printf("   Métrica normalizada (D_i): %.4f\n", assessment.TurnMetric)

	fmt.Println("\n3. FADIGA")
	fmt.Printf("   Tempo total de condução: %s\n", formatMinutes(assessment.Fatigue.TotalDrivingMinutes))
	fmt.Printf("   Maior período contínuo: %s\n", formatMinutes(assessment.Fatigue.LongestContinuousMinutes))
	fmt.Printf("   Limite configurado: %d min\n", assessment.Calibration.FatigueThresholdMinutes)
	fmt.Printf("   Excesso de fadiga: %s\n", formatMinutes(assessment.Fatigue.ExcessMinutes))
	fmt.Printf("   Métrica normalizada (M_T,i): %.4f\n", assessment.Fatigue.Metric)
	fmt.Println("   M_T,i representa a parcela do tempo total conduzida além do limiar.")

	accelerationContribution := assessment.Calibration.WeightAnomalousAccel * assessment.AccelerationMetric
	turnContribution := assessment.Calibration.WeightSharpTurn * assessment.TurnMetric
	fatigueContribution := assessment.Calibration.WeightFatigue * assessment.Fatigue.Metric

	fmt.Println("\n4. COMBINAÇÃO DAS MÉTRICAS")
	fmt.Printf("   Aceleração anômala: %.4f  (w_A = %.4f × A_i = %.4f)\n",
		accelerationContribution, assessment.Calibration.WeightAnomalousAccel, assessment.AccelerationMetric)
	fmt.Printf("   Curvas bruscas: %.4f      (w_D = %.4f × D_i = %.4f)\n",
		turnContribution, assessment.Calibration.WeightSharpTurn, assessment.TurnMetric)
	fmt.Printf("   Condução contínua: %.4f  (w_T = %.4f × M_T,i = %.4f)\n",
		fatigueContribution, assessment.Calibration.WeightFatigue, assessment.Fatigue.Metric)

	fmt.Println("\n5. FATOR DE RISCO FINAL")
	fmt.Printf("   Índice relativo de risco (R_i): %.4f (%.2f%% da escala do modelo)\n",
		assessment.RiskFactor, assessment.RiskFactor*100)
	fmt.Println("========================================")
}

func formatMinutes(minutes float64) string {
	totalSeconds := int64(minutes*60 + 0.5)
	return fmt.Sprintf("%d min e %02d s", totalSeconds/60, totalSeconds%60)
}

func gzipBase64(data []byte) (string, error) {
	var buffer bytes.Buffer
	writer := gzip.NewWriter(&buffer)
	if _, err := writer.Write(data); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buffer.Bytes()), nil
}

func validateTimeline(readings []Reading) {
	for i := 1; i < len(readings); i++ {
		if !readings[i].Timestamp.After(readings[i-1].Timestamp) {
			fatal("timestamps fora de ordem entre as leituras %d e %d", i, i+1)
		}
	}
}

func indexes(header []string) map[string]int {
	result := make(map[string]int, len(header))
	for index, name := range header {
		result[normalizeColumnName(name)] = index
	}
	return result
}

func normalizeColumnName(name string) string {
	return strings.ToLower(strings.TrimSpace(strings.TrimPrefix(name, "\ufeff")))
}

func value(record []string, columns map[string]int, name string) string {
	index := columns[normalizeColumnName(name)]
	if index >= len(record) {
		return ""
	}
	return strings.TrimSpace(record[index])
}

func parseFloat(text, field string, line int) float64 {
	text = strings.ReplaceAll(strings.TrimSpace(text), ",", ".")
	result, err := strconv.ParseFloat(text, 64)
	if err != nil {
		fatal("%s inválida na linha %d: %v", field, line, err)
	}
	return result
}

func parseTimestamp(text string, line int) time.Time {
	location := time.FixedZone("America/Sao_Paulo", -3*60*60)
	for _, layout := range []string{"2006-01-02 15:04:05.000", "2006-01-02 15:04:05", time.RFC3339} {
		if timestamp, err := time.ParseInLocation(layout, text, location); err == nil {
			return timestamp
		}
	}
	fatal("timestamp inválido na linha %d: %q", line, text)
	return time.Time{}
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
