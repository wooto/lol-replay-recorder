import { LeagueClient } from '../../src/usecases/recorder/apis/league-client';
import { expect } from 'chai';

describe('league-client', () => {
  const getSummoner = async () => {
    let leagueClient = new LeagueClient();
    return await leagueClient.getCurrentSummoner();
  };

  it('should getRoflsPath to be not null', async () => {
    let leagueClient = new LeagueClient();
    const path = await leagueClient.getRoflsPath();
    expect(path).to.not.null;
  });

  it('should getHighlightsFolderPath to be not null', async () => {
    let leagueClient = new LeagueClient();
    const path = await leagueClient.getHighlightsFolderPath();
    expect(path).to.not.null;
  });

  it('should getGameSettings to be not null', async () => {
    let leagueClient = new LeagueClient();
    const settings = await leagueClient.getGameSettings();
    expect(settings).to.not.null;
  });

  it('should getInputSettings to be not null', async () => {
    let leagueClient = new LeagueClient();
    const settings = await leagueClient.getInputSettings();
    expect(settings).to.not.null;
  });

  it('should getReplayConfig to be not null', async () => {
    let leagueClient = new LeagueClient();
    const config = await leagueClient.getReplayConfig();
    expect(config).to.not.null;
  });

  it('should getCurrentSummoner to be not null', async () => {
    let leagueClient = new LeagueClient();
    const summoner = await leagueClient.getCurrentSummoner();
    expect(summoner).to.not.null;
  });

  it('should getPatchVersion to be not null', async () => {
    let leagueClient = new LeagueClient();
    const patches = await leagueClient.getPatchVersion();
    expect(patches).to.not.null;
  });

  it('should enableWindowMode to be not null', async () => {
    let leagueClient = new LeagueClient();
    const settings = await leagueClient.enableWindowMode();
    expect(settings).to.not.null;
  });

  it('should getMatchHistoryByPuuid to be not null', async () => {
    let leagueClient = new LeagueClient();
    const { puuid } = await getSummoner();
    const matchHistory = await leagueClient.getMatchHistoryByPuuid(puuid, 0, 10);
    expect(matchHistory).to.not.null;
  });

  it('should getSummonersByRiotId to be not null', async () => {
    let leagueClient = new LeagueClient();
    const { riotId } = await getSummoner();
    const summoners = await leagueClient.getSummonersByRiotId(riotId);
    expect(summoners).to.not.null;
  });
});
