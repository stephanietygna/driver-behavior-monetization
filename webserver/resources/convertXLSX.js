const XLSX = require("xlsx");

function convertToCSV(fileBuffer) {
    // Lê o XLSX
    const workbook = XLSX.read(fileBuffer, { type: "buffer" });

    // Pega a primeira aba
    const firstSheetName = workbook.SheetNames[0];
    const sheet = workbook.Sheets[firstSheetName];

    // Converte para CSV
    const csv = XLSX.utils.sheet_to_csv(sheet);

    return csv;
}

function csvLineToJSON(csvLine) {
    // Validação robusta da linha CSV
    if (!csvLine || typeof csvLine !== 'string') {
        throw new Error('Linha CSV inválida ou vazia');
    }
    
    // Divide a linha CSV em colunas
    const columns = csvLine.split(',');
    
    // Verifica se columns é um array válido
    if (!Array.isArray(columns) || columns.length === 0) {
        throw new Error('Não foi possível dividir a linha CSV em colunas');
    }
    
    // A primeira coluna é o testID
    const testID = columns[0] || '';
    
    if (!testID) {
        throw new Error('testID não encontrado na primeira coluna do CSV');
    }
    
    // Mapeia as colunas para a estrutura TestRecord
    const testRecord = {
        test_id: testID,
        timestamp: columns[1] || '',
        lat: parseFloat(columns[2]) || 0,
        lon: parseFloat(columns[3]) || 0,
        geo_hash: columns[4] || '',
        operator_id: columns[5] || '',
        operator_did: columns[6] || '',
        matrix_type: columns[7] || '',
        cassette_lot: columns[8] || '',
        reagent_lot: columns[9] || '',
        expiry_days_left: parseInt(columns[10]) || 0,
        distance_mm: parseFloat(columns[11]) || 0,
        time_to_migrate_s: parseFloat(columns[12]) || 0,
        control_line_ok: columns[13]?.toLowerCase() === 'true',
        sample_volume_ul: parseFloat(columns[14]) || 0,
        sample_pH: parseFloat(columns[15]) || 0,
        sample_turbidity_NTU: parseFloat(columns[16]) || 0,
        sample_temp_C: parseFloat(columns[17]) || 0,
        ambient_T_C: parseFloat(columns[18]) || 0,
        ambient_RH_pct: parseFloat(columns[19]) || 0,
        lighting_lux: parseFloat(columns[20]) || 0,
        tilt_deg: parseFloat(columns[21]) || 0,
        preincubation_time_s: parseInt(columns[22]) || 0,
        time_since_sampling_min: parseFloat(columns[23]) || 0,
        storage_condition: columns[24] || '',
        prefilter_used: columns[25]?.toLowerCase() === 'true',
        image_taken: columns[26]?.toLowerCase() === 'true',
        image_blur_score: columns[27] ? parseFloat(columns[27]) : null,
        device_fw_version: columns[28] || '',
        produto_id: columns[29] || '',
        kit_calibration_id: columns[30] || '',
        controle_interno_result: columns[31] || '',
        cadeia_frio_status: columns[32]?.toLowerCase() === 'true',
        tempo_transporte_horas: parseFloat(columns[33]) || 0,
        condicao_transporte: columns[34] || '',
        estimated_concentration_ppb: parseFloat(columns[35]) || 0,
        incerteza_estimativa_ppb: parseFloat(columns[36]) || 0,
        acao_recomendada: columns[37] || '',
        result_class: columns[38] || '',
        qc_status: columns[39] || ''
    };
    
    // Converte para JSON string
    const jsonString = JSON.stringify(testRecord);
    
    return {
        testID: testID,
        jsonString: jsonString
    };
}

module.exports = {
    convertToCSV,
    csvLineToJSON
};