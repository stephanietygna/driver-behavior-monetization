# Chaincode de fator de risco

Este chaincode recebe leituras de telemetria em JSON, aplica as tres metricas
comportamentais e calcula o fator de risco normalizado `R_i`.

## Transacoes

- `EvaluateTripRisk(readingsJSON)`: calcula o resultado sem gravar no ledger.
- `CreateRiskAssessment(tripID, readingsJSON)`: calcula e grava o resultado.
- `ReadRiskAssessment(tripID)`: consulta um resultado ja gravado.

`CreateRiskAssessment` guarda as metricas, o fator de risco e a calibracao no
ledger. O mesmo `tripID` nao pode ser gravado duas vezes.

## Formato das leituras

O cliente deve converter o CSV para JSON antes da transacao. Exemplo:

```json
[
  {
    "timestamp": "2026-09-14T10:00:00Z",
    "lat": -22.9000,
    "lon": -43.2000,
    "vehicleSpeed": 50.0
  },
  {
    "timestamp": "2026-09-14T11:21:00Z",
    "lat": -22.9000,
    "lon": -43.2000,
    "vehicleSpeed": 50.0
  }
]
```

## Execução na rede deste repositório (CCAS)

Este projeto usa **Chaincode as a Service**. Por isso, o contrato é compilado
em uma imagem Docker e o peer se conecta ao processo pela porta `9999`.

1. Na VM Ubuntu, entre na pasta `chaincode-risk-factor` e construa a imagem:

```bash
docker build -t SEU_USUARIO_DOCKER/risk-factor:1.0 .
docker push SEU_USUARIO_DOCKER/risk-factor:1.0
```

2. Ao gerar o pacote CCAS, use `risk-factor` como nome e rótulo. O comando
`inmetro ccas` do repositório gera o `connection.json` e a configuração do
serviço automaticamente; não é necessário editá-los manualmente.

3. Depois de instalar o pacote, calcule o `PACKAGE_ID`. Ele deve ser informado
como `CHAINCODE_ID` pelo comando `externalchaincode sync`; não o defina
manualmente antes desse passo.

4. Instale, aprove e faça o commit no canal `demo`, usando o nome
`risk-factor`, versão `1.0` e sequência `1`. O roteiro existente em
`../ccas/README.md` mostra esses comandos; basta substituir os nomes da imagem
e do chaincode.

## Preparação local

```powershell
cd chaincode-risk-factor
go mod tidy
go test ./...
```

O nome do contrato exposto será `RiskContract`.
