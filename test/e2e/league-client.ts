import { LeagueClient } from '../../src/usecases/recorder/apis/league-client';
import { expect } from 'chai';

describe('league-client', () => {
  it('should be tested', async () => {
    let leagueClient = new LeagueClient();
    const path = await leagueClient.getRoflsPath();
    expect(path).to.not.null;
  });
});
