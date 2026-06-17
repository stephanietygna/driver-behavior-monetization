package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	// "github.com/emicklei/go-restful/v3/log"
	"github.com/hyperledger/fabric-chaincode-go/shim"
	"github.com/hyperledger/fabric-contract-api-go/contractapi"
)

type SmartContract struct {
	contractapi.Contract
}

type serverConfig struct {
	CCID    string
	Address string
}

// VehicleData representa os dados do veículo
// type VehicleData struct { // pk: idcarro / placa do veiculo
// 	Latitude  string `json:"latitude"`  // Mudança Brusca de Direção
// 	Longitude string `json:"longitude"` // Mudança Brusca de Direção
// 	Direction string `json:"direction"` // Mudança Brusca de Direção
// 	Speed     string `json:"speed"`     // Detecção de Aceleração Anômala // Mudança Brusca de Direção
// 	AccelX    string `json:"accelX"`    //zigue-zague
// 	AccelY    string `json:"accelY"`    //zigue-zague
// 	AccelZ    string `json:"accelZ"`    //zigue-zague // A aceleração em Z pode ser útil para detectar comportamentos relacionados a movimentos verticais // como subidas, descidas ou saltos, especialmente em terrenos irregulares.
// 	TimeStamp string `json:"timestamp"` //Detecção de Aceleração Anômala
// 	Flag      string `json:"flag"`      // controle de 10 em 10 linhas
// }

type Wallet struct {
	User string `json:"username"`
	Credits int `json:"credits"`
}

// ConvertStringToFloatSlice converte uma string de números separados por espaço em um slice de float64
func ConvertStringToFloatSlice(data string) ([]float64, error) {
	parts := strings.Fields(data)
	var result []float64
	for _, part := range parts {
		value, err := strconv.ParseFloat(part, 64)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, nil
}

// Cria a wallet no nome do usuário fornecido
func (s *SmartContract) CreateWallet(ctx contractapi.TransactionContextInterface, user string) error {
	exists, err := s.WalletExists(ctx, user)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("wallet already exists for user %s", user)
	}

	wallet := Wallet{
		User: user,
		Credits: 0,
	}

	walletJSON, err := json.Marshal(wallet)
	if err != nil {
		return err
	}

	return ctx.GetStub().PutState(user, walletJSON)
}

// adiciona creditos a carteira
func (s *SmartContract) AddCredits(ctx contractapi.TransactionContextInterface, user string, amount int) error {
	walletJSON, err := ctx.GetStub().GetState(user)
	if err != nil {
		return err
	}
	if walletJSON == nil {
		return fmt.Errorf("wallet does not exist for user %s", user)
	}

	var wallet Wallet
	err = json.Unmarshal(walletJSON, &wallet)
	if err != nil {
		return err
	}

	wallet.Credits += amount
	walletJSON, err = json.Marshal(wallet)
	if err != nil {
		return err
	}

	return ctx.GetStub().PutState(user, walletJSON)
}

// Checa se a carteira já existe para o usuário
func (s *SmartContract) WalletExists(ctx contractapi.TransactionContextInterface, user string) (bool, error) {
	walletJSON, err := ctx.GetStub().GetState(user)
	if err != nil {
		return false, err
	}
	return walletJSON != nil, nil
}

// Dá um query na carteira do usuário
func (s *SmartContract) QueryWallet(ctx contractapi.TransactionContextInterface, user string) (int, error) {
	walletJSON, err := ctx.GetStub().GetState(user)
	if err != nil {
		return 0, err
	}
	if walletJSON == nil {
		return 0, fmt.Errorf("wallet does not exist for user %s", user)
	}

	var wallet Wallet
	err = json.Unmarshal(walletJSON, &wallet)
	if err != nil {
		return 0, err
	}
	log.Printf("Créditos: %v", wallet.Credits)

	return wallet.Credits, nil
}


// Recebe o usuário que envia, o usuário que recebe e o valor
func (s *SmartContract) TransferCredits(ctx contractapi.TransactionContextInterface, sender string, receiver string, amount int) error {
	if sender == receiver {
		return fmt.Errorf("sender and receiver cannot be the same")
	}

	senderWalletJSON, err := ctx.GetStub().GetState(sender)
	if err != nil {
		return err
	}
	if senderWalletJSON == nil {
		return fmt.Errorf("wallet does not exist for sender %s", sender)
	}

	receiverWalletJSON, err := ctx.GetStub().GetState(receiver)
	if err != nil {
		return err
	}
	if receiverWalletJSON == nil {
		return fmt.Errorf("wallet does not exist for receiver %s", receiver)
	}

	var senderWallet, receiverWallet Wallet
	err = json.Unmarshal(senderWalletJSON, &senderWallet)
	if err != nil {
		return err
	}
	err = json.Unmarshal(receiverWalletJSON, &receiverWallet)
	if err != nil {
		return err
	}

	if senderWallet.Credits < amount {
		return fmt.Errorf("insufficient funds in sender's wallet")
	}

	senderWallet.Credits -= amount
	receiverWallet.Credits += amount

	senderWalletJSON, err = json.Marshal(senderWallet)
	if err != nil {
		return err
	}
	receiverWalletJSON, err = json.Marshal(receiverWallet)
	if err != nil {
		return err
	}

	err = ctx.GetStub().PutState(sender, senderWalletJSON)
	if err != nil {
		return err
	}

	return ctx.GetStub().PutState(receiver, receiverWalletJSON)
}

func main() {
	// See chaincode.env.example
	config := serverConfig{
		CCID:    os.Getenv("CHAINCODE_ID"),
		Address: os.Getenv("CHAINCODE_SERVER_ADDRESS"),
	}

	chaincode, err := contractapi.NewChaincode(&SmartContract{})

	if err != nil {
		log.Panicf("error create ... chaincode: %s", err)
	}

	server := &shim.ChaincodeServer{
		CCID:     config.CCID,
		Address:  config.Address,
		CC:       chaincode,
		TLSProps: getTLSProperties(),
	}

	if err := server.Start(); err != nil {
		log.Panicf("error starting ... chaincode: %s", err)
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