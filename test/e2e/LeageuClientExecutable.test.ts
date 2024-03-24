import { LeagueClientExecution } from '../../src/apis/LeagueClientExecution';
import { expect } from 'chai';

describe('LeagueClientExecutable', () => {
  it('should be able to find the LeagueClient executable', async () => {
    const paths = await new LeagueClientExecution().getInstalledPaths();
    expect(paths).not.to.be.empty;
  });

  it('should be able to find the LeagueClient config file', async () => {
    const paths = await new LeagueClientExecution().getConfigFilePaths();
    expect(paths).not.to.be.empty;
  });

  it('should be able to enable the LeagueClient config file, set false', async () => {
    const paths = await new LeagueClientExecution().getConfigFilePaths();
    await new LeagueClientExecution().setGameEnabled(paths[0], false);
    const enabled = await new LeagueClientExecution().isGameEnabled(paths[0]);
    expect(enabled).to.be.eql(false);
  });

  it('should be able to enable the LeagueClient config file, set true', async () => {
    const [path] = await new LeagueClientExecution().getConfigFilePaths();
    await new LeagueClientExecution().setGameEnabled(path, true);
    const enabled = await new LeagueClientExecution().isGameEnabled(path);
    expect(enabled).to.be.eql(true);
  });
});
