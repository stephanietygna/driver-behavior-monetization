// Importa bibliotecas do Hyperledger Fabric, Node.js e terceiros
const { Wallets } = require('fabric-network');  // Para acessar identidades na wallet
const crypto = require('crypto');               // Para geração e verificação criptográfica
const fs = require('fs');
const path = require('path');
const jwt = require('jsonwebtoken');            // Para geração de tokens JWT
const forge = require('node-forge');            // Para manipulação de certificados X.509

// 1. Gerenciamento de desafios: mapa em memória para armazenar nonces temporários
const activeChallenges = new Map();

/**
 * Gera um novo desafio de autenticação para o usuário.
 * Um desafio é um nonce aleatório válido por 5 minutos.
 */
function generateChallenge() {
    const nonce = crypto.randomBytes(32).toString('hex'); // Gera 32 bytes aleatórios em hex
    const expiresAt = Date.now() + 300000; // Validade: 5 minutos (300.000 ms)
    return { nonce, expiresAt };
}

/**
 * Verifica uma assinatura DER recebida, comparando com o desafio salvo.
 * 
 * Fluxo:
 * - Verifica parâmetros de entrada.
 * - Recupera o desafio salvo para o usuário.
 * - Busca o certificado do usuário na wallet.
 * - Extrai a chave pública do certificado.
 * - Verifica se a assinatura DER é válida para o nonce.
 * - Se for válida, emite um JWT.
 * - Remove o desafio (uso único).
 * 
 * @param {string} username - Nome de usuário
 * @param {Buffer|Uint8Array} signatureDER - Assinatura recebida em DER
 * @returns {Object} - Um objeto com o JWT emitido
 */
async function verifySignature(username, signatureDER) {
    try {
        // Validação básica dos parâmetros de entrada
        if (!username || !signatureDER) {
            throw new Error('Parâmetros inválidos');
        }

        // Recupera o desafio ativo do mapa
        const challenge = activeChallenges.get(username);
        if (!challenge || Date.now() > challenge.expiresAt) {
            throw new Error('Desafio expirado ou inexistente');
        }

        // Monta caminho da wallet local (padrão INMETROMSP)
        const walletPath = path.join(process.cwd(), 'wallet', 'INMETROMSP');
        const wallet = await Wallets.newFileSystemWallet(walletPath);
        const identity = await wallet.get(username);

        if (!identity) {
            throw new Error('Identidade não encontrada');
        }

        // Converte certificado PEM para objeto de chave pública usando forge
        const cert = forge.pki.certificateFromPem(identity.credentials.certificate);
        const publicKey = forge.pki.publicKeyToPem(cert.publicKey);

        // Cria um verificador com SHA256 e alimenta com o nonce
        const verifier = crypto.createVerify('SHA256');
        verifier.update(challenge.nonce);

        // Verifica a assinatura recebida (em formato DER)
        const isValid = verifier.verify(
            { key: publicKey, format: 'pem', type: 'spki' },
            signatureDER,
            'der'
        );

        // Remove o desafio para evitar reutilização
        activeChallenges.delete(username);

        if (!isValid) {
            throw new Error('Assinatura inválida');
        }

        // Gera e retorna um token JWT válido por 1 hora
        const token = jwt.sign(
            { sub: username },
            process.env.JWT_SECRET,
            { expiresIn: '1h', algorithm: 'HS256' }
        );

        return { token };
    } catch (error) {
        console.error('Falha na verificação:', error);
        throw error;
    }
}

// Exporta as funções para uso em outras partes da aplicação
module.exports = {
    generateChallenge, // Gera um novo nonce + timestamp de expiração
    verifySignature,   // Verifica assinatura DER e emite JWT
};