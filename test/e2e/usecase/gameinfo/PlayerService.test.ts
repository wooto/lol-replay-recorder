import 'reflect-metadata';

import { expect } from 'chai';
import { Container } from 'typedi';
import { PlayerService } from '../../../../src/usecases/gameInfo/PlayerService';
import { RiotTypes } from '../../../../src/model/RiotTypes';
import PlatformId = RiotTypes.PlatformId;

describe('PlayerService', () => {
  const playerService = Container.get(PlayerService);

  it('should recent games', async () => {
    const recentGames = await playerService.fetchRecentProMatches();
    expect(recentGames).to.be.an('array');
  }).timeout(1000 * 10);

  it('should getBestGameByPlayer be not throw error', async () => {
    await playerService.getBestGameByUuid({
      cluster: PlatformId.ASIA,
      puuid: 'WX6I0zYYYc75HfZU-0XK0fxYIth3pJO1tm2BPe6DySABUPhUHlIffhBOkdVUt1pICzrbStMH05e4LQ',
    });
    expect(true).to.be.eq(true);
  }).timeout(1000 * 10);

  // it.only('should fetchProPlayers be greater than 0', async () => {
  //   const players = await playerService.fetchProPlayers({ region: PlatformId.KR });
  //   console.log(players);
  //   expect(players.length).to.be.greaterThan(0);
  // }).timeout(100000);
});
