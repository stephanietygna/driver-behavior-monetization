package main

import (
	//the majority of the imports are trivial...
	"os"
	"bytes"
	"crypto/rand"
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"
	"math/big"
	"encoding/asn1"

	//these imports are for Hyperledger Fabric interface
	"github.com/hyperledger/fabric-contract-api-go/contractapi"
)

type SmartContract struct {
	contractapi.Contract
}

// QueryResult structure used for handling result of query
type QueryResult struct {
	Key    string `json:"Key"`
	Record *Meter
}

// ECDSASignature represents the two mathematical components of an ECDSA signature once
// decomposed.
type ECDSASignature struct {
	R, S *big.Int
}

// Meter constitutes our key|value struct (digital asset) and implements a single
// record to manage the
// meter public key and measures. All blockchain transactions operates with this type.
// IMPORTANT: all the field names must start with upper case
type Meter struct {
	Vehicle       string    `json:"VEHICLE_TYPE"`
	ImbalanceApv  []string  `json:"IMBALANCE_APPROVAL"`
	PbApv         string    `json:"PARK_BRAKE_APPROVAL"`
	OvrlApv       string    `json:"OVERALL_EFFICIENCY_APPROVAL"`
	Time          time.Time `json:"TEST_TIME"`
	Vehicle_plate string    `json:"VEHICLE_PLATE"`
}

/*
//PubKey ecdsa.PublicKey `json:"pubkey"`
	Vehicle Plate string `json:"pubkey"`
	imbalanceApv []bool `json: "imbalanceapproval"`
	pbApv bool `json: "parkbrakeapproval"`
	ovrlApv bool `json: "ovrleffiencyapproval"`
*/

// PublicKeyDecodePEM method decodes a PEM format public key. So the smart contract can lead
// with it, store in the blockchain, or even verify a signature.
// - pemEncodedPub - A PEM-format public key
func PublicKeyDecodePEM(pemEncodedPub string) ecdsa.PublicKey {
    blockPub, _ := pem.Decode([]byte(pemEncodedPub))
    if blockPub == nil {
        panic("failed to parse PEM block containing the public key")
    }
    
    pub, err := x509.ParsePKIXPublicKey(blockPub.Bytes)
    if err != nil {
        panic("failed to parse DER encoded public key: " + err.Error())
    }

    return *pub.(*ecdsa.PublicKey)
}

type FileData struct {
	PubKey string `json:"pub_key"`
}


// Init method is called when the fabpki is instantiated.
// Best practice is to have any Ledger initialization in separate function.
// Note that chaincode upgrade also calls this function to reset
// or to migrate data, so be careful to avoid a scenario where you
// inadvertently clobber your ledger's data!
// func (s *SmartContract) Init(ctx contractapi.TransactionContextInterface) error {
// 	return nil
// }

/*
	SmartContract::registerMeter(...)
	Does the register of a new meter into the ledger.
	The meter is the base of the key|value structure.
	The key constitutes the meter ID.
	- args[0] - meter ID
	- args[1] - the public key associated with the meter
*/
var (
	chavePublicaUsuario = `-----BEGIN PUBLIC KEY-----
MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEYvuVQZCFzqRFfqmMXjGaOk6udBn7
zwSwHsxRqkt0LUOSmsWCV9ACJKpQ9+tMs0h8CesCSbqWQ8CsUddIHB5sJA==
-----END PUBLIC KEY-----`

	chavePrivadaUsuario = `-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIKFlogj6Qk8pvI8MDhqLp/Kfey26rgy+A7kJ6CPfbFYToAoGCCqGSM49
AwEHoUQDQgAEYvuVQZCFzqRFfqmMXjGaOk6udBn7zwSwHsxRqkt0LUOSmsWCV9AC
JKpQ9+tMs0h8CesCSbqWQ8CsUddIHB5sJA==
-----END EC PRIVATE KEY-----`
)

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Função para verificar a assinatura digital
func signDataECDSA(data string) (string, error) {
	// Decodifica a chave privada PEM
	block, _ := pem.Decode([]byte(chavePrivadaUsuario))
	if block == nil {
		return "", fmt.Errorf("falha ao decodificar chave privada PEM")
	}

	// Converte para *ecdsa.PrivateKey
	privKey, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("falha ao parsear chave privada: %v", err)
	}

	// Calcula hash SHA-256 dos dados
	hash := sha256.Sum256([]byte(data))

	// Assina o hash com ECDSA
	signature, err := ecdsa.SignASN1(rand.Reader, privKey, hash[:])
	if err != nil {
		return "", fmt.Errorf("erro ao assinar: %v", err)
	}

	// Retorna em Base64
	return base64.StdEncoding.EncodeToString(signature), nil
}

