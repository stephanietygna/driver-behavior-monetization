// codes_v1_E1.js
///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
//                                       ANÁLISES INICIAIS
///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

const fs = require('fs');
const csv = require('csv-parser');
const streamifier = require('streamifier');
const createCsvWriter = require('csv-writer').createObjectCsvWriter;
const path = require('path');

// Função para substituir valores infinitos e NaN por 0
function replaceInfAndNaN(value) {
    if (!isFinite(value) || isNaN(value)) {
        return 0;
    }
    return value;
}

// Função para processar dados numéricos
function toNumeric(value) {
    const num = parseFloat(value);
    return isNaN(num) ? 0 : num;
}

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// PARTE 1 - IMPORTANDO E ARRUMANDO OS DADOS
///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

async function processData(csvString) {
    // Passo 1: Importando as bases de dados
    // =========================================================================================================================================================
    const data = [];

    return new Promise((resolve, reject) => {
        streamifier.createReadStream(csvString)
            .pipe(csv())
            .on('data', (row) => {
                data.push(row);
            })
            .on('end', () => {
                console.log('CSV file successfully processed');

                // Passo 2: Imputando manualmente a eficiência de cada automóvel
                // =========================================================================================================================================================
                const city_gasoline = [10.3, 10.3, 10.3, 10.3, 12.15, 12.15, 12.15, 12.15, 12.6, 12.6, 12.6, 12.6, null, 12.83, 12.83, 12.83, 12.83, 11.6, 11.6, 11.6, 11.6, 11.6, 11.6, 11.6, 11.6, 11.6, 11.6, 11.6, 11.6, 11.6, 11.6, 12, 12];
                const road_gasoline = [11.3, 11.3, 11.3, 11.3, 13.65, 13.65, 13.65, 13.65, 13.9, 13.9, 13.9, 13.9, null, 14.44, 14.44, 14.44, 14.44, 14.1, 14.1, 14.1, 14.1, 14.1, 14.1, 14.1, 14.1, 14.1, 14.1, 14.1, 14.1, 14.1, 14.1, 14.4, 14.4];
                const city_ethanol = [null, null, null, null, 8.2, 8.2, 8.2, 8.2, 8.9, 8.9, 8.9, 8.9, null, 9.11, 9.11, 9.11, 9.11, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8.3, 8.3];
                const road_ethanol = [null, null, null, null, 9.5, 9.5, 9.5, 9.5, 9.8, 9.8, 9.8, 9.8, null, 10.26, 10.26, 10.26, 10.26, 9.8, 9.8, 9.8, 9.8, 9.8, 9.8, 9.8, 9.8, 9.8, 9.8, 9.8, 9.8, 9.8, 9.8, 10, 10];

                // Passo 3: Adicionando os vetores como novas colunas no data
                // =========================================================================================================================================================
                data.forEach((row, index) => {
                    row.city_gasoline = city_gasoline[index] !== null ? city_gasoline[index] : 0;
                    row.road_gasoline = road_gasoline[index] !== null ? road_gasoline[index] : 0;
                    row.city_ethanol = city_ethanol[index] !== null ? city_ethanol[index] : 0;
                    row.road_ethanol = road_ethanol[index] !== null ? road_ethanol[index] : 0;
                });

                // Passo 4: Adicionando manualmente a cotação do carbono
                // =========================================================================================================================================================
                const Carbon_Price_European = [67.13, 67.13, 67.69, 67.69, 67.13, 67.13, 67.13, 67.13, 80.91, 80.74, 69.88, 67.13, 68.98,
                    67.13, 67.13, 67.13, 67.13, 80.91, 80.91, 80.92, 78.64, 78.64, 78.64, 78.64, 78.64, 69.56,
                    68.69, 68.69, 67.13, 67.1, 67.69, 67.91, 65.25];

                // Passo 5: Adicionando manualmente a cotação do Euro
                // =========================================================================================================================================================
                const Euro_price = [6.1708, 6.1708, 6.1447, 6.1447, 6.1708, 6.1708, 6.1708, 6.1708, 6.1031, 6.0524, 5.9424, 6.1708, 6.1315,
                    6.1708, 6.1708, 6.1708, 6.1708, 6.1031, 6.1031, 5.9710, 5.9851, 5.9851, 5.9851, 5.9851, 5.9851,
                    6.2429, 6.2070, 6.2070, 6.1708, 6.1708, 6.1447, 6.1031, 6.2200];

                data.forEach((row, index) => {
                    row.Carbon_Price_European = Carbon_Price_European[index];
                    row.Euro_price = Euro_price[index];

                    // Passo 6: Colocando a cotação em termos reais
                    // =========================================================================================================================================================
                    row.Real_price = row.Carbon_Price_European * row.Euro_price;

                    // Passo 7: Criando variável da proporção de gasolina no tanque
                    // =========================================================================================================================================================
                    row.Tanque_gasoline = 100 - toNumeric(row['ethanol (%)']);
                });

                ///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
                // PARTE 2 - CALCULANDO A META DE EMISSÃO
                ///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

                // Meta CO2 = PARTE 1 + PARTE 2
                // PARTE 1 = Distância_estrada * [(1/Consumo de gasolina na estrada) * Proporção_Gasolina_Tanque * Emissão de CO2 por litro de gasolina + (1/Consumo de etanol na estrada) * Proporção_Etanol_Tanque * Emissão de CO2 por litro de etanol]
                // PARTE 2 = Distância_cidade * [(1/Consumo de gasolina na cidade) * Proporção_Gasolina_Tanque *  Emissão de CO2 por litro de gasolina + (1/Consumo de etanol na cidade) * Proporção_Etanol_Tanque * Emissão de CO2 por litro de etanol]

                data.forEach(row => {
                    // Converter valores string para números
                    const highwayDistance = toNumeric(row['highway (distance)']);
                    const cityDistance = toNumeric(row['city (distance)']);
                    const ethanolPercent = toNumeric(row['ethanol (%)']);
                    const co2EtanolOriginal = toNumeric(row['co2_etanol_original_gas_1720_flex']);

                    // PARTE 1
                    let parte_1_1 = highwayDistance * (((1 / row.road_gasoline) * (row.Tanque_gasoline / 100) * 1.720)) * 1000;
                    let parte_1_2 = highwayDistance * (((1 / row.road_ethanol) * (ethanolPercent / 100) * 1.501)) * 1000;

                    // Substituir valores NaN e infinitos por 0
                    parte_1_1 = replaceInfAndNaN(parte_1_1);
                    parte_1_2 = replaceInfAndNaN(parte_1_2);

                    const parte_1 = parte_1_1 + parte_1_2;

                    // PARTE 2
                    let parte_2_1 = cityDistance * (((1 / row.city_gasoline) * (row.Tanque_gasoline / 100) * 1.720)) * 1000;
                    let parte_2_2 = cityDistance * (((1 / row.city_ethanol) * (ethanolPercent / 100) * 1.501)) * 1000;

                    // Substituir valores NaN e infinitos por 0
                    parte_2_1 = replaceInfAndNaN(parte_2_1);
                    parte_2_2 = replaceInfAndNaN(parte_2_2);

                    const parte_2 = parte_2_1 + parte_2_2;

                    // META E DIFERENÇA SERÃO ADICIONADAS NO BLOCKCHAIN

                    /*
                    enviar pro blockchain...
                    parametros para calcular parte 1 e 2 la dentro mesmo...
                    */

                    // META
                    row.Meta_CO2 = parte_1 + parte_2;

                    // DIFERENÇA = EMISSÃO REAL - META
                    row.Diff = row.Meta_CO2 * 1.5 - co2EtanolOriginal;

                    // Valor E2
                    row.e2 = row.Diff * row.Real_price / 1000000;
                });

                console.log("--------------------------------------------");

                // Exportar a tabela como CSV
                exportToCsv(data)
                    .then(() => {
                        console.log("Tabela exportada como 'resultados_monetizacao_final.csv'");
                        resolve(data);
                    })
                    .catch(reject);
            })
            .on('error', reject);
    });
}

