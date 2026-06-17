export async function signDigest(digestUint8Array, privateKeyPem) {
    // 1. Converte a chave privada PEM para ArrayBuffer (binário)
    function pemToArrayBuffer(pem) {
        // Remove cabeçalhos, quebras de linha e espaços
        const b64 = pem
            .replace(/-----BEGIN PRIVATE KEY-----/, '')
            .replace(/-----END PRIVATE KEY-----/, '')
            .replace(/\s+/g, '');
        // Decodifica base64 para string binária
        const binary = atob(b64);
        // Cria um Uint8Array para armazenar os bytes
        const buffer = new Uint8Array(binary.length);
        for (let i = 0; i < binary.length; i++) {
            buffer[i] = binary.charCodeAt(i);
        }
        // Retorna o buffer como ArrayBuffer (formato aceito pela Web Crypto API)
        return buffer.buffer;
    }

    // 2. Converte PEM para ArrayBuffer para importar no Web Crypto
    const keyBuffer = pemToArrayBuffer(privateKeyPem);

    // 3. Importa a chave privada para o formato "CryptoKey" da Web Crypto API
    // - Tipo: pkcs8 (formato padrão para chaves privadas)
    // - Algoritmo: ECDSA com curva P-256
    // - Não é extratível (false)
    // - Uso permitido: assinatura ("sign")
    const cryptoKey = await window.crypto.subtle.importKey(
        "pkcs8",
        keyBuffer,
        { name: "ECDSA", namedCurve: "P-256" },
        false,
        ["sign"]
    );

    // 4. Assina o digest (array de bytes) usando a chave privada importada
    // - Algoritmo: ECDSA com SHA-256 para hashing
    // - Entrada: digestUint8Array (digest já calculado, ex: SHA-256)
    const signature = await window.crypto.subtle.sign(
        {
            name: "ECDSA",
            hash: { name: "SHA-256" },
        },
        cryptoKey,
        digestUint8Array
    );

    // 5. Verificação extra: assinatura deve ter exatamente 64 bytes (formato RAW concatenado de R e S)
    const sigArray = new Uint8Array(signature);
    if (sigArray.length !== 64) {
        throw new Error(`Tamanho inválido de assinatura: ${sigArray.length} bytes. Esperado 64 bytes (RAW)`);
    }

    // 6. Retorna a assinatura em formato RAW (Uint8Array de 64 bytes)
    return sigArray;
}