func (s *SmartContract) verifySignature(ctx contractapi.TransactionContextInterface, id string, data string, signature string) error {
    // Extrai o registro do medidor para obter a chave pública
    fileAsBytes, err := ctx.GetStub().GetState(id)
    if err != nil || fileAsBytes == nil {
        return fmt.Errorf("error on retrieving meter ID register: %v", err)
    }

    // Decodifica os bytes do medidor para a estrutura e obtém a chave pública
    fileData := FileData{}
    if err := json.Unmarshal(fileAsBytes, &fileData); err != nil {
        return fmt.Errorf("error unmarshaling file data: %v", err)
    }

    if fileData.PubKey == "" {
        return fmt.Errorf("no public key found for this ID")
    }

    pubkey := PublicKeyDecodePEM(fileData.PubKey)

    // Calcula o hash SHA-256 dos dados
    hash := sha256.Sum256([]byte(data))

    // Decodifica a assinatura base64 para extrair a string DER
    der, err := base64.StdEncoding.DecodeString(signature)
    if err != nil {
        return fmt.Errorf("error decoding base64 signature: %v", err)
    }

    // Deserializa os componentes R e S da assinatura
    sig := &ECDSASignature{}
    if _, err := asn1.Unmarshal(der, sig); err != nil {
        return fmt.Errorf("error unmarshaling signature: %v", err)
    }

    // Verifica a assinatura
    if !ecdsa.Verify(&pubkey, hash[:], sig.R, sig.S) {
        return fmt.Errorf("invalid signature")
    }

    return nil
}

func (s *SmartContract) RegisterSign(ctx contractapi.TransactionContextInterface, vehicle_plate string, jsonStr string, signature string) error {
	err := s.verifySignature(ctx, vehicle_plate, jsonStr, signature)
	if err != nil {
		return fmt.Errorf("Signature verification failed: %v", err)
	}

	jsonStr = strings.ReplaceAll(jsonStr, "'", "\"")
	fmt.Println(jsonStr)

	type Data struct {
		Data []int `json:"data"`
	}

	// Decodificar a string JSON para a struct
	var jsonData Data
	err = json.Unmarshal([]byte(jsonStr), &jsonData)
	if err != nil {
		log.Fatal(err)
	}

	// Acessar a lista de inteiros
	reportData := jsonData.Data

	fmt.Println(reportData)

	pbTotalForce := reportData[len(reportData)-1]
	numWheels := (len(reportData) - 1) / 2

	vehicleWeight := calcTotalWeight(reportData, numWheels)
	fmt.Println("calcTotalWeight OK")
	vehicleMass := calcMass(vehicleWeight)
	fmt.Println("calcMass OK")
	vehicleType := checkType(vehicleMass)
	fmt.Println("checkType OK")
	imbalanceApproval := approvesImbalance(reportData, numWheels)
	fmt.Println("approvesImbalance OK")
	pbApproval := approvesPbEfficiency(calcPbEfficiency(pbTotalForce, vehicleWeight))
	fmt.Println("approvesPbEfficiency OK")
	ovrlEfficiencyApproval := approvesOvrlEfficiency(reportData, vehicleType)
	fmt.Println("approvesOvrlEfficiency OK")

	register := createRegister(ovrlEfficiencyApproval, vehicleType, pbApproval)
	imbalanceRegister := createRegisterImbalance(imbalanceApproval)
	testTime := time.Now()

	var meter = Meter{Vehicle: register[0], ImbalanceApv: imbalanceRegister, PbApv: register[2], OvrlApv: register[1], Time: testTime, Vehicle_plate: vehicle_plate}

	//encapsulates meter in a JSON structure
	meterAsBytes, err := json.Marshal(meter)
	if err != nil {
		log.Fatal("Erro ao serializar o JSON", err)
	}

	//loging...
	fmt.Println("Registering meter: ", meter)

	fileName := fmt.Sprintf("%s.json", vehicle_plate)
	err = os.WriteFile(fileName, meterAsBytes, 0644)
	if err != nil {
		log.Fatal("erro ao escrever no arquivo", err)
	}

	fmt.Println("Storing meter with key:", vehicle_plate)
	err = ctx.GetStub().PutState(vehicle_plate, meterAsBytes)
	if err != nil {
		return fmt.Errorf("failed to put to world state: %v", err)
	}
	fmt.Println("Meter stored successfully")
	return nil
}

