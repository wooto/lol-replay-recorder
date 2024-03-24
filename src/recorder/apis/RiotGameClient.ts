import { existsSync, readFileSync } from 'node:fs';
import path from 'node:path';
import { spawn } from 'node:child_process';

const rcsExePath = `"C:\\Riot Games\\Riot Client\\RiotClientServices.exe"`;

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
    const localAppData = process.env.LOCALAPPDATA;
    return path.join(localAppData, 'Riot Games', 'Riot Client', 'Config', 'lockfile');
  }

  async getLockfileCredentials(path: string): Promise<{ port: string; password: string }> {
    const data = readFileSync(path, 'utf8');
    const parts = data.split(':');
    return { port: parts[2], password: parts[3] };
  };


  async startRiotClient(region: string = 'KR'): Promise<void> {
    return new Promise((resolve, reject) => {
      const process = spawn(rcsExePath,
        ['--launch-product=league_of_legends', `--launch-patchline=live`, `--region=${region.toUpperCase()}`],
        { shell: true });

      process.on('error', (error) => {
        console.error('Failed to start Riot Client Services:', error);
        reject(error);
      });

      process.on('close', (code) => {
        console.log(`Riot Client Services process exited with code ${code}`);
        resolve();
      });
    });
  };
}


export default new RiotGameClient();
