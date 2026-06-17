# Hyperledger Fabric Operator

## Sumário

- [Funções do Operator](#funções-do-operator)
- [Recursos e Requisitos](#recursos-e-requisitos)
- [Instalação Rápida](#instalação-rápida)
- [Comandos da Função `inmetro`](#comandos-da-função-inmetro)
- [Tutorial de Deploy Manual](#tutorial-de-deploy-manual)
  - [1. Criar Cluster Kubernetes](#1-criar-cluster-kubernetes)
  - [2. Instalação do Istio](#2-instalação-do-istio)
  - [3. Instalar o HLF Operator](#3-instalar-o-hlf-operator)
  - [4. Deploy de Organizações](#4-deploy-de-organizações)
  - [5. Criação de Canal](#5-criação-de-canal)
- [Operator UI](#operator-ui)
- [Finalizando](#finalizando)
- [Derrubando o Ambiente](#derrubando-o-ambiente)



## Funções do Operator

- [x] Criação de certificates authorities (CA)
- [x] Criação de peers
- [x] Criação de ordering services
- [x] Criação de recursos sem modificação manual do material criptográfico
- [x] Roteamento de domínio com SNI usando Istio 
- [x] Execução de chaincode como chaincode externo via Kubernetes
- [x] Suporte a Hyperledger Fabric 2.3+
- [x] Gerenciamento de genesis para Ordering Services
- [x] E2E testing including the execution of chaincodes in KIND
- [x] Renovação de certificados

## Recursos e Requisitos

### Recursos:
- [Hyperledger Fabric build ARM](https://www.polarsparc.com/xhtml/Hyperledger-ARM-Build.html)
- [Repositório Oficial do Operator](https://github.com/hyperledger/bevel-operator-fabric)

### Requisitos:

- Linux (testado com Ubuntu 22.04)
- [Kubectl](https://kubernetes.io/pt-br/docs/tasks/tools/install-kubectl-linux/)
- [Krew](https://krew.sigs.k8s.io/)
- [KinD](https://kind.sigs.k8s.io/) ou [K3d](https://k3d.io/v5.6.0/)
- [Istio](https://istio.io/latest/ ) 
- [Helm](https://helm.sh/)
- [JQ](https://jqlang.github.io/jq/download/)
- [Docker](https://docs.docker.com/get-docker/)

## Instalação Rápida
Para instalar todos os requisitos automaticamente, basta executar o comando `./install.sh`. Ele fará a instalação de todos os requisitos e baixará imagens necessárias para a execução da rede.

>[!NOTE]
>Algumas vezes o docker pode retornar um erro de permissão no momento de download das imagens em dispositivos "zerados", como máquinas virtuais novas. Caso isso aconteça, apenas reinicie o dispositivo e execute o script de instalação novamente.
---

## Comandos da Função `inmetro`

O script principal para gerenciar a rede é o `inmetro`. Ele suporta os seguintes comandos:

| Comando        | Descrição                                                                                                           | Observações                                                                                   |
| -------------- | ------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------- |
| `up`           | Sobe toda a rede do zero, de modo offline.                                                                          | Use este comando para a primeira inicialização da rede.                                       |
| `lup`          | Sobe a rede reaproveitando componentes já existentes, sem reiniciar tudo.                                           | Não pode ser usado na primeira vez. Use após `down` para um restart mais rápido.              |
| `down`         | Derruba a rede, mas mantém alguns recursos persistentes.                                                            | Permite levantar rapidamente com `lup` depois.                                                |
| `shutdown`     | Derruba toda a rede e remove todas as dependências.                                                                 | Após esse comando, só é possível subir a rede novamente com `up`.                             |
| `lrestart`     | Reinicia a rede mantendo alguns componentes já existentes.                                                          | Útil para pequenas atualizações sem perda de dados.                                           |
| `restart`      | Reinicia a rede completamente, removendo todos os componentes temporários.                                          | Use para corrigir erros críticos ou ajustes de configuração.                                  |
| `ccas`         | Instala um chaincode usando o Chaincode as a Service. É necessário informar o nome de usuário do Docker como parâmetro. |
| `ccasupgrade`  | Atualiza um chaincode já instalado. É necessário fornecer a nova versão e a sequência como parâmetros.              |

**Como usar:**  
Execute no terminal:

```bash
inmetro <comando>
```
Exemplo:
```bash
inmetro up
```

> [!CAUTION]
> - O comando `shutdown` remove toda a rede e dependências, exigindo o uso de `up` para subir novamente.  
> - Para apenas desligar a rede mantendo instalações, use `down` seguido de `lup` para um novo start rápido.
> - Se a rede foi iniciada em um ambiente com acesso à internet e depois for reiniciada em outro ambiente, podem ocorrer falhas caso a configuração de rede mude.

---

## Tutorial de Deploy Manual
Atualmente, há duas versões sendo utilizadas

- 2.5+, que suporta Chaincode As a Service (CCAS)
- 2.4.6, que suporta chaincode local

Para testar a versão 2.5+, siga as instruções tutorial na pasta [ccas](ccas).

O tutorial a seguir utiliza a versão 2.4.6, que suporta chaincode local. Se a versão 2.5+ eventualmente obter suporte para chaincode local, o tutorial será atualizado.

Uma rede com a seguinte estrutura será instanciada neste tutorial:

- 1 Organização
- 1 Peer (2.4.6)
- 1 orderer

Além do chaincode e cliente.
Também é possível levantar esta rede predefinida com o comando ```network.sh up```.

É possível customizar comandos e modificar a rede conforme a sua necessidade.

## 1. Criar Cluster Kubernetes

#### Usando KinD

```bash
# Criar configuração
cat << EOF > resources/kind-config.yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  image: kindest/node:v1.25.8
  extraPortMappings:
  - containerPort: 30949
    hostPort: 80
  - containerPort: 30950
    hostPort: 443
EOF

# Criar cluster
kind create cluster --config=./resources/kind-config.yaml

# Definir variáveis
export STORAGE_CLASS=standard
export DATABASE=couchdb
```

---

### Instalação do Istio

```bash
# Criar namespace
kubectl create namespace istio-system

# Inicializar operador
istioctl operator init

# Aplicar configuração
kubectl apply -f - <<EOF
apiVersion: install.istio.io/v1alpha1
kind: IstioOperator
metadata:
  name: istio-gateway
  namespace: istio-system
spec:
  addonComponents:
    grafana:
      enabled: false
    kiali:
      enabled: false
    prometheus:
      enabled: false
    tracing:
      enabled: false
  components:
    ingressGateways:
      - enabled: true
        k8s:
          hpaSpec:
            minReplicas: 1
          resources:
            limits:
              cpu: 500m
              memory: 512Mi
            requests:
              cpu: 100m
              memory: 128Mi
          service:
            ports:
              - name: http
                port: 80
                targetPort: 8080
                nodePort: 30949
              - name: https
                port: 443
                targetPort: 8443
                nodePort: 30950
            type: NodePort
        name: istio-ingressgateway
    pilot:
      enabled: true
      k8s:
        hpaSpec:
          minReplicas: 1
        resources:
          limits:
            cpu: 300m
            memory: 512Mi
          requests:
            cpu: 100m
            memory: 128Mi
  meshConfig:
    accessLogFile: /dev/stdout
    enableTracing: false
    outboundTrafficPolicy:
      mode: ALLOW_ANY
  profile: default
EOF
```
---
### Configurar DNS Interno

```bash
kubectl apply -f - <<EOF
kind: ConfigMap
apiVersion: v1
metadata:
  name: coredns
  namespace: kube-system
data:
  Corefile: |
    .:53 {
        errors
        health {
           lameduck 5s
        }
        rewrite name regex (.*)\.localho\.st istio-ingressgateway.istio-system.svc.cluster.local
        hosts {
            ${CLUSTER_IP} istio-ingressgateway.istio-system.svc.cluster.local
            fallthrough
        }
        ready
        kubernetes cluster.local in-addr.arpa ip6.arpa {
           pods insecure
           fallthrough in-addr.arpa ip6.arpa
           ttl 30
        }
        prometheus :9153
        forward . /etc/resolv.conf {
           max_concurrent 1000
        }
        cache 30
        loop
        reload
        loadbalance
    }
EOF
```

---

### Instalar o HLF Operator

```bash
# instalar localmente 
helm install hlf-operator resources/hlf-operator-1.9.2.tgz
```

---
### Instalar o plugin Kubectl

Em caso de erro, verifique se o [Krew](https://krew.sigs.k8s.io/docs/user-guide/setup/install/) está intsalado.

A seguir, instale o Kubectl com o seguinte comando:

```bash
# Instalar plugin HLF
kubectl krew install hlf
```

> Certifique-se de que o [Krew](https://krew.sigs.k8s.io/docs/user-guide/setup/install/) está instalado

---

### Deploy de Organizações

#### Definir Variáveis de Ambiente

```bash
export ORDERER_IMAGE=hyperledger/fabric-orderer
export ORDERER_VERSION=2.5.5
export CA_IMAGE=hyperledger/fabric-ca
export CA_VERSION=1.5.7
```
---
### Criação do CA para INMETRO

```bash
# Criar CA
kubectl hlf ca create --image=$CA_IMAGE --version=$CA_VERSION --storage-class=$STORAGE_CLASS --capacity=1Gi --name=inmetro-ca \
    --enroll-id=enroll --enroll-pw=enrollpw --hosts=inmetro-ca.localho.st --istio-port=443

# Aguardar CA ficar pronto
kubectl wait --timeout=180s --for=condition=Running fabriccas.hlf.kungfusoftware.es --all

# Verificar CA
curl -k https://inmetro-ca.localho.st:443/cainfo
```

#### Registrar Usuário Peer

```bash
kubectl hlf ca register --name=inmetro-ca --user=peer --secret=peerpw --type=peer \
 --enroll-id enroll --enroll-secret=enrollpw --mspid INMETROMSP
```
---
### Deploy de um peer para a organização INMETRO

> [!WARNING] Este peer não suporta [CCAS](ccas)

```bash
export PEER_IMAGE=quay.io/kfsoftware/fabric-peer
export PEER_VERSION=2.4.1-v0.0.3
export MSP_ORG=INMETROMSP
export PEER_SECRET=peerpw

kubectl hlf peer create --statedb=$DATABASE --image=$PEER_IMAGE --version=$PEER_VERSION --storage-class=$STORAGE_CLASS --enroll-id=peer --mspid=$MSP_ORG \
--enroll-pw=$PEER_SECRET --capacity=5Gi --name=inmetro-peer0 --ca-name=inmetro-ca.default --k8s-builder=true --hosts=peer0-inmetro.localho.st --istio-port=443

kubectl wait --timeout=180s --for=condition=Running fabricpeers.hlf.kungfusoftware.es --all

# Verificar peer
openssl s_client -connect peer0-inmetro.localho.st:443

```
---
### Deploy de uma organização `Orderer`

para fazer o deploy de uma organização orderer, temos que:

1. Criar um certification authority (CA)
2. Registrar usuário `orderer` com senha `ordererpw`
3. Criar orderer
---
### Criar o CA

```bash
kubectl hlf ca create --image=$CA_IMAGE --version=$CA_VERSION --storage-class=$STORAGE_CLASS --capacity=1Gi --name=ord-ca \
    --enroll-id=enroll --enroll-pw=enrollpw --hosts=ord-ca.localho.st --istio-port=443

kubectl wait --timeout=180s --for=condition=Running fabriccas.hlf.kungfusoftware.es --all

# Verificar
curl -vik https://ord-ca.localho.st:443/cainfo
```
---
### Registre o usuário `orderer`

```bash
kubectl hlf ca register --name=ord-ca --user=orderer --secret=ordererpw \
    --type=orderer --enroll-id enroll --enroll-secret=enrollpw --mspid=OrdererMSP --ca-url="https://ord-ca.localho.st:443"
```
---
### Deploy de um orderer

**3. Deploy do orderer:**
```bash
kubectl hlf ordnode create --image=$ORDERER_IMAGE --version=$ORDERER_VERSION \
      --storage-class=$STORAGE_CLASS --enroll-id=orderer --mspid=OrdererMSP \
      --enroll-pw=ordererpw --capacity=2Gi --name=ord-node0 --ca-name=ord-ca.default \
      --hosts=orderer0-ord.localho.st --istio-port=443 --admin-hosts=admin-orderer0-ord.localho.st

kubectl wait --timeout=180s --for=condition=Running fabricorderernodes.hlf.kungfusoftware.es --all

# Verificar
kubectl get pods
openssl s_client -connect orderer0-ord.localho.st:443
```

---

## 5. Criação de Canal

#### Registrar Identidades

**OrdererMSP:**
```bash
kubectl hlf ca register --name=ord-ca --user=admin --secret=adminpw \
    --type=admin --enroll-id enroll --enroll-secret=enrollpw --mspid=OrdererMSP

kubectl hlf identity create --name orderer-admin-sign --namespace default \
    --ca-name ord-ca --ca-namespace default \
    --ca ca --mspid OrdererMSP --enroll-id admin --enroll-secret adminpw

kubectl hlf identity create --name orderer-admin-tls --namespace default \
    --ca-name ord-ca --ca-namespace default \
    --ca tlsca --mspid OrdererMSP --enroll-id admin --enroll-secret adminpw
```

---
### Registrar e matricular identidade INMETROMSP

```bash
kubectl hlf ca register --name=inmetro-ca --namespace=default --user=admin --secret=adminpw \
    --type=admin --enroll-id enroll --enroll-secret=enrollpw --mspid=INMETROMSP

kubectl hlf identity create --name inmetro-admin --namespace default \
    --ca-name inmetro-ca --ca-namespace default \
    --ca ca --mspid INMETROMSP --enroll-id admin --enroll-secret adminpw
```
---
### Criando canal principal

```bash
export PEER_ORG_SIGN_CERT=$(kubectl get fabriccas inmetro-ca -o=jsonpath='{.status.ca_cert}')
export PEER_ORG_TLS_CERT=$(kubectl get fabriccas inmetro-ca -o=jsonpath='{.status.tlsca_cert}')
export IDENT_8=$(printf "%8s" "")
export ORDERER_TLS_CERT=$(kubectl get fabriccas ord-ca -o=jsonpath='{.status.tlsca_cert}' | sed -e "s/^/${IDENT_8}/" )
export ORDERER0_TLS_CERT=$(kubectl get fabricorderernodes ord-node0 -o=jsonpath='{.status.tlsCert}' | sed -e "s/^/${IDENT_8}/" )

kubectl apply -f - <<EOF
apiVersion: hlf.kungfusoftware.es/v1alpha1
kind: FabricMainChannel
metadata:
  name: demo
spec:
  name: demo
  adminOrdererOrganizations:
    - mspID: OrdererMSP
  adminPeerOrganizations:
    - mspID: INMETROMSP
  channelConfig:
    application:
      acls: null
      capabilities:
        - V2_0
      policies: null
    capabilities:
      - V2_0
    orderer:
      batchSize:
        absoluteMaxBytes: 1048576
        maxMessageCount: 10
        preferredMaxBytes: 524288
      batchTimeout: 2s
      capabilities:
        - V2_0
      etcdRaft:
        options:
          electionTick: 10
          heartbeatTick: 1
          maxInflightBlocks: 5
          snapshotIntervalSize: 16777216
          tickInterval: 500ms
      ordererType: etcdraft
      policies: null
      state: STATE_NORMAL
    policies: null
  externalOrdererOrganizations: []
  peerOrganizations:
    - mspID: INMETROMSP
      caName: "inmetro-ca"
      caNamespace: "default"
  identities:
    OrdererMSP:
      secretKey: user.yaml
      secretName: orderer-admin-tls
      secretNamespace: default
    OrdererMSP-sign:
      secretKey: user.yaml
      secretName: orderer-admin-sign
      secretNamespace: default
    INMETROMSP:
      secretKey: user.yaml
      secretName: inmetro-admin
      secretNamespace: default
  externalPeerOrganizations: []
  ordererOrganizations:
    - caName: "ord-ca"
      caNamespace: "default"
      externalOrderersToJoin:
        - host: ord-node0
          port: 7053
      mspID: OrdererMSP
      ordererEndpoints:
        - orderer0-ord.localho.st:443
      orderersToJoin: []
  orderers:
    - host: orderer0-ord.localho.st
      port: 443
      tlsCert: |-
${ORDERER0_TLS_CERT}
EOF
```
---
### Inserir peers do INMETRO no canal

```bash
export IDENT_8=$(printf "%8s" "")
export ORDERER0_TLS_CERT=$(kubectl get fabricorderernodes ord-node0 -o=jsonpath='{.status.tlsCert}' | sed -e "s/^/${IDENT_8}/" )

kubectl apply -f - <<EOF
apiVersion: hlf.kungfusoftware.es/v1alpha1
kind: FabricFollowerChannel
metadata:
  name: demo-inmetromsp
spec:
  anchorPeers:
    - host: inmetro-peer0.default
      port: 7051
  hlfIdentity:
    secretKey: user.yaml
    secretName: inmetro-admin
    secretNamespace: default
  mspId: INMETROMSP
  name: demo
  externalPeersToJoin: []
  orderers:
    - certificate: |
${ORDERER0_TLS_CERT}
      url: grpcs://ord-node0.default:7050
  peersToJoin:
    - name: inmetro-peer0
      namespace: default
EOF
```

## Operator UI

O HLF Operator UI fornece uma interface gráfica para uma experiência de usuário mais conveniente. O Operator UI torna mais fácil o processo de criar, clonar, supervisionar, editar e deletar os nós de Peers, Orderers e CAs.

Ele consiste de dois componentes

### Operator API
Fornece acesso aos dados para serem exibidos pelo Operator UI

- Canais
- Peers
- Orderer Nodes
- Certificate Authorities

### Operator UI
Interface gráfica que permite:

- Criar peers
- Criar CAs
- Criar orderers
- Renovar certificados

### Levantando o Operator UI

Primeiro deve-se levantar o Operator API
```bash
export API_URL=api-operator.localho.st # URL de acesso

kubectl hlf operatorapi create --name=operator-api --namespace=default --hosts=$API_URL --ingress-class-name=istio
```

Agora, para levantar o Operator UI

```bash
export HOST=operator-ui.localho.st
export API_URL="http://api-operator.localho.st/graphql"

kubectl hlf operatorui create --name=operator-ui --namespace=default --hosts=$HOST --ingress-class-name=istio --api-url=$API_URL
```

Verifique se eles estão funcionando com o comando a seguir

```
kubectl get pods
```

Seus containeres devem estar com o estado "Running"

### Acessando o Operator UI

No navegador, insira a URL:

operator-ui.localho.st

### Finalizando
A essa altura, você deve ter:

- Um serviço de ordenação com 3 orderers e CA
- Organização INMETRO com 1 peer e CA
- Um canal chamado "demo"
- Um chaincode instalado nos peers do INMETRO, aprovado e "commitado"

### Derrubando o ambiente

```bash
kubectl delete fabricorderernodes.hlf.kungfusoftware.es --all-namespaces --all
kubectl delete fabricpeers.hlf.kungfusoftware.es --all-namespaces --all
kubectl delete fabriccas.hlf.kungfusoftware.es --all-namespaces --all
kubectl delete fabricchaincode.hlf.kungfusoftware.es --all-namespaces --all
kubectl delete fabricmainchannels --all-namespaces --all
kubectl delete fabricfollowerchannels --all-namespaces --all
kubectl delete fabricnetworkconfigs --all-namespaces --all
kubectl delete fabricidentities --all-namespaces --all

# Remover cluster
kind delete cluster
```

### Gerenciamento de Canais
```bash
# Apagar canais
kubectl delete fabricmainchannel <channel>
kubectl delete fabricfollowerchannel <channel>
```

---

## Resultado Final

Após completar este tutorial, você terá:

- **1 Serviço de ordenação** com orderer e CA
- **Organização INMETRO** com 1 peer e CA  
- **1 Canal** chamado "demo"
- **Interface de administração** (Operator UI)
- **Rede pronta** para instalação de chaincodes

---

## Recursos Adicionais

- **Tutorial CCAS:** [ccas/README.md](ccas/README.md)
- **Documentação Oficial:** [Hyperledger Fabric](https://hyperledger-fabric.readthedocs.io/)
- **Repositório do Operator:** [bevel-operator-fabric](https://github.com/hyperledger/bevel-operator-fabric)
- **Clients de Exemplo:** [client/](client/)

---

> [!TIP] Para desenvolvimento rápido, use o script `inmetro up` para deploy automatizado da rede completa.