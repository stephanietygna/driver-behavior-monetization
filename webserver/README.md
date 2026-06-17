#  Webserver - Interface Web para Hyperledger Fabric

> Interface web moderna para interação com redes Hyperledger Fabric

##  Sumário

- [Visão Geral](#-visão-geral)
- [Funcionalidades](#-funcionalidades)
- [Requisitos](#-requisitos)
- [Instalação e Configuração](#-instalação-e-configuração)
- [Interface do Usuário](#-interface-do-usuário)
- [API Endpoints](#-api-endpoints)
- [️Arquitetura](#-arquitetura)
- [Como Usar](#-como-usar)
- [Tutoriais em vídeo](#-tutoriais-em-video)
- [Autenticação e Segurança](#-autenticação-e-segurança)
- [Solução de Problemas](#-solução-de-problemas)

---

##  Visão Geral

Este webserver fornece uma interface web intuitiva para interagir com redes Hyperledger Fabric. Desenvolvido em **Node.js** com **Express**, oferece funcionalidades para registro de usuários, execução de chaincodes e operações de blockchain.

### Principais Características

- **Gerenciamento de usuários** e autenticação
- **Execução de chaincodes** em tempo real
- **Sistema de carteira** para transações
- **Suporte a múltiplos chaincodes** (Vehicle e BrakeTester, mas é capaz de operar qualquer chaincode)
- **Integração completa** com Hyperledger Fabric SDK

---

## Funcionalidades

### Gerenciamento de Usuários
- [x] **Registro** de novos usuários com certificados CSR
- [x] **Login** seguro com validação de identidade
- [x] **Autenticação** baseada em certificados Fabric
- [x] **Sessões** persistentes

### Interação com Blockchain
- [x] **Execução de transações** em chaincodes
- [x] **Consultas** em tempo real ao ledger
- [x] **Suporte a múltiplos chaincodes**
- [x] **Histórico de transações**

---

## Requisitos

### Sistema
- **Node.js** 18.19.0+ 
- **npm**
- **Rede Hyperledger Fabric** ativa

### Dependências Principais
```json
{
  "@grpc/grpc-js": "^1.13.3",
   "@hyperledger/fabric-gateway": "^1.7.1",
   "@noble/curves": "^1.9.1",
   "crypto": "^1.0.1",
   "jsonwebtoken": "^9.0.2",
   "jsrsasign": "^11.1.0",
   "multer": "^1.4.5-lts.1",
   "path": "^0.12.7",
   "yaml": "^2.7.0"   
}
```

---

## Instalação e Configuração

### 1. Instalar Dependências

```bash
cd webserver
npm install
```

### 2. Configurar Conexão com a Rede

O `connection-org.yaml` atualiza dinâmicamente conforme o levantamento do servidor. Ele se parece com o seguinte:

```yaml

name: hlf-network
version: 1.0.0
client:
  organization: "INMETROMSP"
organizations:
  
  INMETROMSP:
    mspid: INMETROMSP
    cryptoPath: /tmp/cryptopath
    certificateAuthorities:
    - inmetro-ca.default
    users:
      inmetro-admin-default:
        cert:
          pem: |
        -----BEGIN CERTIFICATE-----
        <certificado>
        -----END CERTIFICATE-----
            
        key:
          pem: |
        -----BEGIN CERTIFICATE-----
        <certificado>
        -----END CERTIFICATE-----
            
    peers:
      - inmetro-peer0.default
    orderers: []
  OrdererMSP:
    mspid: OrdererMSP
    cryptoPath: /tmp/cryptopath
    users: {}
    peers: []
    orderers:
      - ord-node0.default
      - ord-node1.default
      - ord-node2.default

orderers:
  ord-node0.default:

    url: grpcs://orderer0-ord.inmetro.br:443


    adminUrl: https://admin-orderer0-ord.inmetro.br:443
    adminTlsCert: |
        -----BEGIN CERTIFICATE-----
        <certificado>
        -----END CERTIFICATE-----

    grpcOptions:
      allow-insecure: false
    tlsCACerts:
      pem: |
        -----BEGIN CERTIFICATE-----
        <certificado>
        -----END CERTIFICATE-----

  ord-node1.default:

    url: grpcs://orderer1-ord.inmetro.br:443


    adminUrl: https://admin-orderer1-ord.inmetro.br:443
    adminTlsCert: |
        -----BEGIN CERTIFICATE-----
        <certificado>
        -----END CERTIFICATE-----
        
    grpcOptions:
      allow-insecure: false
    tlsCACerts:
      pem: |
        -----BEGIN CERTIFICATE-----
        <certificado>
        -----END CERTIFICATE-----
        
  ord-node2.default:

    url: grpcs://orderer2-ord.inmetro.br:443


    adminUrl: https://admin-orderer2-ord.inmetro.br:443
    adminTlsCert: |
        -----BEGIN CERTIFICATE-----
        <certificado>
        -----END CERTIFICATE-----

    grpcOptions:
      allow-insecure: false
    tlsCACerts:
      pem: |
        -----BEGIN CERTIFICATE-----
        <certificado>
        -----END CERTIFICATE-----

peers:
  inmetro-peer0.default:

    url: grpcs://peer0.inmetro.br:443

    grpcOptions:
      allow-insecure: false
    tlsCACerts:
      pem: |
        -----BEGIN CERTIFICATE-----
        <certificado>
        -----END CERTIFICATE-----
        
certificateAuthorities:
  
  inmetro-ca.default:

    url: https://inmetro-ca.inmetro.br:443


    registrar:
        enrollId: enroll
        enrollSecret: "enrollpw"

    caName: ca
    tlsCACerts:
      pem: 
       - |
        -----BEGIN CERTIFICATE-----
        <certificado>
        -----END CERTIFICATE-----
            
  
  ord-ca.default:

    url: https://ord-ca.inmetro.br:443


    registrar:
        enrollId: enroll
        enrollSecret: "enrollpw"

    caName: ca
    tlsCACerts:
      pem: 
       - |
        -----BEGIN CERTIFICATE-----
        <certificado>
        -----END CERTIFICATE-----
            


channels:
  demo:

    orderers:
      - ord-node0.default
      - ord-node1.default
      - ord-node2.default

    peers:
       inmetro-peer0.default:
        discover: true
        endorsingPeer: true
        chaincodeQuery: true
        ledgerQuery: true
        eventSource: true
```

### 3. Verificar Rede Fabric

Antes de iniciar o webserver, certifique-se de que a rede Fabric está executando:

```bash
# Verificar pods da rede
kubectl get pods

# Verificar conectividade
curl -k https://peer0-inmetro.localho.st:443
```

### 4. Iniciar o Servidor

```bash
node server.js
```

O servidor estará disponível em: `http://localhost:3000`

---

## Interface do Usuário

### Página Inicial (`/`)
- **Navegação principal** para todas as seções
- **Botões de login/registro** no canto superior direito
- **Links rápidos** para Chaincodes

###  Sistema de Autenticação

#### Registro (`/register`)
- **Upload de chave** para criação de CSR
- **Nome de usuário** único
- **Validação** automática

#### Login (`/verify-login`)
- **Autenticação** por desafio-resposta
- **Verificação** de identidade na rede
- **Redirecionamento** automático após login

### Execução de Chaincodes (`/invoke`)
- **Seleção de chaincode** (Vehicle/BrakeTester)
- **Campos dinâmicos** baseados no chaincode selecionado
- **Execução** de funções específicas

---

## API Endpoints

### Autenticação

| Método | Endpoint | Descrição |
|--------|----------|-----------|
| `POST` | `/register` | Registrar novo usuário com um CSR e username únicos. |
| `GET` | `/verify-login` | Verificar se a tentativa de login é válida (usuário existe e a chave está correta). Caso seja, o usuário é autenticado com sucesso. |
| `POST` | `/get-nonce` | Gera um `nonce` para autenticação do usuário por meio de desafio-resposta. |

### Chaincode

| Método | Endpoint | Descrição |
|--------|----------|-----------|
| `POST` | `/invoke` | Executar função de chaincode |
| `GET` | `/query` | Obter histórico de execuções |

---

## Como Usar

### 1. Primeiro Acesso

1. **Acesse** `http://localhost:3000`
2. **Clique** em "Registrar" (canto superior direito)
3. **Faça upload** da chave do usuário
4. **Digite** um nome de usuário único
5. **Aguarde** a confirmação do registro

### 2. Usando Chaincodes

1. **Faça login** no sistema
2. **Navegue** para "Chaincodes"
3. **Selecione** o chaincode desejado (Vehicle/BrakeTester)
4. **Preencha** os campos necessários
5. **Execute** a função
6. **Visualize** o resultado

---

## Autenticação e Segurança

### Sistema de Certificados

- **Certificados X.509** para autenticação
- **Validação** via Fabric CA
- **Carteiras** protegidas por usuário

### Validações

- **CSR obrigatório** no registro
- **Verificação** de identidade única
- **Validação** de entrada em todas as operações

### Sessões

- **Gerenciamento** de sessões ativas
- **Timeout** automático de 1h
- **Verificação** contínua de autenticação

---

## Tutoriais em vídeo

Para facilitar o desenvolvimento futuro, disponibilizei 3 vídeos explicando um pouco sobre a estrutura da rede. Eles contém o seguinte:

- [Explicação geral da rede](https://youtu.be/m-3-5GaWaSU)
- [Implementação de chaincode na interface](https://youtu.be/o4hTe22b6nY)
- [Explicação do sistema de assinatura canônica](https://youtu.be/lZTsi2ZnteQ)

---

## Solução de Problemas

### Problemas Comuns

#### Erro de Conexão com a Rede

```bash
# Verificar se a rede está ativa
kubectl get pods
```

#### Falha no Registro de Usuário

- **Verificar** se a chave é válida (chave deve ser padrão P256v1, em formato PKCS#8)
- **Confirmar** se o CA está funcionando
- **Verificar** logs do servidor no terminal do navegador ou os logs do peer

#### Erro na Execução de Chaincode

- **Verificar** se o chaincode está instalado
- **Confirmar** se o usuário foi corretamente registrado

### Logs e Debug

```bash
# Iniciar com logs detalhados
DEBUG=* node server.js

# Verificar logs específicos do Fabric
export HFC_LOGGING='{"debug":"console"}'
node server.js
```

### Limpeza de Cache

```bash
# Limpar carteiras (usar com cuidado!)
rm -rf wallet/*

# Reiniciar servidor
pkill node
node server.js
```

---

## ️Personalização

### Modificando a Interface

- **Estilos CSS** estão inline nos arquivos HTML
- **JavaScript** para interatividade está no final de cada página
- **Responsividade** já implementada

---

> ** Dica:** Mantenha sempre sua rede Fabric ativa antes de usar o webserver. Use `kubectl get pods` para verificar o status dos componentes.