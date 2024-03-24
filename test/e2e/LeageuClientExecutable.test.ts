import LeagueClientExecutable from '../../src/recorder/apis/LeagueClientExecutable';
import { expect } from 'chai';

describe('LeagueClientExecutable', () => {
  it('should be able to find the LeagueClient executable', async () => {
    const paths = await LeagueClientExecutable.getInstalledPaths();
    expect(paths).not.to.be.empty;
  });

  it('should be able to find the LeagueClient config file', async () => {
    const paths = await LeagueClientExecutable.getConfigFilePaths();
    expect(paths).not.to.be.empty;
  });

  it('should be able to enable the LeagueClient config file, set true', async () => {
    const paths = await LeagueClientExecutable.getConfigFilePaths();
    await LeagueClientExecutable.setGameEnabled(paths[0], true);
    const enabled = await LeagueClientExecutable.isGameEnabled(paths[0]);
    expect(enabled).to.be.true;
  });

  it('should be able to enable the LeagueClient config file, set false', async () => {
    const paths = await LeagueClientExecutable.getConfigFilePaths();
    await LeagueClientExecutable.setGameEnabled(paths[0], false);
    const enabled = await LeagueClientExecutable.isGameEnabled(paths[0]);
    expect(enabled).to.be.false;
  });
});
