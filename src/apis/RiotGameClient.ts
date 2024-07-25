import { existsSync, readFileSync } from 'node:fs';
import path from 'node:path';
import { spawn } from 'node:child_process';
import * as fs from 'fs';
import { makeRequest } from '../model/RiotRequest';
import { refineRegion, sleepInSeconds } from '../utils/utils';
import { RiotTypes } from '../model/RiotTypes';
import { Locale } from '../model/Locale';
import { promisify } from 'util';

const rcsExePath = `"C:\\Riot Games\\Riot Client\\RiotClientServices.exe"`;

async function waitToExistsFile(filePath: string, timeout: number = 1000 * 60) {
  const start = Date.now();
  while (!fs.existsSync(filePath)) {
    if (Date.now() - start > timeout) {
      throw new Error('File not found');
    }
    await sleepInSeconds(1)
  }
}

async function invokeRiotRequest(lockfile: string, path: string, method: string = 'GET', body: any = null, retry: number = 3): Promise<any> {
  await waitToExistsFile(lockfile);
  const lockContent = (await promisify(fs.readFile)(lockfile, { encoding: 'utf8' })).split(':');
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

  return response;
}

export class RiotGameClient {
  async isRunning() {
    return invokeRiotRequest(
      await this.getLockfilePath(),
      '/lol-patch/v1/products/league_of_legends/state',
      'GET',
      null,
      0,
    );
  }

  async login(username: string, password: string) {
    return await invokeRiotRequest(
      await this.getLockfilePath(),
      '/rso-auth/v1/authorization/gas',
      'POST',
      {
        username,
        password,
      },
      0,
    );
  }

  async getState(): Promise<{ action: string }> {
    return invokeRiotRequest(
      await this.getLockfilePath(),
      '/lol-patch/v1/products/league_of_legends/state',
      'GET',
      null,
      0,
    );
  }

  async getInstalls(): Promise<any> {
    return await invokeRiotRequest(
      await this.getLockfilePath(),
      '/patch/v1/installs',
      'GET',
      null,
      30,
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

  async removeLockfile() {
    try {
      const lockfilePath = await this.getLockfilePath();
      await promisify(fs.unlink)(lockfilePath);
    }catch (e) {
      // ignore
    }
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


  async startRiotClient(region: RiotTypes.Region, locale: Locale): Promise<void> {
    const refinedRegion = refineRegion(region);
    new Promise((resolve, reject) => {
      const process = spawn(rcsExePath,
        [
          '--launch-product=league_of_legends',
          `--launch-patchline=live`,
          `--region=${refinedRegion.toUpperCase()}`,
          `--locale=${locale}`,
          '--skip-to-install',
        ],
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

    await new RiotGameClient().getInstalls();
    await this.waitToPatch();
  };

  async waitToPatch() {
    for (let i = 0; i < 300; i++) {
      try {
        const status = await invokeRiotRequest(
          await this.getLockfilePath(),
          '/patch/v1/installs/league_of_legends.live/status',
          'GET',
          null,
          0,
        );

        if (status.patch.state === 'up_to_date') {
          break;
        }

        console.log(`Installing LoL: ${status.patch.progress.progress}%`);
      }
      catch (e) {
        console.log('Failed to get patch status:', e);
      }
      await sleepInSeconds(1);
    }

  }
}


