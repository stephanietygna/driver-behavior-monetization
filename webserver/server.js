// Importações de bibliotecas e módulos necessários
const { KJUR, KEYUTIL } = require('jsrsasign'); // Utilitários para assinatura/verificação ECDSA
const { Wallets } = require('fabric-network'); // API do Fabric para gerenciar carteiras de identidade
const bodyParser = require('body-parser'); // Middleware para parse de JSON no corpo das requisições
const favicon = require('serve-favicon'); // Middleware para servir o favicon
const jwt = require('jsonwebtoken'); // Biblioteca para criar/verificar tokens JWT
const express = require('express'); // Framework web para Node.js
const multer = require('multer'); // Middleware para manipulação de uploads de arquivos
const crypto = require('crypto'); // Biblioteca Node.js para funções criptográficas
const path = require('path'); // Utilitário para lidar com caminhos de arquivos
const https = require('https')
const xlsx = require('xlsx')
require('dotenv').config(); // Carrega variáveis de ambiente de um arquivo .env
const fs = require('fs'); // File system para ler/escrever arquivos

// Importa funções internas do projeto
const normalizeS = require('./resources/normalization.js'); // Função para normalizar assinatura ECDSA
const register = require('./resources/register.js'); // Função para registrar novo usuário

// Importa funções de processamento de csv para o chaincode monetiza
const {
    // processData,
    extrairDadosDeLinha,
    // extrairDadosDeMultiplasLinhas,
    // exportToCsv,
    // replaceInfAndNaN,
    // toNumeric,
    // //processData,
} = require('./resources/processCSV.js')

// Importa funções para interação com a rede Fabric (invocação de transações)
const {
    initialize,
    close,
    createProposal,
    createTransaction,
    createCommit,
    finalize
} = require('./resources/invoke.js');

// Configuração do multer para armazenar arquivos em memória
const upload = multer({ storage: multer.memoryStorage() });

// BLOCO para modificar o arquivo connection-org.yaml dinamicamente
///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Caminho do arquivo de origem (inmetro.yaml) e caminhos de destino
const originalFilePath = path.join(__dirname, '../resources/inmetro.yaml'); 
const destinationPaths = [
  path.join(__dirname, 'connection-org.yaml') 
];

// Função para copiar e modificar configuração do connection-org.yaml
const modifyFile = () => {
  if (fs.existsSync(originalFilePath)) {
    fs.readFile(originalFilePath, 'utf8', (err, data) => {
      if (err) {
        console.error("Erro ao ler o arquivo:", err);
        return;
      }

      destinationPaths.forEach((destinationFilePath) => {
        fs.writeFile(destinationFilePath, data, 'utf8', (err) => {
          if (err) {
            console.error("Erro ao copiar para o arquivo de destino:", err);
            return;
          }

          // Modifica a linha 5 e insere novas linhas após a linha 10
          const lines = data.split('\n');
          lines[4] = '  organization: "INMETROMSP"';
          lines.splice(10, 0, '    certificateAuthorities:');
          lines.splice(11, 0, '    - inmetro-ca.default');

          fs.writeFile(destinationFilePath, lines.join('\n'), 'utf8', (err) => {
            if (err) {
              console.error("Erro ao escrever no arquivo de destino:", err);
              return;
            }
            console.log(`Arquivo ${destinationFilePath} modificado com sucesso!`);
          });
        });
      });
    });
  } else {
    console.error("Arquivo original não encontrado!");
  }
};
modifyFile();
///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

let test_id_record
// Função auxiliar para converter base64 para hexadecimal
function b64tohex(base64) {
    return Buffer.from(base64, 'base64').toString('hex');
}

// Cria o app Express e define a porta
const app = express();
app.use(express.json({ limit: '50mb', type: 'application/json' }));
app.use(express.urlencoded({ limit: '50mb', extended: true }))

const port = 3000;

// Configura middlewares
app.use(bodyParser.json());

// Serve arquivos estáticos: recursos, views e favicon
app.use('/resources', express.static(path.join(__dirname, 'resources')));
app.use(express.static(path.join(__dirname, 'views')));
app.use(favicon(path.join(__dirname, 'favicon.ico')));
app.use(bodyParser.json());

