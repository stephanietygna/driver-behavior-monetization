package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"math/big"
	"os"
	"strconv"
	"strings"
	"time"

	// "github.com/emicklei/go-restful/v3/log"
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

// VehicleData representa os dados do veículo
type VehicleData struct { // pk: idcarro / placa do veiculo
	Latitude  string `json:"latitude"`  // Mudança Brusca de Direção
	Longitude string `json:"longitude"` // Mudança Brusca de Direção
	Direction string `json:"direction"` // Mudança Brusca de Direção
	Speed     string `json:"speed"`     // Detecção de Aceleração Anômala // Mudança Brusca de Direção
	AccelX    string `json:"accelX"`    // zigue-zague
	AccelY    string `json:"accelY"`    // zigue-zague
	AccelZ    string `json:"accelZ"`    // zigue-zague // A aceleração em Z pode ser útil para detectar comportamentos relacionados a movimentos verticais // como subidas, descidas ou saltos, especialmente em terrenos irregulares.
	// RPM       string `json:"rpm"`       // Detecção de parada para descanso
	TimeStamp string `json:"timestamp"` // Detecção de Aceleração Anômala
	Flag      string `json:"flag"`      // controle de 10 em 10 linhas
}

// VehicleWallet representa a carteira do veículo
type VehicleWallet struct { // pk: idcarro
	Credits int `json:"credits"`
}

// UserInfractions representa o histórico de infrações do condutor
type UserInfractions struct {
	AnomalousAccelerationOccurences int      `json:"anomalousAccelerationOccurences"`
	AnomalousAccelerationTimestamps []string `json:"anomalousAccelerationTimestamps"`
	RestlessDrivingOccurences       int      `json:"restlessDrivingOccurences"`
	RestlessDrivingTimestamps       []string `json:"restlessDrivingTimestamps"`
	SharpTurnOccurences             int      `json:"sharpTurnOccurences"`
	SharpTurnTimestamps             []string `json:"sharpTurnTimestamps"`
	// ZigZagOccurences                int      `json:"zigZagOccurences"`
	// ZigZagTimestamps                []string `json:"zigZagTimestamps"`
}

