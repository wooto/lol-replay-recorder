import { expect } from 'chai';
import { LeagueClientUx } from '../../src/apis/LeagueClientUx';

describe('league-client', () => {
  const getSummoner = async () => {
    let leagueClient = new LeagueClientUx();
    return await leagueClient.getCurrentSummoner();
  };

  it('should getRoflsPath to be not null', async () => {
    let leagueClient = new LeagueClientUx();
    const path = await leagueClient.getRoflsPath();
    expect(path).to.not.null;
  });

  it('should getHighlightsFolderPath to be not null', async () => {
    let leagueClient = new LeagueClientUx();
    const path = await leagueClient.getHighlightsFolderPath();
    expect(path).to.not.null;
  });

  it('should getGameSettings to be not null', async () => {
    let leagueClient = new LeagueClientUx();
    const settings = await leagueClient.getGameSettings();
    expect(settings).to.not.null;
  });

  it('should getInputSettings to be not null', async () => {
    let leagueClient = new LeagueClientUx();
    const settings = await leagueClient.getInputSettings();
    expect(settings).to.not.null;
  });

  it('should getReplayConfig to be not null', async () => {
    let leagueClient = new LeagueClientUx();
    const config = await leagueClient.getReplayConfig();
    expect(config).to.not.null;
  });

  it('should getCurrentSummoner to be not null', async () => {
    let leagueClient = new LeagueClientUx();
    const summoner = await leagueClient.getCurrentSummoner();
    expect(summoner).to.not.null;
  });

  it('should getPatchVersion to be not null', async () => {
    let leagueClient = new LeagueClientUx();
    const patches = await leagueClient.getPatchVersion();
    expect(patches).to.not.null;
  });

  it.skip('should enableWindowMode to be not null', async () => {
    let leagueClient = new LeagueClientUx();
    try {
      await leagueClient.disableWindowMode();
    } catch (error) {
      // ignore
    }
    const settings = await leagueClient.enableWindowMode();
    expect(settings).to.not.null;
  });

  it('should getMatchHistoryByPuuid to be not null', async () => {
    let leagueClient = new LeagueClientUx();
    const { puuid } = await getSummoner();
    const matchHistory = await leagueClient.getMatchHistoryByPuuid(puuid, 0, 10);
    expect(matchHistory).to.not.null;
  });

  it('should getMatchTimelineByMatchId to be not null', async () => {
    let leagueClient = new LeagueClientUx();
    const { puuid } = await getSummoner();
    const matchHistory = await leagueClient.getMatchHistoryByPuuid(puuid, 0, 10);
    const match = matchHistory[0];
    const matchTimeline = await leagueClient.getMatchTimelineByMatchId(match.gameId);
    expect(matchTimeline).to.not.null;
  })

  it.skip('should downloadReplay to be not null', async () => {
    let leagueClient = new LeagueClientUx();
    const { puuid } = await getSummoner();
    const matchHistory = await leagueClient.getMatchHistoryByPuuid(puuid, 0, 10);
    const match = matchHistory[0];
    const replay = await leagueClient.downloadReplay(match.gameId);
    expect(replay).to.not.null;
  });

  // it.only('should launchReplay to be not null', async () => {
  //   let leagueClient = new LeagueClientUx();
  //   const { puuid } = await getSummoner();
  //   const matchHistory = await leagueClient.getMatchHistoryByPuuid(puuid, 0, 10);
  //   console.dir(matchHistory, {depth: null});
  //   const replay = await leagueClient.launchReplay(6994094218);
  //   expect(replay).to.not.null;
  // });
});
