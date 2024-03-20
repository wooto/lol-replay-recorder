import { expect } from 'chai';
import { OpGGRepository } from '../../../src/infrastructure/player/OpGGRepository';

describe.skip('CrawlerService.integration', () => {
  it('_getBuildId', async () => {
    const buildId = await new OpGGRepository().getBuildId();
    console.log(buildId);
    expect(buildId).not.null;
  });

  it('fetchRecentGames', async () => {
    const gamesData = await new OpGGRepository().fetchRecentProMatches();
    expect(gamesData).not.null;
    console.log(gamesData);
  });

  it('getPlayerInfo', async () => {
    const gamesData = await new OpGGRepository().getPlayerInfo();
    expect(gamesData).not.null;
    console.log(gamesData);
  });
});
