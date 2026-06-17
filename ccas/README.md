# Tutorial de instalação de de Chaincode Externo (CCAS)

- O chaincode asset-external está aqui apenas como um exemplo de como ele deve estar programado para ser enviado ao [Docker.io](../docs/CCAS_CREATION.md), e também para ver suas funções
- No momento de instalar, ele ira buscar o chaincode no docker.io ao invés de buscar na pasta local

### Requisitos

- Peers na versão mais recente (2.5+)
- Versão mais recente do HLF Operator (1.10+)

## Configuração Inicial
Siga o tutorial como demonstrado no README [anterior](../README.md). Entretanto, atualize o HLF Operator para a versão mais recente:
```bash
helm repo add kfs https://kfsoftware.github.io/hlf-helm-charts --force-update
helm upgrade --install hlf-operator --version=1.10.0 -- kfs/hlf-operator
```

E no momento de realizar o deploy do peer, utilize este comando faça com o comando a seguir:

```bash
export PEER_IMAGE=hyperledger/fabric-peer
export PEER_VERSION=2.5.5

kubectl hlf peer create --statedb=$DATABASE --image=$PEER_IMAGE --version=$PEER_VERSION --storage-class=$STORAGE_CLASS --enroll-id=peer --mspid=INMETROMSP \
        --enroll-pw=peerpw --capacity=5Gi --name=inmetro-peer0 --ca-name=inmetro-ca.default \
        --hosts=peer0-inmetro.localho.st --istio-port=443

kubectl wait --timeout=180s --for=condition=Running fabricpeers.hlf.kungfusoftware.es --all
```

Feito isso, prossiga com o tutorial até a seção de ingressar peers no canal e retorne.

## Preparar conexão para instalar o chaincode

Para preparar a conexão, precisamos

1. Criar o objeto `FabricNetworkConfig` no cluster
2. Obter a string de conexão do "Kubernetes Secrets"
3. Obter a cadeia de conexão sem os usuários das organizações

```bash

# This identity will register and enroll the user for inmetro
kubectl hlf identity create --name inmetro-admin --namespace default \
    --ca-name inmetro-ca --ca-namespace default \
    --ca ca --mspid INMETROMSP --enroll-id explorer-admin --enroll-secret explorer-adminpw \
    --ca-enroll-id=enroll --ca-enroll-secret=enrollpw --ca-type=admin


# networkconfig inmetro & orderer
kubectl hlf networkconfig create --name=inmetro-cp \
  -o INMETROMSP -o OrdererMSP -c demo \
  --identities=inmetro-admin.default --secret=inmetro-cp


```

### Obter a string de conexão do "Kubernetes Secret"

```bash
# fetch connection string from kubernetes secret
kubectl get secret inmetro-cp -o jsonpath="{.data.config\.yaml}" | base64 --decode > resources/inmetro.yaml

```

### Criação o arquivo de configuração de instalação de chaincode

```bash
# remove the code.tar.gz chaincode.tgz if they exist
rm code.tar.gz chaincode.tgz
export CHAINCODE_NAME=asset
export CHAINCODE_LABEL=asset
cat << METADATA-EOF > "metadata.json"
{
  "type": "ccaas",
  "label": "${CHAINCODE_LABEL}"
}
METADATA-EOF
## chaincode as a service
cat > "connection.json" <<CONN_EOF
{
  "address": "${CHAINCODE_NAME}:7052",
  "dial_timeout": "10s",
  "tls_required": false
}
CONN_EOF

tar cfz code.tar.gz connection.json
tar cfz chaincode.tgz metadata.json code.tar.gz
export PACKAGE_ID=$(kubectl hlf chaincode calculatepackageid --path=chaincode.tgz --language=node --label=$CHAINCODE_LABEL)
echo "PACKAGE_ID=$PACKAGE_ID"
```

### Instale o arquivo de configuração

```bash
kubectl hlf chaincode install --path=./chaincode.tgz \
    --config=resources/inmetro.yaml --language=golang --label=$CHAINCODE_LABEL --user=inmetro-admin-default --peer=inmetro-peer0.default

```
### Sincronize com o chaincode externo
```bash
# sync
kubectl hlf externalchaincode sync --image=victor07july/asset:latest \
    --name=$CHAINCODE_NAME \
    --namespace=default \
    --package-id=$PACKAGE_ID \
    --tls-required=false \
    --replicas=1
```


### Faça o approve e commit

```bash
  export SEQUENCE=1
  export VERSION="1.0"
  export ENDORSEMENT_POLICY="OR('INMETROMSP.member')"

  kubectl hlf chaincode approveformyorg --config=resources/inmetro.yaml  --user=inmetro-admin-default --peer=inmetro-peer0.default \
      --package-id=$PACKAGE_ID \
      --version "$VERSION" --sequence "$SEQUENCE" --name=$CHAINCODE_LABEL \
      --policy="${ENDORSEMENT_POLICY}" --channel=demo

echo "Aguarde 30 segundos para o chaincode ser aprovado"
sleep 30

export SEQUENCE=1
export VERSION="1.0"
export ENDORSEMENT_POLICY="OR('INMETROMSP.member')"

  kubectl hlf chaincode commit --config=resources/inmetro.yaml --user=inmetro-admin-default --mspid=INMETROMSP \
      --version "$VERSION" --sequence "$SEQUENCE" --name=$CHAINCODE_LABEL \
      --policy="${ENDORSEMENT_POLICY}" --channel=demo
```

Agora, o chaincode já pode ser utilizado através de [clientes](../client/) ou através da função invoke.

```bash
kubectl hlf chaincode invoke --config=resources/inmetro.yaml \
    --user=inmetro-admin-default --peer=inmetro-peer0.default \
    --chaincode=$CHAINCODE_LABEL --channel=demo \
    --fcn=initLedger
```