// ----------------------------- ROTAS HTML -----------------------------

// Página inicial
app.get('/', (req, res) => {
    res.sendFile(path.join(__dirname, 'views', 'index.html'));
});

// Página de dúvidas
app.get('/help', (req, res) => {
    res.sendFile(path.join(__dirname, 'views', 'duvidas.html'));
});

// Página de login
app.get('/login', (req, res) => {
    res.sendFile(path.join(__dirname, 'views', 'login.html'));
});

// Página de registro
app.get('/register', (req, res) => {
    res.sendFile(path.join(__dirname, 'views', 'register.html'));
});

// Página da carteira
app.get('/carteira', (req, res) => {
    res.sendFile(path.join(__dirname, 'views', 'carteira.html'));
});

// Página dos chaincodes
app.get('/chaincode', (req, res) => {
    res.sendFile(path.join(__dirname, 'views', 'chaincode.html'));
});

// ----------------------------- ROTAS API -----------------------------

// Endpoint POST para registrar um novo usuário (envia CSR)
app.post('/register', upload.single('csr'), async (req, res) => { 
    const { username } = req.body;

    if (!username) {
        return res.status(400).json({ message: 'Username is required' });
    }

    if (!req.file) {
        return res.status(400).json({ message: 'CSR file is required' });
    }

    try {
        const csrPEM = req.file.buffer.toString('utf-8');
        await register(username, csrPEM);
        res.status(200).json({ success: true, message: `User '${username}' registered successfully` });
    } catch (error) {
        console.error("Erro ao registrar usuário:", error);
        
        if (error.message.includes('A user with the same public key already exists.')) {
            return res.status(409).json({ message: 'Já existe um usuário registrado com a mesma chave!' });
        }

        return res.status(500).json({ message: 'Failed to register user', error: error.message });
    }
});

// Endpoint POST para gerar e enviar um nonce de desafio para login
const nonceStore = {}; // Armazena nonce temporariamente por usuário
app.post('/get-nonce', async (req, res) => {
    try {
    const { username } = req.body;

    if (!username) {
        return res.status(400).json({ message: 'Username é obrigatório' });
    }

    const walletPath = path.join(process.cwd(), 'wallet', 'INMETROMSP');
    const wallet = await Wallets.newFileSystemWallet(walletPath);
    const identity = await wallet.get(username);

    if (!identity){
        return res.status(404).json({ message: "Usuário não registrado!" });
    }

    const nonce = crypto.randomBytes(32).toString('hex');
    nonceStore[username] = nonce;
    res.json({ success: true, nonce });
    } catch (error){
        res.status(500).json({ success: false, message: error.message })
    }
});

// Endpoint POST para verificar assinatura do nonce (desafio-resposta) e gerar JWT
app.post('/verify-login', async (req, res) => {
    try {
        const { signature, nonce } = req.body;

        // Busca o username que gerou este nonce
        const username = Object.keys(nonceStore).find(
            (key) => nonceStore[key] === nonce
        );
        if (!username){
            return res.status(400).json({ success: false, message: "Nonce inválido ou expirado" });
        }

        console.log('Verificando login:', { username, nonce, storedNonce: nonceStore[username] });

        // Confere se o nonce ainda é válido
        if (!nonceStore[username] || nonceStore[username] !== nonce) {
            return res.status(400).json({ success: false, message: 'Nonce inválido ou expirado' });
        }

        delete nonceStore[username]; // Invalida o nonce após uso

        // Verifica assinatura do nonce usando certificado da carteira
        const walletPath = path.join(process.cwd(), 'wallet', 'INMETROMSP');
        const wallet = await Wallets.newFileSystemWallet(walletPath);
        const identity = await wallet.get(username);

        if (!identity) {
            return res.status(404).json({ success: false, message: 'Usuário não encontrado' });
        }

        const cert = KEYUTIL.getKey(identity.credentials.certificate);
        const sig = new KJUR.crypto.Signature({ alg: 'SHA256withECDSA' });
        sig.init(cert);
        sig.updateString(nonce);
        const isValid = sig.verify(b64tohex(signature));

        if (!isValid) {
            return res.status(401).json({ success: false, message: 'Assinatura inválida' });
        }

        // Gera JWT para autenticação
        const token = jwt.sign(
            { sub: username, username: username },
            process.env.JWT_SECRET,
            { expiresIn: process.env.JWT_EXPIRES || '1h', algorithm: 'HS256' }
        );

        res.json({ success: true, token });

    } catch (error) {
        res.status(500).json({ success: false, message: error.message });
    }
});

