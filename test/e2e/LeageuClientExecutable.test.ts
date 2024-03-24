import LeagueClientExecutable from '../../src/usecases/recorder/apis/LeagueClientExecutable';
import { expect } from 'chai';

describe('LeagueClientExecutable', () => {
  it('should be able to find the LeagueClient executable', async () => {
    const paths = await LeagueClientExecutable.getInstalledPaths();
    expect(paths).not.to.be.empty;
  });
});