// Struct usado exclusivamente para o funcionamento da função DetectRestlessDriving
type controlRestlessDrivingStruct struct {
	firstEverTimestamp        time.Time // Armazena o primeiro timestamp de todos
	firstEverTimestampFlag    bool      // Indica se o primeiro timestamp de todos já foi armazenado
	nextPauseTime             time.Time // Armazena o próximo horário em que o motorista deve fazer uma pausa
	minutesBetweenPauses      int       // Define o intervalo de minutos entre as pausas
	minutesBetweenPausesHarsh int       // Define o intervalo de minutos entre as pausas sabendo que o motorista não pausou previamente
	hasPaused                 bool      // Indica se o motorista fez uma pausa recentemente
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

// ConvertUnixTimestampToTimeObject converte uma string de tempo em formato Unix para um objeto do tipo time.Time
func ConvertUnixTimestampToTimeObject(timestampString string) (time.Time, error) {
	timestampInt, err := strconv.ParseInt(timestampString, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("falha ao converter string em int: %s", err)
	}
	timestampTime := time.Unix(timestampInt, 0)
	return timestampTime, nil
}

func (s *SmartContract) AnalyzeDriverBehavior(ctx contractapi.TransactionContextInterface, idcarro string) error {
	// Recuperar o histórico de dados do veículo do ledger
	// [BUG] Ele lê o próximo mesmo que não tenha nada
	historyIterator, err := ctx.GetStub().GetHistoryForKey(idcarro)
	if err != nil {
		return fmt.Errorf("falha ao obter histórico de dados do veículo: %s", err)
	}
	defer historyIterator.Close()

	// Inicializar saldo
	var saldo int

	// Variável usada exclusivamente para o funcionamento da função DetectRestlessDriving
	var controlRestlessDriving controlRestlessDrivingStruct = controlRestlessDrivingStruct{
		firstEverTimestamp:        time.Time{},
		firstEverTimestampFlag:    true,
		nextPauseTime:             time.Time{},
		minutesBetweenPauses:      80,
		minutesBetweenPausesHarsh: 20,
		hasPaused:                 true,
	}

	// Analisar cada registro histórico
	latitudeSlice := []string{}
	longitudeSlice := []string{}
	speedSlice := []string{}
	timestampSlice := []string{}
	accelXSlice := []string{}
	accelYSlice := []string{}
	accelZSlice := []string{}
	// rpmSlice := []string{}
	flagSlice := []string{}
	directionSlice := []string{}

	// // Ver com o copilot se dá pra melhorar chamando a função QueryUserInfractions
	// indexName := "USERINFRACTIONS"
	// compositeKey, err := ctx.GetStub().CreateCompositeKey(indexName, []string{idcarro})
	// if err != nil {
	// 	return err
	// }

	// userInfractionsAsBytes, err := ctx.GetStub().GetState(compositeKey)
	// if err != nil {
	// 	return fmt.Errorf("failed to read from world state: %s", err)
	// }

	// if userInfractionsAsBytes == nil {
	// 	return fmt.Errorf("histórico do veículo não encontrado")
	// }

	// var userInfractions UserInfractions
	// err = json.Unmarshal(userInfractionsAsBytes, &userInfractions)
	// if err != nil {
	// 	return fmt.Errorf("falha ao desserializar o histórico do veículo: %s", err)
	// }
	// // Ver com o copilot se dá pra melhorar chamando a função QueryUserInfractions

	// Iterar sobre o histórico e aplicar análises
	for historyIterator.HasNext() {
		historyEntry, err := historyIterator.Next()
		if err != nil {
			return fmt.Errorf("falha ao iterar sobre o histórico de dados do veículo: %s", err)
		}

		var historicalData VehicleData
		err = json.Unmarshal(historyEntry.Value, &historicalData)
		if err != nil {
			return fmt.Errorf("falha ao desserializar dados históricos do veículo: %s", err)
		}

		// speed, err := strconv.ParseFloat(historicalData.Speed, 32)
		// log.Printf("Velocidade (MK1): %v", historicalData.Speed)
		// if err != nil {
		// 	if historicalData.Speed == "" {
		// 		break
		// 	} else {
		// 		return fmt.Errorf("falha ao converter velocidade histórica: %s", err)
		// 	}
		// }
		// timestamp, err := strconv.ParseInt(historicalData.TimeStamp, 10, 32)
		// if err != nil {
		// 	return fmt.Errorf("falha ao converter timestamp histórico: %s", err)
		// }

		if controlRestlessDriving.firstEverTimestampFlag {
			ts, err := ConvertUnixTimestampToTimeObject(historicalData.TimeStamp)
			if err != nil {
				return fmt.Errorf("erro ao converter timestamp mais recente em objeto time.Time: %s", err)
			}
			controlRestlessDriving.firstEverTimestamp = ts
			controlRestlessDriving.firstEverTimestampFlag = false
			controlRestlessDriving.nextPauseTime = controlRestlessDriving.firstEverTimestamp.Add(time.Duration(controlRestlessDriving.minutesBetweenPauses) * time.Minute)
		}

		latitudeSlice = append(latitudeSlice, historicalData.Latitude)
		longitudeSlice = append(longitudeSlice, historicalData.Longitude)
		flagSlice = append(flagSlice, historicalData.Flag)
		speedSlice = append(speedSlice, historicalData.Speed)
		timestampSlice = append(timestampSlice, historicalData.TimeStamp)
		directionSlice = append(directionSlice, historicalData.Direction)
		accelXSlice = append(accelXSlice, historicalData.AccelX)
		accelYSlice = append(accelYSlice, historicalData.AccelY)
		accelZSlice = append(accelZSlice, historicalData.AccelZ)
		// rpmSlice = append(rpmSlice, historicalData.RPM)

		// Interrompe a execução após 10 registros
		if len(speedSlice) == 10 {
			break
		}
	}

	// TODO: Ver com o Victor se dá pra melhorar esse e outros trechos utilizadn as funções do smart contract ao invés de reescrever tudo aqui
	// Puxa o histórico de ocorrências do usuário antes de fazer as detecções
	userInfractionsKey, err := ctx.GetStub().CreateCompositeKey("USERINFRACTIONS", []string{idcarro})
	if err != nil {
		return fmt.Errorf("erro ao criar chave composta para o histórico de infrações: %s", err)
	}

	userInfractionsAsBytes, err := ctx.GetStub().GetState(userInfractionsKey)
	if err != nil {
		return fmt.Errorf("erro ao recuperar o histórico de infrações do usuário: %s", err)
	}

	userInfractions := UserInfractions{}
	if userInfractionsAsBytes != nil {
		err = json.Unmarshal(userInfractionsAsBytes, &userInfractions)
		if err != nil {
			return fmt.Errorf("falha ao desserializar o histórico de infrações do usuário: %s", err)
		}
	}

	// TODO: Ver com o Victor se dá pra melhorar esse e outros trechos utilizadn as funções do smart contract ao invés de reescrever tudo aqui
	// userInfractions, err := s.QueryUserInfractions(ctx, idcarro)
	//
	// if err != nil {
	// 	return fmt.Errorf("erro ao consultar histórico de infrações: %s", err)
	// }

	// Esse bloco será executado a cada 10 linhas/segundos e se o primeioro flag for true
	if len(flagSlice) > 0 && flagSlice[0] == "true" {

		// Detecção de aceleração anômala
		credAnomalousAcceleration, err, flagAnomalousAcceleration := DetectAnomalousAcceleration(speedSlice, userInfractions.AnomalousAccelerationOccurences)
		if err != nil {
			return fmt.Errorf("erro ao detectar aceleração anômala: %s", err)
		}
		if flagAnomalousAcceleration {
			userInfractions.AnomalousAccelerationOccurences++
			userInfractions.AnomalousAccelerationTimestamps = append(userInfractions.AnomalousAccelerationTimestamps, timestampSlice[len(timestampSlice)-1])
		}
		saldo += credAnomalousAcceleration

		// Detecção de condução sem pausas para descanso. É controlada pela variável controlRestlessDriving
		mostRecentTime, err := ConvertUnixTimestampToTimeObject(timestampSlice[9])
		if err != nil {
			return fmt.Errorf("erro ao converter timestamp mais recente em objeto time.Time: %s", err)
		}

		// // Aqui checa-se se já está na hora da pausa de descanso
		// if mostRecentTime.After(controlRestlessDriving.nextPauseTime) { // Se já passou do horário da pausa...
		// 	controlRestlessDriving.hasPaused = false					// ... o motorista não pausou
		// 	controlRestlessDriving.nextPauseTime = controlRestlessDriving.nextPauseTime.Add(time.Duration(controlRestlessDriving.minutesBetweenPausesHarsh) * time.Minute) // Atualiza o próximo horário de pausa
		// }

		// Aqui checa-se se já está na hora da pausa de descanso
		if mostRecentTime.After(controlRestlessDriving.nextPauseTime) {

			if controlRestlessDriving.hasPaused { // Aplica-se a detecção normal se for true

				credRestlessDriving, err, flagRestlessDriving := DetectRestlessDriving(latitudeSlice, longitudeSlice, timestampSlice, userInfractions.RestlessDrivingOccurences)
				if err != nil {
					return fmt.Errorf("erro ao detectar condução sem pausas para descanso: %s", err)
				}
				if flagRestlessDriving {
					controlRestlessDriving.hasPaused = false                                                                                                                       // O motorista não pausou
					controlRestlessDriving.nextPauseTime = controlRestlessDriving.nextPauseTime.Add(time.Duration(controlRestlessDriving.minutesBetweenPausesHarsh) * time.Minute) // Atualiza o próximo horário de pausa usando intervalo menor
					userInfractions.RestlessDrivingOccurences++
					userInfractions.RestlessDrivingTimestamps = append(userInfractions.RestlessDrivingTimestamps, timestampSlice[9])
				} else {
					controlRestlessDriving.hasPaused = true                                                                                                                   // Se não houve infração, o motorista está descansado novamente
					controlRestlessDriving.nextPauseTime = controlRestlessDriving.nextPauseTime.Add(time.Duration(controlRestlessDriving.minutesBetweenPauses) * time.Minute) // Atualiza o próximo horário de pausa usando intervalo normal
				}
				saldo += credRestlessDriving

			} else { // Aplica-se detecção rigorosa se for false

				credHarshRestlessDriving, err, flagHarshRestlessDriving := DetectHarshRestlessDriving(latitudeSlice, longitudeSlice, timestampSlice, userInfractions.RestlessDrivingOccurences) // Aqui se usa a função complementar
				if err != nil {
					return fmt.Errorf("erro ao detectar novamente condução sem pausas para descanso: %s", err)
				}
				if flagHarshRestlessDriving {
					controlRestlessDriving.hasPaused = false                                                                                                                       //	o motorista não pausou
					controlRestlessDriving.nextPauseTime = controlRestlessDriving.nextPauseTime.Add(time.Duration(controlRestlessDriving.minutesBetweenPausesHarsh) * time.Minute) // Atualiza o próximo horário de pausa
					userInfractions.RestlessDrivingOccurences++
					userInfractions.RestlessDrivingTimestamps = append(userInfractions.RestlessDrivingTimestamps, timestampSlice[9])
				} else {
					controlRestlessDriving.hasPaused = true                                                                                                                   // Se não houve infração, o motorista está descansado novamente
					controlRestlessDriving.nextPauseTime = controlRestlessDriving.nextPauseTime.Add(time.Duration(controlRestlessDriving.minutesBetweenPauses) * time.Minute) // Atualiza o próximo horário de pausa usando intervalo normal
				}
				saldo += credHarshRestlessDriving
			}
		}

	// 	// Detecção de zigue-zague
	// 	credZigZag, err, flagZigZag := DetectZigZag(accelXSlice, accelYSlice, accelZSlice, userInfractions.ZigZagOccurences)
	// 	if flagZigZag {
	// 		userInfractions.ZigZagOccurences++
	// 		userInfractions.ZigZagTimestamps = append(userInfractions.ZigZagTimestamps, timestampSlice[len(timestampSlice)-1])
	// 	}
	// 	if err != nil {
	// 		return fmt.Errorf("erro ao detectar condução em zigue-zague: %s", err)
	// 	}
	// 	saldo += credZigZag

	// }

	// Detecção de curvas bruscas
	credSharpTurn, err, flagSharpTurn := DetectSharpTurn(speedSlice[0], directionSlice[0], userInfractions.SharpTurnOccurences)
	if err != nil {
		return fmt.Errorf("erro ao detectar curva acentuada: %s", err)
	}
	if flagSharpTurn {
		userInfractions.SharpTurnOccurences++
		userInfractions.SharpTurnTimestamps = append(userInfractions.SharpTurnTimestamps, timestampSlice[len(timestampSlice)-1])
	}
	saldo += credSharpTurn

	// Atualizar o saldo na carteira do cliente
	walletKey, err := ctx.GetStub().CreateCompositeKey("WALLET", []string{idcarro})
	if err != nil {
		return fmt.Errorf("erro ao criar chave composta para a carteira: %s", err)
	}

	vehicleWalletAsBytes, err := ctx.GetStub().GetState(walletKey)
	if err != nil {
		return fmt.Errorf("erro ao recuperar o saldo atual da carteira: %s", err)
	}

	vehicleWallet := VehicleWallet{}
	if vehicleWalletAsBytes != nil {
		err = json.Unmarshal(vehicleWalletAsBytes, &vehicleWallet)
		if err != nil {
			return fmt.Errorf("falha ao desserializar a carteira do veículo: %s", err)
		}
	}

	vehicleWallet.Credits += saldo
	vehicleWalletJSON, err := json.Marshal(vehicleWallet)
	if err != nil {
		return fmt.Errorf("falha ao serializar a carteira do veículo: %s", err)
	}

	// Salvar o registro atual da carteira no ledger
	_ = ctx.GetStub().PutState(walletKey, vehicleWalletJSON)

	// Limpar infrações antigas com base no timestamp mais recente
	// currentTime, err := strconv.ParseInt(timestampSlice[len(timestampSlice)-1], 10, 64)
	// if err != nil {
	// 	return fmt.Errorf("falha ao converter timestamp atual: %s", err)
	// }
	// userInfractions.CleanOldInfractions(timestampSlice[len(timestampSlice)-1])

	// Atualizar histórico de infrações do usuário
	userInfractionsJSON, err := json.Marshal(userInfractions)
	if err != nil {
		return fmt.Errorf("falha ao serializar o histórico de infrações do usuário: %s", err)
	}

	// Salvar o registro atual do histórico de infrações no ledger e finalizar
	return ctx.GetStub().PutState(userInfractionsKey, userInfractionsJSON)
}

// StoreVehicleData armazena os dados do veículo no ledger
// func (s *SmartContract) StoreVehicleData(ctx contractapi.TransactionContextInterface, idcarro string, unixTimestamp string, latitudeStr string, longitudeStr string, speedStr string, accelXstr string, accelYstr string, accelZstr string, rpmStr string, flag string) error {
func (s *SmartContract) StoreVehicleData(ctx contractapi.TransactionContextInterface, idcarro string, unixTimestamp string, latitudeStr string, longitudeStr string, speedStr string, accelXstr string, accelYstr string, accelZstr string, flag string) error {
	// Recuperar dados anteriores para calcular a direção
	previousDataJSON, err := ctx.GetStub().GetState(idcarro)
	if err != nil {
		return fmt.Errorf("falha ao ler os dados do veículo do ledger: %s", err)
	}

	if previousDataJSON != nil {
		var previousVehicleData VehicleData
		err = json.Unmarshal(previousDataJSON, &previousVehicleData)
		if err != nil {
			return fmt.Errorf("falha ao desserializar os dados do veículo: %s", err)
		}

		// Calcular a direção
		latitude, err := strconv.ParseFloat(latitudeStr, 64)
		if err != nil {
			return fmt.Errorf("falha ao converter latitude: %s", err)
		}
		longitude, err := strconv.ParseFloat(longitudeStr, 64)
		if err != nil {
			return fmt.Errorf("falha ao converter longitude: %s", err)
		}
		previousLatitude, err := strconv.ParseFloat(previousVehicleData.Latitude, 64)
		if err != nil {
			return fmt.Errorf("falha ao converter latitude anterior: %s", err)
		}
		previousLongitude, err := strconv.ParseFloat(previousVehicleData.Longitude, 64)
		if err != nil {
			return fmt.Errorf("falha ao converter longitude anterior: %s", err)
		}
		direction := CalculateBearing(previousLatitude, previousLongitude, latitude, longitude)
		// Criar a estrutura VehicleData
		vehicleData := VehicleData{
			Latitude:  latitudeStr,
			Longitude: longitudeStr,
			Direction: fmt.Sprintf("%f", direction),
			Speed:     speedStr,
			AccelX:    accelXstr,
			AccelY:    accelYstr,
			AccelZ:    accelZstr,
			// RPM:       rpmStr,
			TimeStamp: unixTimestamp,
			Flag:      flag,
		}

		// Armazenar os dados no ledger
		vehicleDataJSON, err := json.Marshal(vehicleData)
		if err != nil {
			return fmt.Errorf("falha ao serializar os dados do veículo: %s", err)
		}

		return ctx.GetStub().PutState(idcarro, vehicleDataJSON)
		// if err != nil {
		// 	return fmt.Errorf("falha ao armazenar os dados do veículo no ledger: %s", err)
		// }

	}
	// ELSE
	// Se não há dados anteriores, armazenar apenas a latitude e longitude
	newVehicleData := VehicleData{
		Latitude:  latitudeStr,
		Longitude: longitudeStr,
		Direction: "0", // inicialmente, a direção pode ser 0
		Speed:     speedStr,
		AccelX:    accelXstr,
		AccelY:    accelYstr,
		AccelZ:    accelZstr,
		// RPM:       rpmStr,
		TimeStamp: unixTimestamp,
		Flag:      flag,
	}

	// Armazenar os dados no ledger
	newVehicleDataJSON, err := json.Marshal(newVehicleData)
	if err != nil {
		return fmt.Errorf("falha ao serializar os dados do veículo: %s", err)
	}

	return ctx.GetStub().PutState(idcarro, newVehicleDataJSON)
	// if err != nil {
	// 	return fmt.Errorf("falha ao armazenar os dados do veículo no ledger: %s", err)
	// }

	// return nil
}

// StoreSimpleVehicleData armazena os dados do veículo no ledger sem verificação extra
// func (s *SmartContract) StoreSimpleVehicleData(ctx contractapi.TransactionContextInterface, idcarro string, unixTimestamp string, latitudeStr string, longitudeStr string, speedStr, direction string, accelXstr string, accelYstr string, accelZstr string, rpmStr string, flag string) error {
func (s *SmartContract) StoreSimpleVehicleData(ctx contractapi.TransactionContextInterface, idcarro string, unixTimestamp string, latitudeStr string, longitudeStr string, speedStr, direction string, accelXstr string, accelYstr string, accelZstr string, flag string) error {
	vehicleData := VehicleData{
		Latitude:  latitudeStr,
		Longitude: longitudeStr,
		Direction: direction,
		Speed:     speedStr,
		AccelX:    accelXstr,
		AccelY:    accelYstr,
		AccelZ:    accelZstr,
		// RPM:       rpmStr,
		TimeStamp: unixTimestamp,
		Flag:      flag,
	}

	vehicleDataJSON, err := json.Marshal(vehicleData)
	if err != nil {
		return fmt.Errorf("falha ao serializar os dados do veículo: %s", err)
	}

	return ctx.GetStub().PutState(idcarro, vehicleDataJSON)
}

// InitVehicleWallet inicializa uma carteira de veículo com quantidade inicial de créditos 0
func (s *SmartContract) CreateVehicleWallet(ctx contractapi.TransactionContextInterface, idcarro string) error {
	// verifique se a carteira já existe
	indexName := "WALLET"
	compositeKey, err := ctx.GetStub().CreateCompositeKey(indexName, []string{idcarro})
	if err != nil {
		return err
	}

	// verificação de integridade referencial
	vehicleWalletAsBytes, err := ctx.GetStub().GetState(compositeKey)
	if err != nil {
		return fmt.Errorf("failed to read from world state: %s", err)
	}

	if vehicleWalletAsBytes != nil {
		return fmt.Errorf("carteira já existe para o veiculo %s", idcarro)
	}

	vehicleWallet := VehicleWallet{
		Credits: 0,
	}

	vehicleWalletJSON, err := json.Marshal(vehicleWallet)
	if err != nil {
		return fmt.Errorf("falha ao serializar a carteira do veículo: %s", err)
	}

	return ctx.GetStub().PutState(compositeKey, vehicleWalletJSON)
}

// Inicializa um histórico de infrações de um veículo
func (s *SmartContract) CreateUserInfractions(ctx contractapi.TransactionContextInterface, idcarro string) error {
	// verifique se o histórico de infrações já existe
	indexName := "USERINFRACTIONS"
	compositeKey, err := ctx.GetStub().CreateCompositeKey(indexName, []string{idcarro})
	if err != nil {
		return err
	}

	// verificação de integridade referencial
	userInfractionsAsBytes, err := ctx.GetStub().GetState(compositeKey)
	if err != nil {
		return fmt.Errorf("failed to read from world state: %s", err)
	}

	if userInfractionsAsBytes != nil {
		return fmt.Errorf("histórico de infrações já existe para o veiculo %s", idcarro)
	}

	userInfractions := UserInfractions{
		AnomalousAccelerationOccurences: 0,
		AnomalousAccelerationTimestamps: []string{},
		RestlessDrivingOccurences:       0,
		RestlessDrivingTimestamps:       []string{},
		SharpTurnOccurences:             0,
		SharpTurnTimestamps:             []string{},
		ZigZagOccurences:                0,
		ZigZagTimestamps:                []string{},
	}

	userInfractionsJSON, err := json.Marshal(userInfractions)
	if err != nil {
		return fmt.Errorf("falha ao serializar o histórico do veículo: %s", err)
	}

	return ctx.GetStub().PutState(compositeKey, userInfractionsJSON)
}

// QueryVehicleWallet consulta a carteira do veículo armazenada no ledger
func (s *SmartContract) QueryVehicleWallet(ctx contractapi.TransactionContextInterface, idcarro string) (*VehicleWallet, error) {
	indexName := "WALLET"
	compositeKey, err := ctx.GetStub().CreateCompositeKey(indexName, []string{idcarro})
	if err != nil {
		return nil, err
	}

	vehicleWalletAsBytes, err := ctx.GetStub().GetState(compositeKey)
	if err != nil {
		return nil, fmt.Errorf("failed to read from world state: %s", err)
	}

	if vehicleWalletAsBytes == nil {
		return nil, fmt.Errorf("carteira do veículo não encontrada")
	}

	var vehicleWallet VehicleWallet
	err = json.Unmarshal(vehicleWalletAsBytes, &vehicleWallet)
	if err != nil {
		return nil, fmt.Errorf("falha ao desserializar a carteira do veículo: %s", err)
	}

	log.Printf("Créditos: %v", vehicleWallet.Credits)
	// repeat string
	// fmt.Println(strings.Repeat("=", 10))

	return &vehicleWallet, nil
}

// QueryUserInfractions consulta o histórico de infrações do usuário armazenada no ledger
func (s *SmartContract) QueryUserInfractions(ctx contractapi.TransactionContextInterface, idcarro string) (*UserInfractions, error) {
	indexName := "USERINFRACTIONS"
	compositeKey, err := ctx.GetStub().CreateCompositeKey(indexName, []string{idcarro})
	if err != nil {
		return nil, err
	}

	userInfractionsAsBytes, err := ctx.GetStub().GetState(compositeKey)
	if err != nil {
		return nil, fmt.Errorf("failed to read from world state: %s", err)
	}

	if userInfractionsAsBytes == nil {
		return nil, fmt.Errorf("histórico de infrações do usuário não encontrado")
	}

	var userInfractions UserInfractions
	err = json.Unmarshal(userInfractionsAsBytes, &userInfractions)
	if err != nil {
		return nil, fmt.Errorf("falha ao desserializar o histórico de infrações do usuário: %s", err)
	}

	// log.Printf("Créditos: %v", userInfractions.Credits)
	// repeat string
	// fmt.Println(strings.Repeat("=", 10))

	return &userInfractions, nil
}

func (s *SmartContract) TestRichQuery(ctx contractapi.TransactionContextInterface, query string) error {
	queryString := fmt.Sprintf(`{"selector":{"timestamp":"%s"}}`, query) // ABC4444
	resultsIterator, err := ctx.GetStub().GetQueryResult(queryString)

	if err != nil {
		return fmt.Errorf("falha ao consultar o ledger: %s", err)
	}

	defer resultsIterator.Close()

	for resultsIterator.HasNext() {
		queryResponse, err := resultsIterator.Next()
		if err != nil {
			return fmt.Errorf("falha ao iterar sobre os resultados da consulta: %s", err)
		}

		var vehicleData VehicleData
		err = json.Unmarshal(queryResponse.Value, &vehicleData)
		if err != nil {
			return fmt.Errorf("falha ao desserializar os dados do veículo: %s", err)
		}

		// log.Println("Registro: ", &vehicleData)
	}

	return nil
}

// Limpa registros de infrações de mais de 80 minutos atrás do histórico do condutor
func (u *UserInfractions) CleanOldInfractions(currentTime string) error {

	convertedCurrentTime, err := ConvertUnixTimestampToTimeObject(currentTime)
	if err != nil {
		return fmt.Errorf("erro ao converter timestamp: %v", err.Error())
	}
	threshold := convertedCurrentTime.Add(-80 * time.Minute) // 80 minutos antes de currentTime
	tamanho := len(currentTime)

	var (
		newAnomalousTimestamps       []string
		newRestlessDrivingTimestamps []string
		newSharpTurnTimestamps       []string
		newZigZagTimestamps          []string
	)

	for i := 0; i < tamanho; i++ {
		anomalousAccelerationTimestamp, err := ConvertUnixTimestampToTimeObject(u.AnomalousAccelerationTimestamps[i])
		if err != nil {
			return fmt.Errorf("erro ao converter timestamp: %v", err.Error())
		}
		if anomalousAccelerationTimestamp.Before(threshold) {
			newAnomalousTimestamps = append(newAnomalousTimestamps, u.AnomalousAccelerationTimestamps[i])
		}

		restlessDrivingTimestamp, err := ConvertUnixTimestampToTimeObject(u.RestlessDrivingTimestamps[i])
		if err != nil {
			return fmt.Errorf("erro ao converter timestamp: %v", err.Error())
		}
		if restlessDrivingTimestamp.Before(threshold) {
			newRestlessDrivingTimestamps = append(newRestlessDrivingTimestamps, u.RestlessDrivingTimestamps[i])
		}

		sharpTurnTimestamp, err := ConvertUnixTimestampToTimeObject(u.SharpTurnTimestamps[i])
		if err != nil {
			return fmt.Errorf("erro ao converter timestamp: %v", err.Error())
		}
		if sharpTurnTimestamp.Before(threshold) {
			newSharpTurnTimestamps = append(newSharpTurnTimestamps, u.SharpTurnTimestamps[i])
		}

		zigZagTimestamp, err := ConvertUnixTimestampToTimeObject(u.ZigZagTimestamps[i])
		if err != nil {
			return fmt.Errorf("erro ao converter timestamp: %v", err.Error())
		}
		if zigZagTimestamp.Before(threshold) {
			newZigZagTimestamps = append(newZigZagTimestamps, u.ZigZagTimestamps[i])
		}

	}

	u.AnomalousAccelerationOccurences = len(newAnomalousTimestamps)
	u.AnomalousAccelerationTimestamps = newAnomalousTimestamps

	u.RestlessDrivingOccurences = len(newRestlessDrivingTimestamps)
	u.RestlessDrivingTimestamps = newRestlessDrivingTimestamps

	u.SharpTurnOccurences = len(newSharpTurnTimestamps)
	u.SharpTurnTimestamps = newSharpTurnTimestamps

	u.ZigZagOccurences = len(newZigZagTimestamps)
	u.ZigZagTimestamps = newZigZagTimestamps

	return nil
}

// // Limpa registros de infrações de mais de 80 minutos atrás do histórico do condutor
// func (u *UserInfractions) CleanOldInfractions(currentTime string) error {

// 	convertedCurrentTime, err := ConvertUnixTimestampToTimeObject(currentTime)
// 	if err != nil {
// 		return fmt.Errorf("erro ao converter timestamp: %v", err.Error())
// 	}
// 	threshold := convertedCurrentTime.Add(-80*time.Minute)// 80 minutos antes

// 	// Limpar ZigZag
// 	newZigZagTimestamps := []string{}
// 	for _, ts := range u.ZigZagTimestamps {
// 		timestamp, err := ConvertUnixTimestampToTimeObject(ts)
// 		if err != nil {
// 			return fmt.Errorf("erro ao converter timestamp: %v", err.Error())
// 		}
// 		if timestamp.Before(threshold) {
// 			newZigZagTimestamps = append(newZigZagTimestamps, ts)
// 		}
// 	}
// 	u.ZigZagOccurences = len(newZigZagTimestamps)
// 	u.ZigZagTimestamps = newZigZagTimestamps

// 	// Limpar AnomalousAcceleration
// 	newAnomalousTimestamps := []string{}
// 	for _, ts := range u.AnomalousAccelerationTimestamps {
// 		timestamp, err := ConvertUnixTimestampToTimeObject(ts)
// 		if err != nil {
// 			return fmt.Errorf("erro ao converter timestamp: %v", err.Error())
// 		}
// 		if timestamp.Before(threshold) {
// 			newAnomalousTimestamps = append(newAnomalousTimestamps, ts)
// 		}
// 	}
// 	u.AnomalousAccelerationOccurences = len(newAnomalousTimestamps)
// 	u.AnomalousAccelerationTimestamps = newAnomalousTimestamps

// 	// Limpar SharpTurn
// 	newSharpTurnTimestamps := []string{}
// 	for _, ts := range u.SharpTurnTimestamps {
// 		timestamp, err := ConvertUnixTimestampToTimeObject(ts)
// 		if err != nil {
// 			return fmt.Errorf("erro ao converter timestamp: %v", err.Error())
// 		}
// 		if timestamp.Before(threshold) {
// 			newSharpTurnTimestamps = append(newSharpTurnTimestamps, ts)
// 		}
// 	}
// 	u.SharpTurnOccurences = len(newSharpTurnTimestamps)
// 	u.SharpTurnTimestamps = newSharpTurnTimestamps

// 	// Limpar RestlessDriving
// 	newRestlessDrivingTimestamps := []string{}
// 	for _, ts := range u.RestlessDrivingTimestamps {
// 		timestamp, err := ConvertUnixTimestampToTimeObject(ts)
// 		if err != nil {
// 			return fmt.Errorf("erro ao converter timestamp: %v", err.Error())
// 		}
// 		if timestamp.Before(threshold) {
// 			newRestlessDrivingTimestamps = append(newRestlessDrivingTimestamps, ts)
// 		}
// 	}
// 	u.RestlessDrivingOccurences = len(newRestlessDrivingTimestamps)
// 	u.RestlessDrivingTimestamps = newRestlessDrivingTimestamps

// 	return nil
// }

// Função para detectar aceleração anômala
func DetectAnomalousAcceleration(speedSlice []string, numPreviousAnomalousAcceleration int) (int, error, bool) {
	detection := false
	credits, _ := BonusAnomalousAcceleration(10, numPreviousAnomalousAcceleration)
	
	tamanho := len(speedSlice)
	valorfinal, err := strconv.ParseFloat(speedSlice[tamanho-1], 32)
	if err != nil {
		return 0, fmt.Errorf("erro ao converter valor para float32: %v", err.Error()), false
	}
	valorinicial, err := strconv.ParseFloat(speedSlice[0], 32)
	if err != nil {
		return 0, fmt.Errorf("erro ao converter valor para float32: %v", err.Error()), false
	}

	deltaSpeed := math.Abs(valorfinal - valorinicial)

	// Se a variação de velocidade for maior que 30 km/h em menos de 5 segundos
	if deltaSpeed > 30 {
		detection = true
		penalty, err := PenaltyAnomalousAcceleration(50, numPreviousAnomalousAcceleration)
		if err != nil {
			fmt.Println("Erro ao aplicar penalide:", err)
		}
		credits = -penalty
	}

	log.Print("Aceleração anômala: ", detection)
	return credits, nil, detection
}

// Função para verificar pausas para descanso durante a viagem.
func DetectRestlessDriving(latitudeSlice []string, longitudeSlice []string, timestampSlice []string, numPreviousRestlessDriving int) (int, error, bool) {
	credits, _ := BonusRestlessDriving(200, numPreviousRestlessDriving)
	detection := false // Indica que houve condução COM pausas
	tamanho := len(timestampSlice)

	// Percorrendo os slices em ordem crescente no tempo
	for i := 0; i < tamanho; i++ {

		// Convertendo strings de timestamp em objetos de hora
		pastTime, err := ConvertUnixTimestampToTimeObject(timestampSlice[i])
		if err != nil {
			return 0, fmt.Errorf("erro ao converter timestamp: %v", err.Error()), false
		}
		recentTime, err := ConvertUnixTimestampToTimeObject(timestampSlice[i+1])
		if err != nil {
			return 0, fmt.Errorf("erro ao converter timestamp: %v", err.Error()), false
		}
		elapsedSeconds := recentTime.Sub(pastTime).Minutes()

		// Definindo latitudes e longitudes atuais e anteriores
		pastLatitude := latitudeSlice[i]
		recentLatitude := latitudeSlice[i+1]
		pastLongitude := longitudeSlice[i]
		recentLongitude := longitudeSlice[i+1]

		// Verificar se houve alguma pausa
		if pastLatitude == recentLatitude && pastLongitude == recentLongitude && elapsedSeconds >= 8 { // Significa que houve uma pausa de pelo menos 8 minutos, logo interrompe a detecção aqui
			break
		} else { // Significa que não houve pausa e a penalidade é aplicada
			penalty, err := PenaltyRestlessDriving(80, numPreviousRestlessDriving)
			if err != nil {
				fmt.Println("Erro ao aplicar penalidade: ", err)
			}
			credits = -penalty
			detection = true // Isso significa que foi detectado que NÃO houve pausa
		}
	}

	log.Printf("Condução sem pausas: %v", detection)
	return credits, nil, detection
}

// Função que complementa a DetectRestlessDriving para casos em que o motorista NÃO para para descansar
func DetectHarshRestlessDriving(latitudeSlice []string, longitudeSlice []string, timestampSlice []string, numPreviousRestlessDriving int) (int, error, bool) {
	credits := 70      // Só ganha 70 quando finalmente faz pausa e não tem bônus
	detection := false // Indica que houve condução COM pausas
	tamanho := len(timestampSlice)

	// Percorrendo os slices em ordem crescente no tempo
	for i := 0; i < tamanho; i++ {

		// Convertendo strings de timestamp em objetos de hora
		pastTime, err := ConvertUnixTimestampToTimeObject(timestampSlice[i])
		if err != nil {
			return 0, fmt.Errorf("erro ao converter timestamp: %v", err.Error()), false
		}
		recentTime, err := ConvertUnixTimestampToTimeObject(timestampSlice[i+1])
		if err != nil {
			return 0, fmt.Errorf("erro ao converter timestamp: %v", err.Error()), false
		}
		elapsedSeconds := recentTime.Sub(pastTime).Minutes()

		// Definindo latitudes e longitudes atuais e anteriores
		pastLatitude := latitudeSlice[i]
		recentLatitude := latitudeSlice[i+1]
		pastLongitude := longitudeSlice[i]
		recentLongitude := longitudeSlice[i+1]

		// Verificar se houve alguma pausa
		if pastLatitude == recentLatitude && pastLongitude == recentLongitude && elapsedSeconds >= 8 { // Significa que houve uma pausa de pelo menos 8 minutos, logo interrompe a detecção aqui
			break
		} else { // Significa que não houve pausa de novo e a penalidade é aplicada
			penalty := 50
			credits = -penalty
			detection = true // Isso significa que foi detectado que NÃO houve pausa
		}
	}

	log.Printf("Condução sem pausas de novo: %v", detection)
	return credits, nil, detection
}

// Função para detectar mudanças bruscas de direção
func DetectSharpTurn(speed string, direction string, numPreviousSharpTurn int) (int, error, bool) {
	credits, _ := BonusSharpTurn(10, numPreviousSharpTurn)
	detection := false

	// Conversão da direção para float
	directionFloat, err := strconv.ParseFloat(direction, 64)
	if err != nil {
		fmt.Println("Erro ao converter direção:", err)
	}

	// debug
	// log.Printf("Direção: %v", direction)
	// log.Printf("Velocidade (MK2): %v", speed)

	//converter
	speedFloat, err := strconv.ParseFloat(speed, 64)
	if err != nil {
		fmt.Println("Erro ao converter velocidade:", err)
	}

	// Se a direção for maior que 0.7 rad e a velocidade maior que 30 km/h
	if directionFloat == 0 {
		log.Printf("Direção neutra")
	} else if directionFloat < 0.7 && speedFloat > 30 {
		penalty, err := PenaltySharpTurn(30, numPreviousSharpTurn) // Penalidade + aumento, falta definir numPreviousSharpTurn
		if err != nil {
			fmt.Println("Erro ao aplicar penalide:", err)
		}
		credits = -penalty
		detection = true
	}

	log.Printf("Curva acentuada: %v", detection)
	return credits, nil, detection
}

// // Função para detectar comportamento de zigue-zague
// func DetectZigZag(accelXSlice []string, accelYSlice []string, accelZSlice []string, numPreviousZigZag int) (int, error, bool) {

// 	// pegar cerca de 10 segundos de linhas
// 	// então, comparar segundo[9] com segundo [8] OU com segundo[9] com segundo[7]
// 	// ex: comparar o sinal atual com o de 2 segundos antes

// 	var zigzagCount int
// 	credits, _ := BonusZigZag(10, numPreviousZigZag)
// 	detection := false

// 	// ele vai ler de tras para frente (do mais antigo até o mais recente)
// 	for i := len(accelXSlice) - 1; i > 0; i-- {
// 		// Recupera últimos valores de aceleração para detectar zigue-zague

		// parametros removidos
		// currentAccelX := accelXSlice[i]
		// nextAccelX := accelXSlice[i-1]
		// nextAccelY := accelYSlice[i-1]

		currentAccelY, err := strconv.ParseFloat(accelYSlice[i], 64)
		if err != nil {
			log.Printf("Erro ao converter aceleração Y: %v", err)
			continue
		}
		currentAccelZ := accelZSlice[i]
		nextAccelZ := accelZSlice[i-1]

// 		// Compara os valores para detectar zigue-zague
// 		if currentAccelY >= 0.0080 && currentAccelZ != nextAccelZ {
// 			zigzagCount++
// 		}
// 	}

// 	// Se o número de zigue-zagues for maior ou igual a 3, aplica penalização
// 	if zigzagCount >= 3 {
// 		// Aplique penalização na carteira do veículo
// 		penalty, err := PenaltyZigZag(40, numPreviousZigZag)
// 		if err != nil {
// 			fmt.Println("Erro ao aplicar pendalide:", err)
// 		}
// 		credits = -penalty
// 		detection = true
// 	}

// 	// Caso contrário, o veículo está dirigindo de forma aceitável
// 	log.Printf("Zigue-zague: %v", detection)
// 	return credits, nil, detection
// }

func CalculateBearing(lat1, lon1, lat2, lon2 float64) float64 {
	// CalculateBearing calcula a direção entre dois pontos geográficos
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

func (s *SmartContract) QueryVehicleData(ctx contractapi.TransactionContextInterface, idcarro string) (*VehicleData, error) {
	// QueryVehicleData consulta os dados do veículo armazenados no ledger
	vehicleDataJSON, err := ctx.GetStub().GetState(idcarro)
	if err != nil {
		return nil, fmt.Errorf("falha ao ler os dados do veículo do ledger: %s", err)
	}
	if vehicleDataJSON == nil {
		return nil, fmt.Errorf("dados do veículo não encontrados")
	}

	var vehicleData VehicleData
	err = json.Unmarshal(vehicleDataJSON, &vehicleData)
	if err != nil {
		return nil, fmt.Errorf("falha ao desserializar os dados do veículo: %s", err)
	}

	return &vehicleData, nil
}

func (s *SmartContract) GiveCredits(ctx contractapi.TransactionContextInterface, idcarro string, credits int) error {
	indexName := "WALLET"
	compositeKey, err := ctx.GetStub().CreateCompositeKey(indexName, []string{idcarro})
	if err != nil {
		return fmt.Errorf("erro ao criar chave composta para a carteira: %s", err)
	}

	vehicleWalletAsBytes, err := ctx.GetStub().GetState(compositeKey)
	if err != nil {
		return fmt.Errorf("erro ao recuperar o saldo atual da carteira: %s", err)
	}

	if vehicleWalletAsBytes == nil {
		return fmt.Errorf("carteira do veículo não encontrada")
	}

	var vehicleWallet VehicleWallet
	err = json.Unmarshal(vehicleWalletAsBytes, &vehicleWallet)
	if err != nil {
		return fmt.Errorf("falha ao desserializar a carteira do veículo: %s", err)
	}

	vehicleWallet.Credits += credits

	vehicleWalletJSON, err := json.Marshal(vehicleWallet)
	if err != nil {
		return fmt.Errorf("falha ao serializar a carteira do veículo: %s", err)
	}

	return ctx.GetStub().PutState(compositeKey, vehicleWalletJSON)
}

// Função para bonificar o usuário quando ele evita AnomalousAcceleration
func BonusAnomalousAcceleration(baseValue int, numPreviousOccurences int) (bonus int, err error) {

	// Checando se os parâmetros de entrada estão corretos
	if numPreviousOccurences < 0 {
		return 0, fmt.Errorf("número de ocorrências prévias deve ser 0 ou maior")
	} else if baseValue < 0 {
		return 0, fmt.Errorf("valor base deve ser 0 ou maior")
	}

	// Definindo o valor base
	if baseValue == 0 {
		baseValue = 10
	}

	bonus = baseValue
	if numPreviousOccurences <= 1 {
		bonus = bonus + bonus/10 // aumenta 10%
	}

	return bonus, nil
}

// Função para bonificar o usuário quando ele evita RestlessDriving
func BonusRestlessDriving(baseValue int, numPreviousOccurences int) (bonus int, err error) {

	// Checando se os parâmetros de entrada estão corretos
	if numPreviousOccurences < 0 {
		return 0, fmt.Errorf("número de ocorrências prévias deve ser 0 ou maior")
	} else if baseValue < 0 {
		return 0, fmt.Errorf("valor base deve ser 0 ou maior")
	}

	// Definindo o valor base
	if baseValue == 0 {
		baseValue = 10
	}

	bonus = baseValue
	if numPreviousOccurences <= 1 {
		bonus = bonus + bonus/10 // aumenta 10%
	}

	return bonus, nil
}

// Função para bonificar o usuário quando ele evita SharpTurn
func BonusSharpTurn(baseValue int, numPreviousOccurences int) (bonus int, err error) {

	// Checando se os parâmetros de entrada estão corretos
	if numPreviousOccurences < 0 {
		return 0, fmt.Errorf("número de ocorrências prévias deve ser 0 ou maior")
	} else if baseValue < 0 {
		return 0, fmt.Errorf("valor base deve ser 0 ou maior")
	}

	// Definindo o valor base
	if baseValue == 0 {
		baseValue = 10
	}

	bonus = baseValue
	if numPreviousOccurences <= 1 {
		bonus = bonus + bonus/10 // aumenta 10%
	}

	return bonus, nil
}

// Função para bonificar o usuário quando ele evita ZigZag
func BonusZigZag(baseValue int, numPreviousOccurences int) (bonus int, err error) {

	// Checando se os parâmetros de entrada estão corretos
	if numPreviousOccurences < 0 {
		return 0, fmt.Errorf("número de ocorrências prévias deve ser 0 ou maior")
	} else if baseValue < 0 {
		return 0, fmt.Errorf("valor base deve ser 0 ou maior")
	}

	// Definindo o valor base
	if baseValue == 0 {
		baseValue = 10
	}

	bonus = baseValue
	if numPreviousOccurences <= 1 {
		bonus = bonus + bonus/10 // aumenta 10%
	}

	return bonus, nil
}

// Função para penalizar reincidências de AnomalousAcceleration
func PenaltyAnomalousAcceleration(baseValue int, numPreviousOccurences int) (penalty int, err error) {

	// Checando se os parâmetros de entrada estão corretos
	if numPreviousOccurences < 0 {
		return 0, fmt.Errorf("número de ocorrências prévias deve ser 0 ou maior")
	} else if baseValue < 0 {
		return 0, fmt.Errorf("valor base deve ser 0 ou maior")
	}

	// Definindo o valor base
	if baseValue == 0 {
		baseValue = 50
	}

	// Aplicando a penalização (essa daqui é só um exemplo)
	if numPreviousOccurences < 10 {
		penalty = baseValue + (baseValue*numPreviousOccurences)/10
	} else {
		penalty = int(baseValue * 2)
	}

	return penalty, nil
}

// Função para penalizar reincidências de RestlessDriving
func PenaltyRestlessDriving(baseValue int, numPreviousOccurences int) (penalty int, err error) {

	// Checando se os parâmetros de entrada estão corretos
	if numPreviousOccurences < 0 {
		return 0, fmt.Errorf("número de ocorrências prévias deve ser 0 ou maior")
	} else if baseValue < 0 {
		return 0, fmt.Errorf("valor base deve ser 0 ou maior")
	}

	// Definindo o valor base
	if baseValue == 0 {
		baseValue = 30
	}

	// Aplicando a penalização (essa daqui é só um exemplo)
	if numPreviousOccurences < 10 {
		penalty = baseValue + (baseValue*numPreviousOccurences)/10
	} else {
		penalty = int(baseValue * 2)
	}

	return penalty, nil
}

// Função para penalizar reincidências de SharpTurn
func PenaltySharpTurn(baseValue int, numPreviousOccurences int) (penalty int, err error) {

	// Checando se os parâmetros de entrada estão corretos
	if numPreviousOccurences < 0 {
		return 0, fmt.Errorf("número de ocorrências prévias deve ser 0 ou maior")
	} else if baseValue < 0 {
		return 0, fmt.Errorf("valor base deve ser 0 ou maior")
	}

	// Definindo o valor base
	if baseValue == 0 {
		baseValue = 30
	}

	// Aplicando a penalização (essa daqui é só um exemplo)
	if numPreviousOccurences < 10 {
		penalty = baseValue + (baseValue*numPreviousOccurences)/10
	} else {
		penalty = int(baseValue * 2)
	}

	return penalty, nil
}

// Função para penalizar reincidências de ZigZag
func PenaltyZigZag(baseValue int, numPreviousOccurences int) (penalty int, err error) {

	// Checando se os parâmetros de entrada estão corretos
	if numPreviousOccurences < 0 {
		return 0, fmt.Errorf("número de ocorrências prévias deve ser 0 ou maior")
	} else if baseValue < 0 {
		return 0, fmt.Errorf("valor base deve ser 0 ou maior")
	}

	// Definindo o valor base
	if baseValue == 0 {
		baseValue = 40
	}

	// Aplicando a penalização (essa daqui é só um exemplo)
	if numPreviousOccurences < 10 {
		penalty = baseValue + (baseValue*numPreviousOccurences)/10
	} else {
		penalty = int(baseValue * 2)
	}

	return penalty, nil
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
