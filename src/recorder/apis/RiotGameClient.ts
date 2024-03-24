import { existsSync, readFileSync } from 'node:fs';

class RiotGameClient {
  async isRunning() {
    return true;
  }

  async getClientPath(): Promise<string[]> {
    const paths = ['C:\\Riot Games\\Riot Client'];
    for (const path of paths) {
      if (existsSync(path)) {
        return [path];
      }
    }
    return [];
  }

  async getLockfilePath(): Promise<string> {
    const path = await this.getClientPath();
    return `${path}\\Config\\lockfile`;
  }

  async getLockfileCredentials(path: string): Promise<{ port: string; password: string }> {
    const data = readFileSync(path, 'utf8');
    const parts = data.split(':');
    return { port: parts[2], password: parts[3] };
  };
}


export default new RiotGameClient();
