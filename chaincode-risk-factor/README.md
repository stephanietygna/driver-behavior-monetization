# Chaincode de fator de risco

Este chaincode recebe leituras de telemetria em JSON, aplica as tres metricas
comportamentais e calcula o fator de risco normalizado `R_i`.

Os pesos da calibração são derivados das porcentagens relativas de acidentes
das três funções consideradas no modelo: aceleração/desaceleração (16,12%),
curva acentuada (1,96%) e cansaço do condutor (61,77%). Como essas três
funções representam 79,85% do total da tabela de origem, elas são normalizadas
para somar 1 antes de compor a equação.

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

## Converter o CSV OBD do repositório

O programa cliente `cmd/obd-to-readings` extrai apenas `timestamp`, `lat`,
`lon` e `vehicle_speed` (km/h) do CSV. Ele não altera o arquivo original e
gera o JSON que será enviado ao chaincode.

```bash
cd chaincode-risk-factor
go run ./cmd/obd-to-readings \
  -input ../data/obd_clean.csv \
  -route obd-15-spin-trajeto-t1 \
  -output /tmp/obd-trajeto.json
```

O arquivo `obd_clean.csv` contém 2.753 leituras desse trajeto. O identificador
do trajeto que será gravado no ledger é definido separadamente na transação.

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

## Trajetos grandes pelo terminal

Para não ultrapassar o limite de argumentos do terminal, compacte o JSON com
gzip e base64 e invoque `CreateRiskAssessmentCompressed`:

```bash
COMPACTADO="$(gzip -c /tmp/obd-trajeto.json | base64 -w 0)"

kubectl hlf chaincode invoke \
  --config=resources/inmetro.yaml \
  --user=inmetro-admin-default \
  --peer=inmetro-peer0.default \
  --channel=demo \
  --chaincode=risk-factor \
  --fcn=CreateRiskAssessmentCompressed \
  --args=obd-15-spin-trajeto-t1 \
  --args="$COMPACTADO"
```

Essa opção calcula e armazena exatamente o mesmo resultado da transação
`CreateRiskAssessment`; a compactação existe apenas no transporte.
