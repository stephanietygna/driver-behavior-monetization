package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/hyperledger/fabric-chaincode-go/shim"
	"github.com/hyperledger/fabric-contract-api-go/contractapi"
)

// TestRecord: representa a estrutura de dados do teste
type TestRecord struct {
	TestID                    string     `json:"test_id"`
	Timestamp                 string     `json:"timestamp"`
	Lat                       float64    `json:"lat"`
	Lon                       float64    `json:"lon"`
	GeoHash                   string     `json:"geo_hash"`
	OperatorID                string     `json:"operator_id"`
	OperatorDID               string     `json:"operator_did"`
	MatrixType                string     `json:"matrix_type"`
	CassetteLot               string     `json:"cassette_lot"`
	ReagentLot                string     `json:"reagent_lot"`
	ExpiryDaysLeft            int        `json:"expiry_days_left"`
	DistanceMM                float64    `json:"distance_mm"`
	TimeToMigrateS            float64    `json:"time_to_migrate_s"`
	ControlLineOK             bool       `json:"control_line_ok"`
	SampleVolumeUL            float64    `json:"sample_volume_uL"`
	SamplePH                  float64    `json:"sample_pH"`
	SampleTurbidityNTU        float64    `json:"sample_turbidity_NTU"`
	SampleTempC               float64    `json:"sample_temp_C"`
	AmbientTC                 float64    `json:"ambient_T_C"`
	AmbientRHPct              float64    `json:"ambient_RH_pct"`
	LightingLux               float64    `json:"lighting_lux"`
	TiltDeg                   float64    `json:"tilt_deg"`
	PreincubationTimeS        float64    `json:"preincubation_time_s"`
	TimeSinceSamplingMin      float64    `json:"time_since_sampling_min"`
	StorageCondition          string     `json:"storage_condition"`
	PrefilterUsed             bool       `json:"prefilter_used"`
	ImageTaken                bool       `json:"image_taken"`
	ImageBlurScore            float64    `json:"image_blur_score"`
	DeviceFWVersion           string     `json:"device_fw_version"`
	ProdutoID                 string     `json:"produto_id"`
	KitCalibrationID          string     `json:"kit_calibration_id"`
	ControleInternoResult     string     `json:"controle_interno_result"`
	CadeiaFrioStatus          bool       `json:"cadeia_frio_status"`
	TempoTransporteHoras      float64    `json:"tempo_transporte_horas"`
	CondicaoTransporte        string     `json:"condicao_transporte"`
	EstimatedConcentrationPpb float64  `json:"estimated_concentration_ppb"`
	IncertezaEstimativaPpb    float64    `json:"incerteza_estimativa_ppb"`
	AcaoRecomendada           string     `json:"acao_recomendada"`
	ResultClass               string     `json:"result_class"`
	QCStatus                  string     `json:"qc_status"`
}

type SmartContract struct {
	contractapi.Contract
}

type serverConfig struct {
	CCID    string
	Address string
}

func (s *SmartContract) StoreTest(ctx contractapi.TransactionContextInterface, testID string, jsonStr string) error {
	var record TestRecord
	err := json.Unmarshal([]byte(jsonStr), &record)
	if err != nil {
		return fmt.Errorf("error: %s", err)
	}

	recordBytes, err := json.Marshal(record)
	if err!=nil{
		return fmt.Errorf("error: %s", err)
	}

	err = ctx.GetStub().PutState(testID, recordBytes)
	if err!=nil{
		return fmt.Errorf("error: %s", err)
	}

	fmt.Println("Teste ID: ", testID, " registrado com sucesso")
	return nil
}

// QueryTest recupera um registro de teste pelo ID
func (s *SmartContract) QueryTest(ctx contractapi.TransactionContextInterface, testID string) (*TestRecord, error) {
	// Obter o estado do ledger para a chave fornecida (testID)
	result, err := ctx.GetStub().GetState(testID)
	if err != nil {
		return nil, fmt.Errorf("error while getting state: %s", err)
	}

	// Verifica se a chave existe no ledger
	if result == nil {
		return nil, fmt.Errorf("no record found for testID: %s", testID)
	}

	// Decodifica os dados armazenados em JSON para a struct TestRecord
	var record TestRecord
	err = json.Unmarshal(result, &record)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling data: %s", err)
	}

	// Retorna o registro
	return &record, nil
}

// GetAllTests retorna todos os registros armazenados
func (s *SmartContract) GetAllTests(ctx contractapi.TransactionContextInterface) ([]TestRecord, error) {
    resultsIterator, err := ctx.GetStub().GetStateByRange("", "")
    if err != nil {
        return nil, err
    }
    defer resultsIterator.Close()

    var records []TestRecord
    for resultsIterator.HasNext() {
        queryResponse, err := resultsIterator.Next()
        if err != nil {
            return nil, err
        }

        var record TestRecord
        err = json.Unmarshal(queryResponse.Value, &record)
        if err != nil {
            return nil, err
        }
        records = append(records, record) // sem ponteiro
    }

    return records, nil
}

func main() {
	// See chaincode.env.example
	config := serverConfig{
		CCID:    os.Getenv("CHAINCODE_ID"),
		Address: os.Getenv("CHAINCODE_SERVER_ADDRESS"),
	}

	chaincode, err := contractapi.NewChaincode(&SmartContract{})

	if err != nil {
		log.Panicf("error create chaincode: %s", err)
	}

	server := &shim.ChaincodeServer{
		CCID:     config.CCID,
		Address:  config.Address,
		CC:       chaincode,
		TLSProps: getTLSProperties(),
	}

	if err := server.Start(); err != nil {
		log.Panicf("error starting chaincode: %s", err)
	}
}

func getTLSProperties() shim.TLSProperties {
	// Check if chaincode is TLS enabled
	tlsDisabledStr := getEnvOrDefault("CHAINCODE_TLS_DISABLED", "true")
	key := getEnvOrDefault("CHAINCODE_TLS_KEY", "")
	cert := getEnvOrDefault("CHAINCODE_TLS_CERT", "")
	clientCACert := getEnvOrDefault("CHAINCODE_CLIENT_CA_CERT", "")

	// convert tlsDisabledStr to boolean
	tlsDisabled := getBoolOrDefault(tlsDisabledStr, false)
	var keyBytes, certBytes, clientCACertBytes []byte
	var err error

	if !tlsDisabled {
		keyBytes, err = os.ReadFile(key)
		if err != nil {
			log.Panicf("error while reading the crypto file: %s", err)
		}
		certBytes, err = os.ReadFile(cert)
		if err != nil {
			log.Panicf("error while reading the crypto file: %s", err)
		}
	}
	// Did not request for the peer cert verification
	if clientCACert != "" {
		clientCACertBytes, err = os.ReadFile(clientCACert)
		if err != nil {
			log.Panicf("error while reading the crypto file: %s", err)
		}
	}

	return shim.TLSProperties{
		Disabled:      tlsDisabled,
		Key:           keyBytes,
		Cert:          certBytes,
		ClientCACerts: clientCACertBytes,
	}
}

func getEnvOrDefault(env, defaultVal string) string {
	value, ok := os.LookupEnv(env)
	if !ok {
		value = defaultVal
	}
	return value
}

// Note that the method returns default value if the string
// cannot be parsed!
func getBoolOrDefault(value string, defaultVal bool) bool {
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return defaultVal
	}
	return parsed
}