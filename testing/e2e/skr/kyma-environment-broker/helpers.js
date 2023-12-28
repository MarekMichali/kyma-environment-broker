const {wait, debug} = require('../utils');
const {expect} = require('chai');
const fs = require('fs');
const os = require('os');

async function provisionSKR(
    keb,
    kcp,
    gardener,
    instanceID,
    name,
    platformCreds,
    btpOperatorCreds,
    customParams,
    timeout,
) {
  /*
  const resp = await keb.provisionSKR(
      name,
      instanceID,
      platformCreds,
      btpOperatorCreds,
      customParams,
  );
  expect(resp).to.have.property('operation');

  const operationID = resp.operation;
  debug(`Operation ID ${operationID}`);

  await ensureOperationSucceeded(keb, kcp, instanceID, operationID, timeout);

  debug('Fetching runtime operation status...');
  const runtimeStatus = await kcp.getRuntimeStatusOperations(instanceID);
  const objRuntimeStatus = JSON.parse(runtimeStatus);
  expect(objRuntimeStatus).to.have.nested.property('data[0].shootName').not.empty;
  debug('Fetching shoot info from gardener...');*/
 // const re = await kcp.getRuntimeEvents("4125AECA-3FD8-4049-B635-6F526B96D0D4");
  //console.log(re);
  // usage
  const operationID=`0b7aac57-0127-43e4-9d95-d9b1ad8c6b1a`;
  const shoot = await kcp.getKubeconfig2("https://kyma-env-broker.cp.dev.kyma.cloud.sap/kubeconfig/4F5D3511-11BE-4B3A-891A-704B5515E79A");
  //console.log(z)
 // const shoot = await kcp.getKubeconfig("c-0470bd5");
  //const shoot = await gardener.getShoot(objRuntimeStatus.data[0].shootName); //replace with kcp cli? it fetches here the shoot kubeconfig
  //console.log(shoot);
  //debug(`Compass ID ${shoot.compassID}`);
1 
  return {
    operationID,
    shoot,
  }; 
}

function ensureValidShootOIDCConfig(shoot, targetOIDCConfig) {
  expect(shoot).to.have.nested.property('oidcConfig.clientID', targetOIDCConfig.clientID);
  expect(shoot).to.have.nested.property('oidcConfig.issuerURL', targetOIDCConfig.issuerURL);
  expect(shoot).to.have.nested.property('oidcConfig.groupsClaim', targetOIDCConfig.groupsClaim);
  expect(shoot).to.have.nested.property('oidcConfig.usernameClaim', targetOIDCConfig.usernameClaim);
  expect(shoot).to.have.nested.property(
      'oidcConfig.usernamePrefix',
      targetOIDCConfig.usernamePrefix,
  );
  expect(shoot.oidcConfig.signingAlgs).to.eql(targetOIDCConfig.signingAlgs);
}

async function deprovisionSKR(keb, kcp, instanceID, timeout, ensureSuccess=true) {
  const resp = await keb.deprovisionSKR(instanceID);
  expect(resp).to.have.property('operation');

  const operationID = resp.operation;
  console.log(`Deprovision SKR - operation ID ${operationID}`);

  if (ensureSuccess) {
    await ensureOperationSucceeded(keb, kcp, instanceID, operationID, timeout);
  }

  return operationID;
}

async function updateSKR(keb,
    kcp,
    gardener,
    instanceID,
    shootName,
    customParams,
    timeout,
    btpOperatorCreds = null,
    isMigration = false) {
  const resp = await keb.updateSKR(instanceID, customParams, btpOperatorCreds, isMigration);
  expect(resp).to.have.property('operation');

  const operationID = resp.operation;
  debug(`Operation ID ${operationID}`);

  await ensureOperationSucceeded(keb, kcp, instanceID, operationID, timeout);

  const shoot = await gardener.getShoot(shootName);

  return {
    operationID,
    shoot,
  };
}

async function ensureOperationSucceeded(keb, kcp, instanceID, operationID, timeout) {
  const res = await wait(
      () => keb.getOperation(instanceID, operationID),
      (res) => res && res.state && (res.state === 'succeeded' || res.state === 'failed'),
      timeout,
      1000 * 30, // 30 seconds
  ).catch(async (err) => {
    const runtimeStatus = await kcp.getRuntimeStatusOperations(instanceID);
    const events = await kcp.getRuntimeEvents(instanceID);
    const msg = `${err}\nError thrown by ensureOperationSucceeded: Runtime status: ${runtimeStatus}`;
    throw new Error(`${msg}\nEvents:\n${events}`);
  });

  if (res.state !== 'succeeded') {
    const runtimeStatus = await kcp.getRuntimeStatusOperations(instanceID);
    throw new Error(`Error thrown by ensureOperationSucceeded: operation didn't succeed in time:
     ${JSON.stringify(res, null, `\t`)}\nRuntime status: ${runtimeStatus}`);
  }

  console.log(`Operation ${operationID} finished with state ${res.state}`);

  return res;
}

async function getShootName(keb, instanceID) {
  const resp = await keb.getRuntime(instanceID);
  expect(resp.data).to.be.lengthOf(1);

  return resp.data[0].shootName;
}

async function getCatalog(keb) {
  return keb.getCatalog();
}

async function ensureValidOIDCConfigInCustomerFacingKubeconfig(keb, instanceID, oidcConfig) {
  let kubeconfigContent;
  try {
    kubeconfigContent = await keb.downloadKubeconfig(instanceID);
  } catch (err) {
    console.log(err);
  }

  const issuerMatchPattern = '\\b' + oidcConfig.issuerURL + '\\b';
  const clientIDMatchPattern = '\\b' + oidcConfig.clientID + '\\b';
  expect(kubeconfigContent).to.match(new RegExp(issuerMatchPattern, 'g'));
  expect(kubeconfigContent).to.match(new RegExp(clientIDMatchPattern, 'g'));
}

async function saveKubeconfig(kubeconfig) {
  const directory = `${os.homedir()}/.kube`;
  if (!fs.existsSync(directory)) {
    fs.mkdirSync(directory, {recursive: true});
  }

  fs.writeFileSync(`${directory}/config`, kubeconfig);
}

module.exports = {
  provisionSKR,
  deprovisionSKR,
  saveKubeconfig,
  updateSKR,
  ensureOperationSucceeded,
  getShootName,
  ensureValidShootOIDCConfig,
  ensureValidOIDCConfigInCustomerFacingKubeconfig,
  getCatalog,
};
