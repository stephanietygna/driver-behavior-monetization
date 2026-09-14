// obd-to-readings converte um CSV OBD para o JSON aceito pelo RiskContract.
// Ele é um programa cliente: não faz parte do chaincode e não acessa o ledger.
package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

type reading struct {
	Timestamp time.Time `json:"timestamp"`
	Lat       float64   `json:"lat"`
	Lon       float64   `json:"lon"`
	SpeedKmh  float64   `json:"vehicleSpeed"`
}

func main() {
	inputPath := flag.String("input", "../data/obd_clean.csv", "caminho do CSV OBD")
	outputPath := flag.String("output", "trajeto.json", "arquivo JSON a gerar")
	routeID := flag.String("route", "", "valor opcional de id_route a converter")
	flag.Parse()

	input, err := os.Open(*inputPath)
	if err != nil {
		fatal("não foi possível abrir CSV: %v", err)
	}
	defer input.Close()

	reader := csv.NewReader(input)
	reader.FieldsPerRecord = -1
	header, err := reader.Read()
	if err != nil {
		fatal("não foi possível ler cabeçalho: %v", err)
	}
	columns := indexes(header)
	for _, name := range []string{"timestamp", "lat", "lon", "vehicle_speed"} {
		if _, ok := columns[name]; !ok {
			fatal("coluna obrigatória ausente: %s", name)
		}
	}

	var readings []reading
	for rowNumber := 2; ; rowNumber++ {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			fatal("erro na linha %d: %v", rowNumber, err)
		}
		if *routeID != "" && value(record, columns, "id_route") != *routeID {
			continue
		}

		timestamp, err := parseTimestamp(value(record, columns, "timestamp"))
		if err != nil {
			fatal("timestamp inválido na linha %d: %v", rowNumber, err)
		}
		lat, err := parseFloat(value(record, columns, "lat"))
		if err != nil {
			fatal("latitude inválida na linha %d: %v", rowNumber, err)
		}
		lon, err := parseFloat(value(record, columns, "lon"))
		if err != nil {
			fatal("longitude inválida na linha %d: %v", rowNumber, err)
		}
		speed, err := parseFloat(value(record, columns, "vehicle_speed"))
		if err != nil {
			fatal("velocidade inválida na linha %d: %v", rowNumber, err)
		}
		readings = append(readings, reading{Timestamp: timestamp, Lat: lat, Lon: lon, SpeedKmh: speed})
	}

	if len(readings) < 2 {
		fatal("a conversão precisa produzir ao menos duas leituras")
	}
	for i := 1; i < len(readings); i++ {
		if !readings[i].Timestamp.After(readings[i-1].Timestamp) {
			fatal("timestamps fora de ordem ou duplicados entre as leituras %d e %d", i, i+1)
		}
	}

	output, err := json.Marshal(readings)
	if err != nil {
		fatal("não foi possível criar JSON: %v", err)
	}
	if err := os.WriteFile(*outputPath, output, 0644); err != nil {
		fatal("não foi possível gravar JSON: %v", err)
	}

	fmt.Printf("%d leituras convertidas para %s\n", len(readings), *outputPath)
}

func indexes(header []string) map[string]int {
	result := make(map[string]int, len(header))
	for index, name := range header {
		result[strings.TrimSpace(name)] = index
	}
	return result
}

func value(record []string, columns map[string]int, name string) string {
	index, ok := columns[name]
	if !ok || index >= len(record) {
		return ""
	}
	return strings.TrimSpace(record[index])
}

func parseFloat(text string) (float64, error) {
	return strconv.ParseFloat(text, 64)
}

func parseTimestamp(text string) (time.Time, error) {
	location := time.FixedZone("America/Sao_Paulo", -3*60*60)
	for _, layout := range []string{"2006-01-02 15:04:05.000", "2006-01-02 15:04:05", time.RFC3339} {
		if timestamp, err := time.ParseInLocation(layout, text, location); err == nil {
			return timestamp, nil
		}
	}
	return time.Time{}, fmt.Errorf("formato não reconhecido: %q", text)
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
