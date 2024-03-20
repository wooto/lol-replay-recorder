import { LeagueClient } from '../../src/usecases/recorder/apis/league-client';
import { expect } from 'chai';

describe('league-client', () => {
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
});
