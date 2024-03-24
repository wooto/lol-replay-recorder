import { exec } from 'child_process';
import { promisify } from 'util';
import { existsSync } from 'node:fs';
import { platform } from 'node:os';
import { readFile, writeFile } from 'fs/promises';
import ini from 'ini';
import path from 'node:path';
import { getActiveWindow, getWindows, mouse } from '@nut-tree/nut-js';
import { sleepInSeconds } from '../../utils/utils';

const execAsync = promisify(exec);

class LeagueClientExecution {
  private async findMacInstalled(): Promise<string[]> {
    const query = 'kMDItemCFBundleIdentifier==com.riotgames.leagueoflegends';
    try {
      const { stdout } = await execAsync(`mdfind ${query}`);
      const paths = stdout.split('\n').filter(line => {
        return line;
      });
      return paths;
    } catch (error) {
      console.error(`Error executing mdfind: ${error}`);
      throw error;
    }
  }

  private async findWindowsInstalled(): Promise<string[]> {
    const paths = ['C:\\Riot Games\\League of Legends'];
    for (const path of paths) {
      if (existsSync(path)) {
        return [path];
      }
    }
    return [];
  }

  async getInstalledPaths(): Promise<string[]> {
    const isWindows = process.platform === 'win32';
    if (isWindows) {
      return this.findWindowsInstalled();
    } else {
      return this.findMacInstalled();
    }
  }

  async getConfigFilePaths(): Promise<string[]> {
    const installedPaths = await this.getInstalledPaths();
    return installedPaths.map(installedPath => {
      return this.getConfigFilePath(installedPath);
    });
  }

  getConfigFilePath(initialPath: string): string | null {
    let lolPath = path.resolve(initialPath);
    if (platform() === 'darwin') {
      lolPath = path.join(lolPath, 'Contents', 'LoL');
    }
    let configPaths = [
      path.join(lolPath, 'DATA', 'CFG', 'game.cfg'),
      path.join(lolPath, 'Config', 'game.cfg'),
      path.join(lolPath, 'Game', 'Config', 'game.cfg'),
    ];
    for (const configPath of configPaths) {
      if (existsSync(configPath)) {
        return configPath;
      }
    }
    return null;
  }

  async isGameEnabled(path: string): Promise<boolean> {
    try {
      const fileContent = await readFile(path, { encoding: 'utf-8' });
      const config = ini.parse(fileContent);
      const value = config?.General?.EnableReplayApi;
      return value?.toString().toLowerCase() === 'true' || value === '1' || value === 1;
    } catch (error) {
      console.error(`Error reading config file: ${error}`);
      return false;
    }
  }

  async setGameEnabled(path: string, enabled: boolean): Promise<void> {
    try {
      const fileContent = await readFile(path, { encoding: 'utf-8' });
      let config = ini.parse(fileContent);

      config.General = config.General || {};
      config.General.EnableReplayApi = enabled ? '1' : '0';

      const newFileContent = ini.stringify(config);
      await writeFile(path, newFileContent, { encoding: 'utf-8' });

      console.info(`Setting EnableReplayApi ${path}=${enabled}`);
    } catch (error) {
      console.error(`Error writing config file: ${error}`);
    }
  }

  async focusClientWindow(): Promise<void> {
    const isWindows = process.platform === 'win32';
    const targetWindowTitle = isWindows ? 'League of Legends (TM) Client' : 'League of Legends';
    const windows = await getWindows();
    for (const window of windows) {
      if ((await window.getTitle()).includes(targetWindowTitle)) {
        console.log('Found League of Legends window', await window.getTitle());
        for (let i = 0; i < 10; i++) {
          await window.focus();
          const region = await window.getRegion();
          await mouse.move([
            {
              x: (region.left + region.width) / 2,
              y: (region.top + region.height) / 2,
            },
          ]);
          await mouse.leftClick();
          await mouse.leftClick();
          if ((await (await getActiveWindow()).getTitle()) === (await window.getTitle())) {
            return;
          }
          await sleepInSeconds(Math.min(2 ** i, 4));
        }
      }
    }

    throw new Error('Cannot find League of Legends window');
  }
}

export default new LeagueClientExecution();
