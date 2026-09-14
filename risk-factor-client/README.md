# Cliente de envio do fator de risco

Este cliente lê um arquivo JSON de telemetria e o envia ao chaincode
`risk-factor`. Ele existe para testar arquivos OBD grandes sem atingir o limite
de argumentos do terminal.

```bash
cd risk-factor-client
go mod tidy
go run . \
  -config ../resources/inmetro.yaml \
  -input /tmp/obd-trajeto.json \
  -trip-id obd-15-spin-trajeto-t1
```

O `trip-id` é imutável: para executar novamente com o mesmo arquivo, escolha
outro identificador, por exemplo `obd-15-spin-trajeto-t1-v2`.
