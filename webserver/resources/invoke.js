// Importa bibliotecas necessárias
const grpc = require('@grpc/grpc-js'); // Cliente gRPC para comunicação com peers
const { connect, hash } = require('@hyperledger/fabric-gateway'); // Gateway Fabric para operações de contrato
const fs = require('fs'); // Para ler arquivos do sistema
const path = require('path'); // Para manipular caminhos de arquivos
const yaml = require("js-yaml"); // Para ler arquivos YAML

// Variáveis globais para manter objetos da sessão atual
let CC_NAME; // Nome do chaincode ativo
const CHANNEL = "demo"; // Nome do canal Hyperledger Fabric
const MSPID = "INMETROMSP"; // ID do MSP da organização
let client, gateway, network, contract;
let proposalBytes, transactionBytes, signedTransaction, commitBytes;

// Encoder para converter strings para bytes UTF-8 (obrigatório no SDK)
const utf8Encoder = new TextEncoder();

/**
 * Inicializa a comunicação com o peer e o gateway usando a identidade de um usuário.
 * 
 * @param {string} username - Nome do usuário que terá sua identidade carregada da wallet.
 * @param {string} chaincode - Nome do chaincode (contrato inteligente) a ser utilizado.
 */

function initialize(username, chaincode) {
  console.log("Inicializando conexão gateway com chaincode")
  CC_NAME = chaincode;

  // Monta o caminho para o arquivo de identidade na wallet
  const identityPath = path.resolve(process.cwd(), 'wallet/INMETROMSP/', `${username}.id`);
  const identityData = JSON.parse(fs.readFileSync(identityPath, 'utf8'));

  // Extrai certificado do arquivo de identidade
  const certificate = utf8Encoder.encode(identityData.credentials.certificate);

  // Lê o arquivo de configuração de conexão YAML para pegar o certificado do peer
  const configContent = fs.readFileSync('connection-org.yaml', 'utf-8');
  const data = yaml.load(configContent);
  const cert = data.peers['inmetro-peer0.default'].tlsCACerts.pem;

  // Converte o certificado TLS do peer para buffer
  const tlsRootCert = Buffer.from(cert);

  // Cria o cliente gRPC com canal seguro TLS para o peer
  client = new grpc.Client('peer0.inmetro.br:443', grpc.credentials.createSsl(tlsRootCert));

  // Conecta o gateway usando a identidade do usuário e o cliente gRPC
  gateway = connect({
    identity: { mspId: MSPID, credentials: certificate },
    hash: hash.none, // Desativa hashing interno (pode mudar dependendo do seu uso)
    client
  });

  // Obtém o objeto de rede (canal)
  network = gateway.getNetwork(CHANNEL);

  // Obtém o contrato (chaincode)
  contract = network.getContract(CC_NAME);
  console.log("Conexão finalizada com sucesso")
}

/**
 * Cria uma proposta de transação para invocar uma função do chaincode.
 * 
 * @param {string} fcn - Nome da função do chaincode a ser chamada.
 * @param {...string} args - Argumentos para a função do chaincode (quantos forem necessários).
 * 
 * @returns {Buffer} - Digest (hash) da proposta gerada, para ser assinado externamente.
 */
async function createProposal(fcn, ...args) {
  // Cria uma proposta não assinada
  const unsignedProposal = contract.newProposal(fcn, {
    arguments: args,
  });

  // Obtém os bytes da proposta (para posterior assinatura)
  proposalBytes = unsignedProposal.getBytes();

  // Digest = hash da proposta
  const proposalDigest = Buffer.from(unsignedProposal.getDigest());

  console.log("Proposta criada. Digest:\n", proposalDigest);
  console.log("=================================================================================");

  return proposalDigest;
}

/**
 * Cria uma transação a partir da proposta, usando a assinatura da proposta.
 * 
 * @param {Buffer} proposalSig - Assinatura da proposta, gerada fora do script.
 * 
 * @returns {Buffer} - Digest (hash) da transação, para ser assinado externamente.
 */
async function createTransaction(proposalSig) {
  // Cria a proposta assinada usando o digest assinado
  const signedProposal = gateway.newSignedProposal(
    proposalBytes, proposalSig
  );

  // Endossa a proposta, gerando uma transação não assinada
  const unsignedTransaction = await signedProposal.endorse();

  // Guarda bytes da transação para próxima etapa
  transactionBytes = unsignedTransaction.getBytes();

  // Digest = hash da transação
  const transactionDigest = Buffer.from(unsignedTransaction.getDigest());

  console.log("Transação criada. Digest:\n", transactionDigest);
  console.log("=================================================================================");

  return transactionDigest;
}

/**
 * Cria o commit da transação, usando a assinatura da transação.
 * 
 * @param {Buffer} transactionSig - Assinatura da transação, gerada fora do script.
 * 
 * @returns {Buffer} - Digest (hash) do commit, para ser assinado externamente.
 */
async function createCommit(transactionSig) {
  // Cria a transação assinada com o digest assinado
  signedTransaction = gateway.newSignedTransaction(
    transactionBytes, transactionSig
  );

  // Submete a transação para ser comprometida na ledger (gera um commit não assinado)
  const unsignedCommit = await signedTransaction.submit();

  // Guarda bytes do commit para etapa final
  commitBytes = unsignedCommit.getBytes();

  // Digest = hash do commit
  const commitDigest = Buffer.from(unsignedCommit.getDigest());

  console.log("Commit criado. Digest:\n", commitDigest);
  console.log("=================================================================================");

  return commitDigest;
}

/**
 * Finaliza o fluxo assinando o commit e obtém o status de confirmação na blockchain.
 * 
 * @param {Buffer} commitSig - Assinatura do commit, gerada fora do script.
 * 
 * @returns {Object} - Resultado retornado pelo chaincode, se houver.
 */
async function finalize(commitSig) {
  // Cria commit assinado usando a assinatura externa
  const signedCommit = gateway.newSignedCommit(commitBytes, commitSig);

  // Obtém status do commit na blockchain
  const status = await signedCommit.getStatus();

  // Obtém o resultado retornado pelo chaincode (se houver)
  const result = signedTransaction.getResult();

  console.log('Transação finalizada. Status:\n', status);
  console.log("=================================================================================");

  let resultJson;

  if (result && result.length > 0) {
    const jsonStr = Buffer.from(result).toString('utf-8');

    try {
      // Tenta converter para JSON se for válido
      resultJson = JSON.parse(jsonStr);
      return resultJson;
    } catch (error) {
      console.error("Erro ao converter para JSON:", error);
    }
  }
}

/**
 * Encerra a conexão com o gateway e o cliente gRPC.
 */
async function close() {
  console.log('======== Fechando conexão gateway ========');
  gateway.close();
  client.close();
}

// Exporta funções para uso externo
module.exports = {
  initialize,
  createProposal,
  createTransaction,
  createCommit,
  finalize,
  close
};
