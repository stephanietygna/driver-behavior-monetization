# Teste standalone das 3 métricas (sem Fabric/blockchain)

Este programa testa isoladamente as 3 métricas formalizadas no estudo:
1. Aceleração/desaceleração anômala
2. Mudança brusca de direção
3. Fadiga por condução contínua

Não depende do `fabric-chaincode-go` nem do `fabric-contract-api-go` — só biblioteca padrão do Go. Ideia: validar a lógica e os limiares antes de integrar ao chaincode.

## Como rodar

```bash
cd metrics_test
go run . -csv=sample_data.csv
```

Ou, com seus próprios dados exportados do Excel para CSV:

```bash
go run . -csv=seus_dados.csv
```

## Formato esperado do CSV

Cabeçalho obrigatório (outras colunas extras são ignoradas):

```
timestamp,lat,lon,vehicle_speed,accel_x
```

- `timestamp`: formato `2006-01-02 15:04:05.000` (ex: `2024-11-30 09:57:01.535`), igual ao da sua planilha.
- `vehicle_speed`: em km/h.
- `accel_x`: em m/s² — **AJUSTE o limiar `AnomalousAccelThreshold` se a unidade real do seu sensor for diferente** (ver seção "Limiares" abaixo).

## `sample_data.csv`

Arquivo sintético incluído para você ver o programa funcionando de cara. Ele contém, na ordem:
- Condução normal (baseline).
- Um evento de frenagem brusca (deve disparar `ACEL_ANOMALA`).
- Uma curva de ~90° em velocidade alta (deve disparar `CURVA_BRUSCA`).
- Uma parada curta de 3 min (não deve resetar o contador de fadiga, pois é menor que o limiar de pausa válida).
- Condução contínua até passar de 80 min (deve disparar `FADIGA`).
- Uma parada longa de 6 min (deve resetar o contador de fadiga).
- Mais condução após a pausa, sem ultrapassar 80 min de novo (não deve disparar `FADIGA` outra vez).

Rode com esse arquivo primeiro para confirmar que o comportamento bate com o esperado antes de usar dados reais.

## Limiares (no topo de `main.go`)

```go
AnomalousAccelThreshold  = 0.833 // A, em m/s² — equivale a "30 km/h em 10s"
AnomalousAccelMaxWindow  = 10 * time.Second // T

SharpTurnAngleThreshold = 0.7  // θ_limite, em radianos
SharpTurnSpeedThreshold = 30.0 // v_limite, em km/h

FatigueThreshold = 80 * time.Minute // T da fadiga

StoppedSpeedThreshold = 3.0            // km/h abaixo disso = "parado"
MinValidPauseDuration = 5 * time.Minute // tempo mínimo parado para contar como pausa real
```

**Pendências que você ainda precisa confirmar e ajustar aqui:**
- `AnomalousAccelThreshold`: depende de você confirmar a unidade real do `accel_x` do seu sensor (m/s², g, etc). Se não for m/s², a conversão de 0,833 não vale e o valor precisa ser recalculado.
- `MinValidPauseDuration`: o valor de 5 minutos é um placeholder — ajuste para o valor que o estudo/orientador confirmar.

## Saída

O programa imprime cada evento detectado com timestamp, tipo de métrica e detalhes numéricos (ex: qual foi o Δa, Δθ, ou quantos minutos de condução contínua). Isso permite comparar visualmente contra o que você espera que aconteça nos seus dados reais, sem precisar subir nada na blockchain.
