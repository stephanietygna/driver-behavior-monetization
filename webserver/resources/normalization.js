// Importa a curva P-256 do pacote @noble/curves
const { p256 } = require('@noble/curves/p256');

/**
 * Recebe uma assinatura ECDSA no formato RAW (64 bytes R||S),
 * normaliza o componente S para a forma canônica,
 * e retorna a assinatura em formato DER.
 * 
 * @param {Uint8Array|Buffer} signatureRawArray - Assinatura bruta: [R(32 bytes), S(32 bytes)]
 * @returns {Uint8Array} - Assinatura codificada em DER, com S normalizado.
 */
function normalizeS(signatureRawArray) {
    try {
        // Converte a entrada para Buffer (caso venha como Uint8Array)
        const rawSignature = Buffer.from(signatureRawArray);

        // Verifica se a assinatura está no formato RAW: 64 bytes (R + S)
        if (rawSignature.length !== 64) {
            throw new Error(`Assinatura deve ter 64 bytes (RAW). Recebido: ${rawSignature.length} bytes`);
        }

        // Separa os 32 primeiros bytes como R e os 32 últimos como S
        const r = rawSignature.subarray(0, 32);
        const s = rawSignature.subarray(32, 64);

        // Converte R e S para BigInt para validar e processar
        const rBigInt = BigInt('0x' + r.toString('hex'));
        const sBigInt = BigInt('0x' + s.toString('hex'));

        // Define a ordem da curva P-256 (valor fixo)
        const curveOrder = BigInt('0xFFFFFFFF00000000FFFFFFFFFFFFFFFFBCE6FAADA7179E84F3B9CAC2FC632551');

        // Valida se R e S estão dentro do intervalo permitido pela curva
        if (rBigInt >= curveOrder || sBigInt >= curveOrder) {
            throw new Error("Componentes R ou S fora do intervalo da curva P-256");
        }

        // Cria objeto assinatura usando a classe da curva
        const signature = new p256.Signature(rBigInt, sBigInt);

        // **NORMALIZA o componente S aqui!**
        // Isso garante que S fique no menor valor válido (canonical form)
        const canonicalSig = signature.normalizeS();

        // Converte para DER (ASN.1) e obtém os bytes
        const finalDer = canonicalSig.toDERRawBytes();

        // Retorna a assinatura DER com S normalizado
        return finalDer;

    } catch (error) {
        console.error("Erro detalhado:", error);
        throw new Error(`Falha na normalização: ${error.message}`);
    }
}

// Exporta a função para uso externo
module.exports = normalizeS;
