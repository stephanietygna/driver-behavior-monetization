# Cliente de teste do fator de risco

Este é o programa que você executa na VM para testar o contrato, no mesmo
estilo do cliente do repositório DriveSmart-Monetization. Ele lê o CSV OBD,
mostra as leituras no terminal, e envia o trajeto completo ao chaincode.

O chaincode continua separado em `chaincode-risk-factor/main.go`: ele é o
programa que fica executando dentro da blockchain, e não deve ser iniciado
manualmente com `go run`.

## Executar

Com a rede e o chaincode `risk-factor` já ativos:

```bash
cd ~/driver-behavior-monetization/client/risk-factor
go run main.go -trip-id obd-15-spin-trajeto-t2
```

Use um `trip-id` novo em cada execução, porque um identificador já gravado no
ledger não pode ser reutilizado.

Para não imprimir as 2.753 leituras na tela, acrescente `-verbose=false`.

```bash
go run main.go -trip-id obd-15-spin-trajeto-t3 -verbose=false
```

O cliente realiza uma única transação `CreateRiskAssessmentCompressed` ao fim
da leitura. Isso evita criar uma transação por linha e mantém o cálculo de
`R_i` aplicado ao trajeto inteiro.

Ao final, o cliente mostra separadamente o número de acelerações anômalas e
curvas bruscas. Para fadiga, mostra o tempo de condução, o maior período
contínuo, o limite, o excesso em minutos e as componentes `B_i` e `E_i`; por
fim, mostra o score ponderado e o fator de risco final `R_i`.
