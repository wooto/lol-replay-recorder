import 'reflect-metadata';

import { Container } from 'typedi';
import { expect } from 'chai';
import { afterEach } from 'mocha';
import { LeagueClientService } from '../../../../src/usecases/gameInfo/LeagueClientService';

describe.skip('LeagueClientSerivce', () => {
  afterEach(() => {
    Container.reset();
  });

  it('should get summoner data', async () => {
    const leagueClientService = Container.get(LeagueClientService);
    const summonerName = 'Gumayusi';
    const tagline = 'KR';
    const summonerData = await leagueClientService.getSummonerData({ summonerName, tagline });
    expect(summonerData).to.have.property('puuid');
  });

  it('should get all multi kill matches on current patch', async () => {
    const leagueClientService = Container.get(LeagueClientService);
    const summonerPuuid = '88a48d5a-11ab-5280-8a66-23ec1a3e5197';
    const matchList = await leagueClientService.getAllMultiKillMatchesOnCurrentPatch({
      summonerPuuid,
    });
    expect(matchList).to.be.an('array');
  });
});
