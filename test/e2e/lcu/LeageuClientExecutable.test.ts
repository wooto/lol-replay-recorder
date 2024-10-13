import { LeagueClient } from '../../../src/controller/LeagueClient';
import { expect } from 'chai';

describe('LeagueClientExecutable', () => {
  it('should be able to find the LeagueClient executable', async () => {
    const paths = await new LeagueClient().getInstalledPaths();
    expect(paths).not.to.be.empty;
  });

  it('should be able to find the LeagueClient config file', async () => {
    const paths = await new LeagueClient().getConfigFilePaths();
    expect(paths).not.to.be.empty;
  });

  // This test is not working because the LeagueClient is not running
  it.skip('should be able to enable the LeagueClient config file, set true', async () => {
    const [path] = await new LeagueClient().getConfigFilePaths();
    await new LeagueClient().setGameEnabled(path, true);
    const enabled = await new LeagueClient().isGameEnabled(path);
    expect(true).to.be.eql(enabled);
  });
});