func (s *SmartContract) RegisterMeter(ctx contractapi.TransactionContextInterface, vehicle_plate string, jsonStr string) error {
	jsonStr = strings.ReplaceAll(jsonStr, "'", "\"")
	fmt.Println(jsonStr)

	type Data struct {
		Data []int `json:"data"`
	}

	// Decodificar a string JSON para a struct
	var jsonData Data
	err := json.Unmarshal([]byte(jsonStr), &jsonData)
	if err != nil {
		log.Fatal(err)
	}

	// Acessar a lista de inteiros
	reportData := jsonData.Data

	fmt.Println(reportData)

	pbTotalForce := reportData[len(reportData)-1]
	numWheels := (len(reportData) - 1) / 2

	vehicleWeight := calcTotalWeight(reportData, numWheels)
	fmt.Println("calcTotalWeight OK")
	vehicleMass := calcMass(vehicleWeight)
	fmt.Println("calcMass OK")
	vehicleType := checkType(vehicleMass)
	fmt.Println("checkType OK")
	imbalanceApproval := approvesImbalance(reportData, numWheels)
	fmt.Println("approvesImbalance OK")
	pbApproval := approvesPbEfficiency(calcPbEfficiency(pbTotalForce, vehicleWeight))
	fmt.Println("approvesPbEfficiency OK")
	ovrlEfficiencyApproval := approvesOvrlEfficiency(reportData, vehicleType)
	fmt.Println("approvesOvrlEfficiency OK")

	register := createRegister(ovrlEfficiencyApproval, vehicleType, pbApproval)
	imbalanceRegister := createRegisterImbalance(imbalanceApproval)
	testTime := time.Now()

	var meter = Meter{Vehicle: register[0], ImbalanceApv: imbalanceRegister, PbApv: register[2], OvrlApv: register[1], Time: testTime, Vehicle_plate: vehicle_plate}

	//encapsulates meter in a JSON structure
	meterAsBytes, err := json.Marshal(meter)
	if err != nil {
		log.Fatal("Erro ao serializar o JSON", err)
	}

	//loging...
	fmt.Println("Registering meter: ", meter)

	fileName := fmt.Sprintf("%s.json", vehicle_plate)
	err = os.WriteFile(fileName, meterAsBytes, 0644)
	if err != nil {
		log.Fatal("erro ao escrever no arquivo", err)
	}

	fmt.Println("Storing meter with key:", vehicle_plate)
	err = ctx.GetStub().PutState(vehicle_plate, meterAsBytes)
	if err != nil {
		return fmt.Errorf("failed to put to world state: %v", err)
	}
	fmt.Println("Meter stored successfully")
	return nil
}

func createRegisterImbalance(imbalanceApproval []bool) []string {
	var imbalance []string
	var str string

	for i := 0; i < len(imbalanceApproval); i++ {
		if imbalanceApproval[i] {
			str = "Braking force imbalance of axis [" + strconv.Itoa(i+1) + "] was [Approved]"
		} else {
			str = "Braking force imbalance of axis [" + strconv.Itoa(i+1) + "] was [Disapproved]"
		}
		imbalance = append(imbalance, str)
	}

	return imbalance
}