// Endpoint POST para criar uma proposta de transação (invoke)
app.post('/invoke', async (req, res) => {
    const token = req.headers.authorization?.split(' ')[1];
    if (!token) return res.status(400).json({ message: "Token não fornecido" });

    let username, proposalDigest;

    try {
        const decoded = jwt.verify(token, process.env.JWT_SECRET);
        username = decoded.sub || decoded.username;
        if (!username) {
            return res.status(400).json({ message: "Token válido, mas sem nome de usuário." });
        }
    } catch (error) {
        console.error("Erro ao verificar token:", error.message);
        return res.status(401).json({ message: 'Token inválido ou expirado' });
    }

    const { fcn, args, vehiclePlate, chaincode } = req.body;

    const proposalArgs = Array.isArray(args) ? args.map(a => String(a)) : [];

    if (!vehiclePlate) {
        return res.status(400).json({ message: 'Placa do veículo é obrigatória.' });
    }

    // Define chaincode específico e cria proposta de acordo com o tipo
    if (chaincode == 'braketester'){
        await initialize(username, 'braketester-external');
        const serializedArgs = [ proposalArgs[0], JSON.stringify(args[1]) ];
        proposalDigest = await createProposal(fcn, ...serializedArgs);
    }
    if (chaincode == 'vehicle'){
        await initialize(username, 'vehicle-external');
        proposalDigest = await createProposal(fcn, ...proposalArgs);
    }
    if (chaincode == 'monetiza'){
        await initialize(username, 'monetiza');
        
        const fileDataText = args[0]
        console.log(fileDataText)
        const linha = args[1]
        const bool = args[2]
        const eZero = 'false'
        if (bool){
            eZero = 'true';
        }
        
        async function processarDados(fileDataText, linha) {
            try {
                console.log("Iniciando processamento dos dados...");

                // Definir as colunas que queremos extrair, essa variável é usada na função extrairDadosDeLinha
                const colunas = [
                'highway (distance)',
                'city (distance)',
                'ethanol (%)',
                'co2_etanol_original_gas_1720_flex',

                'road_gasoline',
                'road_ethanol',
                'city_gasoline',
                'city_ethanol',

                'Tanque_gasoline',
                'Real_price'
                ];

                // (numero da linha a ser lida, colunas a serem extraídas)
                const dadosObjeto = await extrairDadosDeLinha(fileDataText, linha, colunas);

                // Converter o objeto em array de strings na ordem das colunas
                const dadosArray = colunas.map(coluna => {
                    const valor = dadosObjeto[coluna];
                    return valor !== null && valor !== undefined ? String(valor) : '0';
                });

                console.log('Dados extraídos como array:');
                console.log(dadosArray);

                return dadosArray;

            } catch (error) {
                console.error('Erro ao processar dados:', error.message);
                return null;
            }
        }

        const dados = await processarDados(fileDataText, linha);
        if (dados){
            const vehicleId = vehiclePlate;
            const timestamp = new Date().toISOString();

            console.log("dados extraidos:", dados);
            proposalDigest = await createProposal(
                fcn,
                vehicleId,           // id

                dados[0],            // highway distance
                dados[1],            // city distance
                dados[2],            // ethanol percent
                dados[3],            // co2_etanol_original

                dados[4],            // road_gasoline
                dados[5],            // road_ethanol
                dados[6],            // city_gasoline
                dados[7],            // city_ethanol

                dados[8],            // tanque_gasoline
                dados[9],            // real_price

                timestamp,           // timestamp
                eZero                // etanol_zero
            );
        }
    } else if(chaincode==='sollytch'){
        await initialize(username, 'sollytch-chain')
        proposalDigest = await createProposal(fcn, ...proposalArgs)
    }

    return res.status(200).json({
        message: "Proposta criada com sucesso",
        username,
        vehiclePlate,
        proposalDigest: proposalDigest.toString('base64')
    });
});

