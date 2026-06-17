"use strict";

const { Gateway, Wallets } = require("fabric-network");
const yaml = require("js-yaml");
const fs = require("fs");
const path = require("path");
const { query } = require("express");
const { text } = require("stream/consumers");
const mspId = "INMETROMSP";
const CC_NAME = "braketester-external";
const CHANNEL = "demo";
let ccp = null;

// Função para invocar o chaincode
async function invoke(user, vehiclePlate) {
  try {
    console.log("Invoking chaincode using: ", user);

    // Load the network configuration
    const ccpPath = path.resolve(__dirname, "connection-org.yaml");
    if (ccpPath.includes(".yaml")) {
      ccp = yaml.load(fs.readFileSync(ccpPath, "utf8"));
    } else {
      ccp = JSON.parse(fs.readFileSync(ccpPath, "utf8"));
    }

    // Create a new file system-based wallet for managing identities
    const walletPath = path.join(process.cwd(), "wallet", mspId);
    const wallet = await Wallets.newFileSystemWallet(walletPath);
    console.log(`Wallet path: ${walletPath}`);

    // Check if the user identity exists in the wallet
    const identity = await wallet.get(user);
    if (!identity) {
      console.log(`An identity for the user "${user}" does not exist in the wallet`);
      console.log("Run the register.js application before retrying");
      return;
    }

    // Create a new gateway for connecting to our peer node
    const gateway = new Gateway();
    await gateway.connect(ccp, {
      wallet,
      identity: user,
      discovery: { enabled: true, asLocalhost: false },
    });

    // Get the network (channel) our contract is deployed to
    const network = await gateway.getNetwork(CHANNEL);

    // Get the contract from the network
    const contract = network.getContract(CC_NAME);

    const reportData = JSON.stringify({
      data: [4199, 4179, 2923, 2815, 3453, 3120, 1962, 1452, 3384],
    });

    // Submit a transaction
    await contract.submitTransaction("registerMeter", vehiclePlate, reportData);
    console.log("Transaction has been submitted ", vehiclePlate);

    gateway.disconnect();
  } catch (error) {
    console.error(`Failed to submit transaction: ${error}`);
    process.exit(1);
  }
}

// Função para consultar o ledger
async function queryLedger(vehiclePlate, user) {
  try {
    console.log("Querying all records from chaincode using plate:", vehiclePlate);
    //console.log(user, vehiclePlate, "banana");

    // Load the network configuration
    const ccpPath = path.resolve(__dirname, "connection-org.yaml");
    let ccp;
    if (ccpPath.includes(".yaml")) {
      ccp = yaml.load(fs.readFileSync(ccpPath, "utf8"));
    } else {
      ccp = JSON.parse(fs.readFileSync(ccpPath, "utf8"));
    }

    // Create a new file system-based wallet for managing identities
    const walletPath = path.join(process.cwd(), "wallet", mspId);
    const wallet = await Wallets.newFileSystemWallet(walletPath);
    console.log(`Wallet path: ${walletPath}`);

    // Check if the user identity exists in the wallet
    const identity = await wallet.get(user);
    if (!identity) {
      console.log(`An identity for the user "${user}" does not exist in the wallet`);
      console.log("Run the register.js application before retrying");
      return;
    }

    // Create a new gateway for connecting to our peer node
    const gateway = new Gateway();
    await gateway.connect(ccp, {
      wallet,
      identity: user,
      discovery: { enabled: true, asLocalhost: false },
    });

    // Get the network (channel) the contract is deployed to
    const network = await gateway.getNetwork(CHANNEL);

    // Get the contract from the network
    const contract = network.getContract(CC_NAME);

    // Execute the QueryAll function with an empty array
    console.log("Executing query function on the chaincode...");
    const resultBytes = await contract.evaluateTransaction("QueryLedger", vehiclePlate);

    // Parse the result to a readable format
    const result = JSON.parse(resultBytes.toString());
    const updatedResult = {
      ...result,  // Mantém todos os dados originais do JSON
    "VEHICLE_PLATE": vehiclePlate, // Adiciona a placa ao JSON
  };
    console.log("Query Result:", updatedResult);

    // Disconnect from the gateway
    gateway.disconnect();

    return result;
  } catch (error) {
    console.error(`Failed to query the ledger: ${error.message}`);
    process.exit(1);
  }
}

invoke("aa","ABC1234")