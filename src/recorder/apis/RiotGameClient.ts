import { readFileSync } from 'node:fs';
import LeagueClientExecutable from './LeagueClientExecutable';

class RiotGameClient {
  async isRunning() {
    return true;
  }

  async getClientPath(): Promise<string> {
    const path = await LeagueClientExecutable.getInstalledPaths()[0];
    return `${path}\\League of Legends\\LeagueClient.exe`;
  }

  async getLockfilePath(): Promise<string> {
    const path = await this.getClientPath();
    return `${path}\\lockfile`;
  }

  async getLockfileCredentials(path: string): Promise<{ port: string; password: string }> {
    const data = readFileSync(path, 'utf8');
    const parts = data.split(':');
    return { port: parts[2], password: parts[3] };
  };
}


export default new RiotGameClient();
