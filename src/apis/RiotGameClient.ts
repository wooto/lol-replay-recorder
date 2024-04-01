import { existsSync, readFileSync } from 'node:fs';
import path from 'node:path';
import { spawn } from 'node:child_process';
import * as fs from 'fs';
import { makeRequest } from '../model/RiotRequest';

const rcsExePath = `"C:\\Riot Games\\Riot Client\\RiotClientServices.exe"`;

async function invokeRiotRequest(lockfile: string, path: string, method: string = 'GET', body: any = null, retry: number = 3): Promise<any> {
  const lockContent = fs.readFileSync(lockfile, { encoding: 'utf8' }).split(':');
  const port = lockContent[2];
  const password = lockContent[3];
  const auth = Buffer.from(`riot:${password}`).toString('base64');

  const url = `https://127.0.0.1:${port}${path}`;

  const response = await makeRequest(
    method,
    url,
    {
      'Authorization': `Basic ${auth}`,
      'Content-Type': 'application/json',
    },
    body,
    retry,
  );

  if (!response.ok) {
    throw new Error(`Failed to ${method} '${path}'. Status: ${response.status}`);
  }

  return response.json();
}

export class RiotGameClient {
  async isRunning() {
    return invokeRiotRequest(
      await this.getLockfilePath(),
      '/lol-patch/v1/products/league_of_legends/state',
      'POST',
      {},
      10,
    );
  }

  async login(username: string, password: string) {
    return invokeRiotRequest(
      await this.getLockfilePath(),
      '/rso-auth/v1/authorization',
      'POST',
      {
        username,
        password,
      },
      5,
    );
  }

  //'/lol-patch/v1/products/league_of_legends/state'
  async getState(): Promise<{ action: string }> {
    return invokeRiotRequest(
      await this.getLockfilePath(),
      '/lol-patch/v1/products/league_of_legends/state',
      'GET',
      null,
      10,
    );
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


  async startRiotClient(region: string = 'KR', options?: { wait: boolean }): Promise<void> {
    options = options || { wait: false };

    new Promise((resolve, reject) => {
      const process = spawn(rcsExePath,
        ['--launch-product=league_of_legends', `--launch-patchline=live`, `--region=${region.toUpperCase()}`],
        { shell: true });

      process.on('error', (error) => {
        console.error('Failed to start Riot Client Services:', error);
        reject(error);
      });

      process.on('close', (code) => {
        console.log(`Riot Client Services process exited with code ${code}`);
        resolve(null);
      });
    });

    if (options?.wait) {
      for (let i = 0; i < 30; i++) {
        try {
          if (await this.isRunning()) {
            console.log('Riot Client Services is running.');
            return;
          }
        } catch (e) {
          console.log('Riot Client Services is not running yet.');
        }
        await new Promise((resolve) => {
          setTimeout(resolve, 1000);
        });
      }
    }
  };
}


