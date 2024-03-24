import { expect } from 'chai';
import RiotGameClient from '../../src/recorder/apis/RiotGameClient';

describe('RiotGameClient', () => {
  it('should be able to find the RiotGameClient executable', async () => {
    const path = await RiotGameClient.getClientPath();
    expect(path).not.to.be.empty;
  });

  it('should be able to find the RiotGameClient lockfile', async () => {
    const path = await RiotGameClient.getLockfilePath();
    expect(path).not.to.be.empty;
  });
});
