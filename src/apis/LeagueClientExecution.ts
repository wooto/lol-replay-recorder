import { exec } from 'child_process';
import { promisify } from 'util';
import { existsSync } from 'node:fs';
import { readFile, writeFile } from 'fs/promises';
import ini from 'ini';
import path from 'node:path';
import { sleepInSeconds } from '../utils/utils';
import { RiotGameClient } from './RiotGameClient';
import { LeagueClientUx } from './LeagueClientUx';
import { Locale } from '../model/Locale';
import { getActiveWindow, getWindows, mouse } from '@kirillvakalov/nut-tree__nut-js';
import { RiotTypes } from '../model/RiotTypes';
import { YamlEditor } from '../utils/YamlEditor';

const execAsync = promisify(exec);

export class LeagueClientExecution {
  async startRiotProcessesSafely(params: {
    region: RiotTypes.Region,
    locale: Locale,
    username: string,
    password: string,
  }) {
    await new LeagueClientExecution().stopRiotProcesses();
    this.setLocale(params.locale);
    for (let i = 0; i < 5; i++) {
      try {
        await new RiotGameClient().startRiotClient(params.region as any, params.locale);
        await new RiotGameClient().login(params.username, params.password, params.region);
        // await new LeagueClientUx().startClient({ region: params.region, locale: params.locale });
        const { action } = await new LeagueClientUx().getState({ options: { retry: 15 } });
        if (action !== 'Idle') {
          throw new Error('Client is not ready', action);
        }
        break;
      } catch (e) {
        console.error('Error starting Riot processes:', e);
        await sleepInSeconds(1);
      }
    }
    const { action } = await new LeagueClientUx().getState({ options: { retry: 10 } });
    if (action !== 'Idle') {
      throw new Error('Client is not ready', action);
    }

    const { locale } = await new LeagueClientUx().getRegionLocale();
    console.log('RCU response', await new RiotGameClient().getRegionLocale());
    console.log('LCU response', await new LeagueClientUx().getRegionLocale());

    if (locale !== params.locale) {
      throw new Error(`Locale is not correct: ${locale}`);
    }

    await sleepInSeconds(5);
  }

  async stopRiotProcesses() {
    const prcoesses = [
      'RiotClientUx.exe',
      'RiotClientServices.exe',
      'RiotClient.exe',
      'Riot Client.exe',
      'LeagueClient.exe',
      'League of Legends.exe',
      'LeagueClientUxRender.exe',
    ];
    for(const process of prcoesses) {
      try{
        await execAsync(`taskkill /F /IM "${process}" /T`);
      }catch (e) {
        // ignore
      }
    }

    for(const process of prcoesses) {
      for(let i = 0; i < 30; i++) {
        try {
          // process to check if the process is still running
          await sleepInSeconds(1);
          const { stdout } = await execAsync(`tasklist /FI "IMAGENAME eq ${process}"`);
          if (!stdout.includes(process)) {
            break;
          }
        } catch (e) {
          break;
        }
      }
    }
    await new RiotGameClient().removeLockfile();
    await new LeagueClientUx().removeLockfile();


  };
async findWindowsInstalled(): Promise<string[]> {
    const paths = ['C:\\Riot Games\\League of Legends'];
    for (const path of paths) {
      if (existsSync(path)) {
        return [path];
      }
    }
    return [];
  }

  getProductSettingsPath(): string {
    return path.join('C:', 'ProgramData', 'Riot Games', 'Metadata', 'league_of_legends.live', 'league_of_legends.live.product_settings.yaml');
  }

  async getInstalledPaths(): Promise<string[]> {
    return this.findWindowsInstalled();
  }

  async getConfigFilePaths(): Promise<string[]> {
    const installedPaths = await this.getInstalledPaths();
    return installedPaths.map(installedPath => {
      return this.getConfigFilePath(installedPath);
    });
  }

  getConfigFilePath(initialPath: string): string | null {
    let lolPath = path.resolve(initialPath);
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
      return value?.toString().toLowerCase() === 'true' || value === '1' || value === 1 || value === true;
    } catch (error) {
      return false;
    }
  }

  async setGameEnabled(path: string, enabled: boolean): Promise<void> {
    try {
      const fileContent = await readFile(path, { encoding: 'utf-8' });
      let config = ini.parse(fileContent);

      config.General = config.General || {};
      config.General.EnableReplayApi = enabled ? 1 : 0;

      const newFileContent = ini.stringify(config);
      await writeFile(path, newFileContent, { encoding: 'utf-8' });

    } catch (error) {
      console.error(`Error writing config file: ${error}`);
    }
  }

  async setLocale(locale: string) {
    const yamlEditor = new YamlEditor(this.getProductSettingsPath());

    const avaliable_locales: string[] = yamlEditor.data['locale_data']['available_locales'];
    if(!avaliable_locales.includes(locale)) {
      throw new Error(`Invalid locale: ${locale}, available locales: ${avaliable_locales}`);
    }
    yamlEditor.updateKey('locale_data.default_locale', locale);
    yamlEditor.updateKey('settings.locale', locale);
    yamlEditor.saveChanges();
  }


  async focusClientWindow(): Promise<void> {
    const targetWindowTitle = 'League of Legends (TM)';
    const windows = await getWindows();
    for (const window of windows) {
      if ((await window.getTitle()).includes(targetWindowTitle)) {
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
