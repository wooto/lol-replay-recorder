import { expect } from 'chai';
import RiotGameClient from '../../src/apis/RiotGameClient';
import LeagueClientExecutable from '../../src/apis/LeagueClientExecutable';

describe('RiotGameClient', () => {
  it('should be able to find the RiotGameClient executable', async () => {
    const path = await RiotGameClient.getClientPath();
    expect(path).not.to.be.empty;
  });

  it('should be able to find the RiotGameClient lockfile', async () => {
    const path = await RiotGameClient.getLockfilePath();
    console.log(path);
    expect(path).not.to.be.empty;
  });

  it('should be able to find the RiotGameClient lockfile credentials', async () => {
    const path = await RiotGameClient.getLockfilePath();
    const credentials = await RiotGameClient.getLockfileCredentials(path);
    expect(credentials).not.to.be.empty;
  });

  it.skip('should be able to start the RiotGameClient', async () => {
    await LeagueClientExecutable.stopRiotProcesses();
    await RiotGameClient.startRiotClient();
    expect(await RiotGameClient.isRunning()).to.be.true;
  });
});
