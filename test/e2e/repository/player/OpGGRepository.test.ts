import { expect } from 'chai';
import { OpGGRepository } from '../../../../src/infrastructure/player/OpGGRepository';

describe('OpGGClient', () => {
  const opGGRepository = new OpGGRepository();
  it('should get build id', async () => {
    const buildId = await opGGRepository.getBuildId();
    expect(buildId).to.be.a('string');
  });

  it('should fetch recent games', async () => {
    const recentGames = await opGGRepository.fetchRecentProMatches();
    expect(recentGames).to.be.an('array');
  });

  it('should filter recent games', async () => {
    const recentGames = await opGGRepository.filterRecentGames();
    expect(recentGames).to.be.an('array');
  });
});
