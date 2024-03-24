import { expect } from 'chai';
import { LeagueClientExecution, RiotGameClient } from '../../src';

describe('RiotGameClient', () => {
  it('should be able to find the RiotGameClient executable', async () => {
    const path = await new RiotGameClient().getClientPath();
    expect(path).not.to.be.empty;
  });

  it('should be able to find the RiotGameClient lockfile', async () => {
    const path = await new RiotGameClient().getLockfilePath();
    console.log(path);
    expect(path).not.to.be.empty;
  });

  it('should be able to find the RiotGameClient lockfile credentials', async () => {
    const path = await new RiotGameClient().getLockfilePath();
    const credentials = await new RiotGameClient().getLockfileCredentials(path);
    expect(credentials).not.to.be.empty;
  });

  it.skip('should be able to start the RiotGameClient', async () => {
    await new LeagueClientExecution().stopRiotProcesses();
    await new RiotGameClient().startRiotClient();
    expect(await new RiotGameClient().isRunning()).to.be.true;
  });
});
