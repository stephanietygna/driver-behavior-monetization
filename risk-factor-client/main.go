// risk-factor-client envia um arquivo de leituras JSON ao chaincode RiskContract.
// Diferente do kubectl, ele lê o arquivo diretamente e evita o limite de tamanho
// de argumentos do terminal ao testar trajetos OBD grandes.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/hyperledger/fabric-sdk-go/pkg/client/channel"
	"github.com/hyperledger/fabric-sdk-go/pkg/core/config"
	"github.com/hyperledger/fabric-sdk-go/pkg/fabsdk"
)

func main() {
	configPath := flag.String("config", "../resources/inmetro.yaml", "arquivo de conexão da rede Fabric")
	inputPath := flag.String("input", "/tmp/obd-trajeto.json", "arquivo JSON de leituras")
	tripID := flag.String("trip-id", "", "identificador único do trajeto no ledger")
	channelName := flag.String("channel", "demo", "canal Fabric")
	chaincodeName := flag.String("chaincode", "risk-factor", "nome do chaincode")
	userName := flag.String("user", "inmetro-admin-default", "identidade Fabric")
	mspID := flag.String("msp", "INMETROMSP", "MSP da organização")
	flag.Parse()

	if *tripID == "" {
		fatal("-trip-id é obrigatório")
	}

	readings, err := os.ReadFile(*inputPath)
	if err != nil {
		fatal("não foi possível ler JSON do trajeto: %v", err)
	}
	if !json.Valid(readings) {
		fatal("o arquivo de leituras não contém JSON válido")
	}

	// Esta é a mesma configuração e identidade usadas por `kubectl hlf`.
	sdk, err := fabsdk.New(config.FromFile(*configPath))
	if err != nil {
		fatal("não foi possível iniciar SDK Fabric: %v", err)
	}
	defer sdk.Close()

	context := sdk.ChannelContext(*channelName, fabsdk.WithUser(*userName), fabsdk.WithOrg(*mspID))
	client, err := channel.New(context)
	if err != nil {
		fatal("não foi possível criar cliente do canal: %v", err)
	}

	// O conteúdo do arquivo vira o segundo argumento da transação, sem passar pelo shell.
	response, err := client.Execute(channel.Request{
		ChaincodeID: *chaincodeName,
		Fcn:         "CreateRiskAssessment",
		Args:        [][]byte{[]byte(*tripID), readings},
	})
	if err != nil {
		fatal("a transação foi rejeitada: %v", err)
	}

	fmt.Printf("Transação confirmada: %s\n", response.TransactionID)
	fmt.Println(string(response.Payload))
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
