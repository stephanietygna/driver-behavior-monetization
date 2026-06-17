'use strict';

const { Wallets } = require('fabric-network');
const FabricCAServices = require('fabric-ca-client');
const fs = require('fs');
const path = require('path');
const yaml = require("js-yaml");
const admin = 'enroll'
const mspId = "INMETROMSP";
const caName = 'inmetro-ca.default';
let ccp = null;

async function register(user, csrPEM) {
    try {
        const ccpPath = path.resolve(__dirname, "../connection-org.yaml");
        ccp = yaml.load(fs.readFileSync(ccpPath, "utf8"));

        const caInfo = ccp.certificateAuthorities[caName];
        const caURL = caInfo.url;
        const ca = new FabricCAServices(caURL);

        const walletPath = path.join(process.cwd(), 'wallet', mspId);
        const wallet = await Wallets.newFileSystemWallet(walletPath);
        console.log(`Wallet path: ${walletPath}`);

        const userIdentity = await wallet.get(user);
        if (userIdentity) {
            console.log(`An identity for the user ${user} already exists in the wallet`);
            return;
        }

        const adminIdentity = await wallet.get(admin);
        if (!adminIdentity) {
            console.log(`An admin identity for the enroll user ${admin} does not exist in the wallet`);
            console.log('Run the enrollAdmin.js application before retrying');
            return;
        }

        const provider = wallet.getProviderRegistry().getProvider(adminIdentity.type);
        const adminUser = await provider.getUserContext(adminIdentity, admin);

        const secret = await ca.register({
            enrollmentID: user,
            role: 'client',
        }, adminUser);

        const enrollment = await ca.enroll({
            enrollmentID: user,
            enrollmentSecret: secret,
            csr: csrPEM
        });

        const x509Identity = {
            credentials: {
                certificate: enrollment.certificate,
            },
            mspId: mspId,
            type: 'X.509',
        };

        await wallet.put(user, x509Identity);
        console.log(`Successfully registered and enrolled user "${user}" with provided CSR.`);

    } catch (error) {
        console.error(`Failed to register user "${user}": ${error}`);
    }
}
module.exports = register;