// Função para exportar dados para CSV
async function exportToCsv(data) {
    const csvWriter = createCsvWriter({
        path: '/home/ubuntu/fabric-inmetro/client/monetiza_e1/dados_UFRN/resultados_monetizacao_final.csv',
        header: Object.keys(data[0]).map(key => ({ id: key, title: key })),
        encoding: 'utf8'
    });

    return csvWriter.writeRecords(data);
}

// Função para executar apenas quando chamada explicitamente (removido a execução automática)
// A função processData() só será executada quando chamada pelo invoke.js

// Função específica para extrair dados de linha e colunas específicas
async function extrairDadosDeLinha(csvString, rowIndex, columnNames) {
    try {
        // Processar APENAS se não já foi processado
        console.log(`Extraindo dados da linha ${rowIndex}...`);

        // Ler e processar apenas a linha específica necessária
        const data = [];

        return new Promise((resolve, reject) => {
            streamifier.createReadStream(csvString)
                .pipe(csv())
                .on('data', (row) => {
                    data.push(row);
                })
                  .on('end', () => {
                    try {
                        if (rowIndex >= data.length || rowIndex < 0) {
                            throw new Error(`Índice de linha inválido: ${rowIndex}. Dados disponíveis: 0 a ${data.length - 1}`);
                        }

                        // Processar apenas a linha específica
                        const row = data[rowIndex];

                        // Aplicar as mesmas transformações que processData() faria
                        const city_gasoline = [10.3, 10.3, 10.3, 10.3, 12.15, 12.15, 12.15, 12.15, 12.6, 12.6, 12.6, 12.6, null, 12.83, 12.83, 12.83, 12.83, 11.6, 11.6, 11.6, 11.6, 11.6, 11.6, 11.6, 11.6, 11.6, 11.6, 11.6, 11.6, 11.6, 11.6, 12, 12];
                        const road_gasoline = [11.3, 11.3, 11.3, 11.3, 13.65, 13.65, 13.65, 13.65, 13.9, 13.9, 13.9, 13.9, null, 14.44, 14.44, 14.44, 14.44, 14.1, 14.1, 14.1, 14.1, 14.1, 14.1, 14.1, 14.1, 14.1, 14.1, 14.1, 14.1, 14.1, 14.1, 14.4, 14.4];
                        const city_ethanol = [null, null, null, null, 8.2, 8.2, 8.2, 8.2, 8.9, 8.9, 8.9, 8.9, null, 9.11, 9.11, 9.11, 9.11, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8.3, 8.3];
                        const road_ethanol = [null, null, null, null, 9.5, 9.5, 9.5, 9.5, 9.8, 9.8, 9.8, 9.8, null, 10.26, 10.26, 10.26, 10.26, 9.8, 9.8, 9.8, 9.8, 9.8, 9.8, 9.8, 9.8, 9.8, 9.8, 9.8, 9.8, 9.8, 9.8, 10, 10];

                        // Adicionar eficiências
                        row.city_gasoline = city_gasoline[rowIndex] !== null ? city_gasoline[rowIndex] : 0;
                        row.road_gasoline = road_gasoline[rowIndex] !== null ? road_gasoline[rowIndex] : 0;
                        row.city_ethanol = city_ethanol[rowIndex] !== null ? city_ethanol[rowIndex] : 0;
                        row.road_ethanol = road_ethanol[rowIndex] !== null ? road_ethanol[rowIndex] : 0;

                        // Adicionar preços de carbono e euro (arrays específicos)
                        const Carbon_Price_European = [67.13, 67.13, 67.69, 67.69, 67.13, 67.13, 67.13, 67.13, 80.91, 80.74, 69.88, 67.13, 68.98, 67.13, 67.13, 67.13, 67.13, 80.91, 80.91, 80.92, 78.64, 78.64, 78.64, 78.64, 78.64, 69.56, 68.69, 68.69, 67.13, 67.1, 67.69, 67.91, 65.25];
                        const Euro_price = [6.1708, 6.1708, 6.1447, 6.1447, 6.1708, 6.1708, 6.1708, 6.1708, 6.1031, 6.0524, 5.9424, 6.1708, 6.1315, 6.1708, 6.1708, 6.1708, 6.1708, 6.1031, 6.1031, 5.9710, 5.9851, 5.9851, 5.9851, 5.9851, 5.9851, 6.2429, 6.2070, 6.2070, 6.1708, 6.1708, 6.1447, 6.1031, 6.2200];

                        row.Carbon_Price_European = Carbon_Price_European[rowIndex];
                        row.Euro_price = Euro_price[rowIndex];
                        row.Real_price = row.Carbon_Price_European * row.Euro_price;
                        row.Tanque_gasoline = 100 - toNumeric(row['ethanol (%)']);


                        // Se columnNames não foi fornecido, retorna toda a linha
                        if (!columnNames) {
                            resolve(row);
                            return;
                        }

                        // Se columnNames é uma string, converte para array
                        if (typeof columnNames === 'string') {
                            columnNames = [columnNames];
                        }

                        // Extrair apenas as colunas solicitadas
                        const resultado = {};
                        columnNames.forEach(columnName => {
                            if (columnName in row) {
                                resultado[columnName] = row[columnName];
                            } else {
                                console.warn(`Coluna '${columnName}' não encontrada. Colunas disponíveis: ${Object.keys(row).join(', ')}`);
                                resultado[columnName] = null;
                            }
                        });

                        resolve(columnNames.length === 1 ? resultado[columnNames[0]] : resultado);

                    } catch (error) {
                        reject(error);
                    }
                })
                .on('error', reject);
        });

    } catch (error) {
        console.error(`Erro ao extrair dados da linha ${rowIndex}:`, error.message);
        throw error;
    }
}


// Exportar funções para uso como módulo
module.exports = {
    processData,
    extrairDadosDeLinha,
    // extrairDadosDeMultiplasLinhas,
    exportToCsv,
    replaceInfAndNaN,
    toNumeric,
    processData,
};