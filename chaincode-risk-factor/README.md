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

## Preparacao local

```powershell
cd chaincode-risk-factor
go mod tidy
go test ./...
```

Para empacotar e implantar, use o ciclo de vida do chaincode da rede Fabric que
voce estiver usando. O nome do contrato exposto sera `RiskContract`.
