import { expect } from 'chai';
import { Container } from 'typedi';
import { ProBuildStatsRepository } from '../../../../src/infrastructure/player/ProBuildStatsRepository';

describe('ProBuildStatsRepository', () => {
  it('should be fetch pro', async () => {
    const proBuildStatsRepository = Container.get(ProBuildStatsRepository);
    const proPlayers = await proBuildStatsRepository.fetchProPlayers();
    expect(proPlayers).to.be.an('array');
  });
});