func createRegister(ovrlEfficiencyApproval bool, vehicleType bool, pbApproval bool) []string {
	var str string
	var register []string

	if vehicleType {
		str = "Vehicle type => [Heavy Weight]"
	} else {
		str = "Vehicle type => [Light Weight]"
	}

	register = append(register, str)

	if ovrlEfficiencyApproval {
		str = "Total braking efficiency was [Approved]"
	} else {
		str = "Total braking efficiency was [Disapproved]"
	}

	register = append(register, str)

	if pbApproval {
		str = "Parking braker was [Approved]"
	} else {
		str = "Parking braker was [Disapproved]"
	}

	register = append(register, str)

	return register
}

func (s *SmartContract) QueryLedger(ctx contractapi.TransactionContextInterface, vehicle_plate string) (*Meter, error) {
	// Obter o estado do ledger para a chave fornecida (vehicle_plate)
	result, err := ctx.GetStub().GetState(vehicle_plate)
	if err != nil {
		return nil, fmt.Errorf("error while getting state: %s", err)
	}

	meter := new(Meter)
	err = json.Unmarshal(result, meter)

	// Verifica se a chave existe no ledger
	if result == nil {
		return nil, fmt.Errorf("no record found for vehicle plate: %s", vehicle_plate)
	}
	fmt.Println("teste")

	// Retorna o valor armazenado como uma string
	return meter, nil
}

//////////////////////////////////////////////////////////////////////////////////////////////////////////

// Calculates the mass of a vehicle using weight values from report.
func calcMass(weightSum int) float64 {
	gvtAcceleration := 9.8
	return float64(weightSum) / gvtAcceleration
}

// Sums the weight values and returns the total result.
func calcTotalWeight(reportData []int, numWheels int) int {
	var weightSum int
	for i := 0; i < numWheels; i++ {
		weightSum += reportData[i]
	}
	return weightSum
}

// Checking the type of a vehicle by its mass.
func checkType(vehicleMass float64) bool {
	var vehicleType bool
	if vehicleMass > 3500 {
		vehicleType = true // True stands for heavy weight vehicle.
	} else {
		vehicleType = false // False stands for light weight vehicle.
	}
	return vehicleType
}

// Calculates the braking force imbalance from two wheels.
func calcImbalance(leftWheel int, rightWheel int) float64 {
	var higherNum int
	var lowerNum int
	if leftWheel > rightWheel {
		higherNum = leftWheel
		lowerNum = rightWheel
	} else {
		higherNum = rightWheel
		lowerNum = leftWheel
	}
	imbalance := 100 * (float64(higherNum-lowerNum) / float64(higherNum))
	return imbalance
}

// Check if the braking force of each axle is approved or not.
func approvesImbalance(reportData []int, numWheels int) []bool {
	var approvalStatus []bool
	for i := numWheels; i < (len(reportData) - 2); i += 2 {
		if calcImbalance(reportData[i], reportData[i+1]) <= 20 {
			approvalStatus = append(approvalStatus, true)
		} else {
			approvalStatus = append(approvalStatus, false)
		}
	}
	return approvalStatus
}

// Calculates overall braking efficiency
func calcOvrlEfficiency(reportData []int) float64 {
	var overallEfficiency, weightSum, brakingFrcSum float64
	for i := 0; i < len(reportData); i++ {
		if i < (len(reportData)-1)/2 {
			weightSum += float64(reportData[i])
		} else {
			brakingFrcSum += float64(reportData[i])
		}
	}
	overallEfficiency = weightSum / brakingFrcSum
	return 100 * overallEfficiency
}

// Calculates the parking braker efficiency.
func calcPbEfficiency(totalForce int, totalWeight int) float64 {
	return 100 * (float64(totalForce) / float64(totalWeight))
}

// Check if parking braker efficiency is approved or not.
func approvesPbEfficiency(pbEfficiency float64) bool {
	if pbEfficiency >= 18 {
		return true
	} else {
		return false
	}
}

// Check if overall braking efficiency is approved or not.
func approvesOvrlEfficiency(reportData []int, vehicleType bool) bool {
	if vehicleType { // Heavy vehicle
		if calcOvrlEfficiency(reportData) >= 50 {
			return true
		} else {
			return false
		}
	} else { // Light vehicle
		if calcOvrlEfficiency(reportData) >= 55 {
			return true
		} else {
			return false
		}
	}
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