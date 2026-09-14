// Programa cliente para testar o RiskContract a partir de um CSV OBD.
//
// Execute este arquivo na VM, e não o main.go da pasta chaincode-risk-factor:
//
//	cd ~/driver-behavior-monetization/client/risk-factor
//	go run main.go -trip-id obd-15-spin-trajeto-t2
//
// O cliente lê o CSV, mostra cada leitura no terminal e, ao final, envia uma
// única transação à blockchain. Assim o resultado representa o trajeto inteiro
// sem criar uma transação para cada uma das milhares de leituras.
package main

import (
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

func main() {
	inputPath := flag.String("input", "../../data/obd_clean.csv", "caminho do CSV OBD")
	routeID := flag.String("route", "obd-15-spin-trajeto-t1", "valor de id_route a processar")
	tripID := flag.String("trip-id", "", "identificador novo e único do trajeto no ledger")
	configPath := flag.String("config", "../../resources/inmetro.yaml", "arquivo de configuração da rede")
	verbose := flag.Bool("verbose", true, "mostrar cada leitura do CSV no terminal")
	flag.Parse()

	if *tripID == "" {
		fatal("informe -trip-id, por exemplo: -trip-id obd-15-spin-trajeto-t2")
	}

	readings := readCSV(*inputPath, *routeID, *verbose)
	validateTimeline(readings)

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
	fmt.Printf("Enviando uma única avaliação do trajeto à blockchain...\n\n")
	invoke(*configPath, *tripID, compressed)
}

// readCSV converte apenas os campos necessários para as métricas do contrato.
func readCSV(path, routeID string, verbose bool) []Reading {
	file, err := os.Open(path)
	if err != nil {
		fatal("não foi possível abrir o CSV %q: %v", path, err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	header, err := reader.Read()
	if err != nil {
		fatal("não foi possível ler o cabeçalho: %v", err)
	}
	columns := indexes(header)
	for _, column := range []string{"timestamp", "lat", "lon", "vehicle_speed", "id_route"} {
		if _, ok := columns[column]; !ok {
			fatal("coluna obrigatória ausente: %s", column)
		}
	}

	var readings []Reading
	for line := 2; ; line++ {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			fatal("erro na linha %d do CSV: %v", line, err)
		}
		if routeID != "" && value(record, columns, "id_route") != routeID {
			continue
		}

		reading := Reading{
			Timestamp: parseTimestamp(value(record, columns, "timestamp"), line),
			Lat:       parseFloat(value(record, columns, "lat"), "latitude", line),
			Lon:       parseFloat(value(record, columns, "lon"), "longitude", line),
			SpeedKmh:  parseFloat(value(record, columns, "vehicle_speed"), "velocidade", line),
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

func invoke(configPath, tripID, compressedReadings string) {
	configPath, err := filepath.Abs(configPath)
	if err != nil {
		fatal("não foi possível localizar o arquivo de configuração: %v", err)
	}

	// Usa o mesmo acesso já validado com `kubectl hlf` na VM.
	command := exec.Command("kubectl", "hlf", "chaincode", "invoke",
		"--config="+configPath,
		"--user=inmetro-admin-default",
		"--peer=inmetro-peer0.default",
		"--channel=demo",
		"--chaincode=risk-factor",
		"--fcn=CreateRiskAssessmentCompressed",
		"--args="+tripID,
		"--args="+compressedReadings,
	)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		fatal("a transação não foi concluída: %v", err)
	}
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
		result[strings.TrimSpace(name)] = index
	}
	return result
}

func value(record []string, columns map[string]int, name string) string {
	index := columns[name]
	if index >= len(record) {
		return ""
	}
	return strings.TrimSpace(record[index])
}

func parseFloat(text, field string, line int) float64 {
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