// Endpoint POST para consulta (query) na ledger com proposta assinada
app.post('/query', async (req, res) => {
    const token = req.headers.authorization?.split(' ')[1];
    if (!token) return res.status(400).json({ message: "Token não fornecido" });

    let username, fcn, args, proposalDigest;

    try {
        const decoded = jwt.verify(token, process.env.JWT_SECRET);
        username = decoded.sub || decoded.username;
        if (!username) {
            return res.status(400).json({ message: "Token válido, mas sem nome de usuário." });
        }
    } catch (error) {
        console.error("Erro ao verificar token:", error.message);
        return res.status(401).json({ message: 'Token inválido ou expirado' });
    }

    const { vehiclePlate, chaincode, reportData } = req.body;
    if (!vehiclePlate) {
        return res.status(400).json({ message: 'Placa do veículo é obrigatória.' });
    }

    if (chaincode == 'braketester'){
        await initialize(username, 'braketester-external');
        args = [ vehiclePlate, JSON.stringify(reportData) ];
        fcn = 'QueryLedger';
        proposalDigest = await createProposal(fcn, ...args);
    }

    if (chaincode == 'vehicle'){
        await initialize(username, 'vehicle-external');
        args = [ vehiclePlate, JSON.stringify(reportData) ];
        fcn = 'QueryVehicleWallet';
        proposalDigest = await createProposal(fcn, ...args);
    }

    if (chaincode == 'monetiza'){
        await initialize(username, 'monetiza');
        args = [ vehiclePlate, JSON.stringify(reportData) ];
        fcn = 'ReadVehicle';
        proposalDigest = await createProposal(fcn, vehiclePlate);
    }

    if (chaincode == 'sollytch'){
        await initialize(username, 'sollytch-chain');
        args = [ vehiclePlate, JSON.stringify(reportData) ];
        fcn = 'GetAllTests';
        proposalDigest = await createProposal(fcn, vehiclePlate);
    }

    return res.status(200).json({
        message: "Proposta criada com sucesso",
        username,
        vehiclePlate,
        proposalDigest: proposalDigest.toString('base64')
    });
});

// Endpoint POST para criar transação a partir da proposta assinada
app.post('/createTransaction', async (req, res) => {
    try {
        const canonicalTransSig = normalizeS(req.body.proposalSig);
        const transactionDigest = await createTransaction(canonicalTransSig);
        return res.status(200).json({
            message: "Transação criada com sucesso",
            transactionDigest: transactionDigest.toString('base64')
        });
    } catch (e) {
        console.error(e);
        return res.status(400).json({ message: 'erro criando transacao' });
    }
});

// Endpoint POST para criar commit da transação assinada
app.post('/createCommit', async (req, res) => {
    try {
        const canonicalCommitSig = normalizeS(req.body.transactionSig);
        const commitDigest = await createCommit(canonicalCommitSig);
        return res.status(200).json({
            message: "Commit criado com sucesso",
            commitDigest: commitDigest.toString('base64')
        });
    } catch (e) {
        console.error(e);
        return res.status(400).json({ message: 'erro criando commit' });
    }
});

// Endpoint POST para finalizar a transação (commit assinado)
app.post('/finalize', async (req, res) => {
    try {
        const canonicalFinalSig = normalizeS(req.body.commitSig);
        const finalizeResponse = await finalize(canonicalFinalSig);
        close();
        return res.status(200).json({
            message: "Transação finalizada com sucesso",
            data: finalizeResponse || {}
        });
    } catch (error) {
        console.error("Erro ao finalizar transação:", error.message);
        return res.status(500).json({ message: "Erro ao finalizar transação." });
    }
});

const options = {
    key: fs.readFileSync('./certs/server.key'),
    cert: fs.readFileSync('./certs/server.cert')
}
// Inicializa o servidor na porta definida
https.createServer(options,app).listen(port,() => {
    console.log(`Servidor levantado na porta ${port}`)
})

// app.listen(port, () => {
//     console.log(`Server listening at http://localhost:${port}`);
// });