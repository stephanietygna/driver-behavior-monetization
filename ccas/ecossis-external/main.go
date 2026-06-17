package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log"
	"math/big"
	"os"
	"strconv"

	"github.com/hyperledger/fabric-chaincode-go/shim"
	"github.com/hyperledger/fabric-contract-api-go/contractapi"
)

type SmartContract struct {
	contractapi.Contract
}

// ECDSASignature represents the two mathematical components of an ECDSA signature once
// decomposed.
type ECDSASignature struct {
	R, S *big.Int
}

type serverConfig struct {
	CCID    string
	Address string
}

type FileData struct { // id é a chave primária
	Timestamp   string `json:"timestamp"`
	Geolocation string `json:"geolocation"`
	Hash        string `json:"hash"`
	PubKey      string `json:"pubkey"`
}

func (s *SmartContract) StoreFileData(ctx contractapi.TransactionContextInterface, id string, timestamp string, geolocation string, hash string, pubkey string) error {
	fileData := FileData{
		Timestamp:   timestamp,
		Geolocation: geolocation,
		Hash:        hash,
		PubKey:      pubkey,
	}

	fileDataBytes, err := json.Marshal(fileData)
	if err != nil {
		return fmt.Errorf("failed to marshal file data: %v", err)
	}

	return ctx.GetStub().PutState(id, fileDataBytes)
}

func (s *SmartContract) QueryFileData(ctx contractapi.TransactionContextInterface, id string) (*FileData, error) {
	fileAsBytes, err := ctx.GetStub().GetState(id)

	if err != nil {
		return nil, fmt.Errorf("failed to read from world state. %s", err.Error())
	}

	if fileAsBytes == nil {
		return nil, fmt.Errorf("%s does not exist", id)
	}

	fileData := new(FileData)
	_ = json.Unmarshal(fileAsBytes, fileData)

	return fileData, nil
}

func (s *SmartContract) CheckSignature(ctx contractapi.TransactionContextInterface, id string, info string, sign string) error {

	//loging...
	fmt.Println("Testing args: ", id, info, sign)
	fmt.Println("Data ID: ", id)
	fmt.Println("Information: ", info)

	// extrai o registro do medidor
	fileAsBytes, err := ctx.GetStub().GetState(id)

	//test if we receive a valid meter ID
	if err != nil || fileAsBytes == nil {
		return fmt.Errorf("error on retrieving meter ID register")
	}

	//cria estrutura para manipular os bytes do medidor
	fileData := FileData{}

	//loging...
	fmt.Println("Retrieving meter bytes: ", fileAsBytes)

	// decodifica os bytes do medidor para a estrutura e obtem a chave publica
	json.Unmarshal(fileAsBytes, &fileData)
	pubkey := PublicKeyDecodePEM(fileData.PubKey)

	//loging...
	fmt.Println("Retrieving meter after unmarshall: ", fileData)

	//calculates the information hash
	hash := sha256.Sum256([]byte(info))

	//decodifica a assinatura para extrair a string de bytes codificada em DER
	der, err := base64.StdEncoding.DecodeString(sign)
	if err != nil {
		return fmt.Errorf("error on decode the digital signature: %v", err)
	}

	//cria uma estrutura de dados para armazenar a assinatura
	sig := &ECDSASignature{}

	//unmarshal the R and S components of the ASN.1-encoded signature
	//deserializa os componentes R e S da assinatura codificada em ASN.1
	_, err = asn1.Unmarshal(der, sig)
	if err != nil {
		return fmt.Errorf("error on get R and S terms from the digital signature: %v", err)
	}

	//valida a assinatura digital
	valid := ecdsa.Verify(&pubkey, hash[:], sig.R, sig.S)

	// buffer is a JSON array containing records
	var buffer bytes.Buffer
	buffer.WriteString("[")
	buffer.WriteString("\"Counter\":")
	buffer.WriteString(strconv.FormatBool(valid))
	buffer.WriteString("]")

	// notifica o resultado. caso seja true, a mensagem foi assinada corretamente e não foi adulterada
	log.Printf("Signature verified: %t\n", valid)
	// print buffer
	log.Print(buffer.String())

	// return success
	return nil
}

func PublicKeyDecodePEM(pemEncodedPub string) ecdsa.PublicKey {
	blockPub, _ := pem.Decode([]byte(pemEncodedPub))
	x509EncodedPub := blockPub.Bytes
	genericPublicKey, _ := x509.ParsePKIXPublicKey(x509EncodedPub)
	publicKey := genericPublicKey.(*ecdsa.PublicKey)

	return *publicKey
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
