import { spawn } from 'node:child_process';
import CustomError from '../model/CustomError';
import { makeRequest } from '../model/LcuRequest';
import Summoner from '../model/Summoner';
import { refineRegion, sleepInSeconds } from '../utils/utils';
import { Locale } from '../model/Locale';
import path from 'node:path';
import { promisify } from 'util';
import * as fs from 'fs';
import { RiotTypes } from '../model/RiotTypes';

const lcExePath = `"C:\\Riot Games\\League of Legends\\LeagueClient.exe"`;

export class LeagueClientUx {
  patch: string;

  constructor() {
    this.patch = '';
  }

  // LeagueClient.exe
  async startClient(params: {
    region: RiotTypes.Region,
    locale: Locale
  }) {
    new Promise((resolve, reject) => {
      const process = spawn(lcExePath,
        [
          `--region=${refineRegion(params.region).toUpperCase()}`,
          `--locale=${params.locale}`,
        ],
        { shell: true });

      process.on('error', (error) => {
        console.error('Failed to start LeagueClient:', error);
        reject(error);
      });

      process.on('close', (code) => {
        console.log(`LeagueClient process exited with code ${code}`);
        resolve(null);
      });
    });
  }

  async waitForClientToBeReady() {
    for(let i = 0; i < 30; i++) {
      try {
        await this.getState({});
        console.log('Client is ready.');
        return true;
      } catch (e) {
        await sleepInSeconds(1);
      }
    }
    throw new CustomError('League Client took too long to start.');
  }

  /// GAME SETTINGS/CONFIGURATION REQUESTS ///
  async getHighlightsFolderPath(): Promise<string> {
    return await makeRequest('GET', '/lol-highlights/v1/highlights-folder-path');
  }

  async getGameSettings() {
    return await makeRequest('GET', '/lol-game-settings/v1/game-settings');
  }

  async getInputSettings() {
    return await makeRequest('GET', '/lol-game-settings/v1/input-settings');
  }

  async getRegionLocale(retries: number) {
    return await makeRequest('GET', '/riotclient/region-locale', null, retries);
  }

  async patchGameSettings(settingsResource: any) {
    return await makeRequest('PATCH', '/lol-game-settings/v1/game-settings', settingsResource);
  }

  async saveGameSettings() {
    return await makeRequest('POST', '/lol-game-settings/v1/save');
  }

  async disableWindowMode() {
    const settingsResource = {
      General: {
        WindowMode: 0,
      },
    };
    const updatedSettings = await this.patchGameSettings(settingsResource);
    const saved = await this.saveGameSettings();
    if (!saved) {
      throw new CustomError(
        'Failed to disable windowed mode automatically. Please manually disable it in the League Client settings before attempting to record a clip.',
      );
    }
    return updatedSettings;
  }

  async enableWindowMode() {
    const settingsResource = {
      General: {
        WindowMode: 1,
      },
    };
    const updatedSettings = await this.patchGameSettings(settingsResource);
    const saved = await this.saveGameSettings();
    if (!saved) {
      throw new CustomError(
        'Failed to enable windowed mode automatically. Please manually enable it in the League Client settings before attempting to record a clip.',
      );
    }
    return updatedSettings;
  }

  /// REPLAY REQUESTS ///
  async getReplayConfig() {
    return await makeRequest('GET', '/lol-replays/v1/configuration');
  }

  async getReplayMetaData(matchId: string) {
    return await makeRequest('GET', `/lol-replays/v1/metadata/${matchId}`);
  }

  async getRoflsPath() {
    return await makeRequest('GET', `/lol-replays/v1/rofls/path`);
  }

  async downloadReplay(matchId: any) {
    await makeRequest('POST', `/lol-replays/v1/rofls/${matchId}/download`, {}, 10);
    return await this.waitForReplayDownloadToComplete(matchId);
  }

  async waitForReplayDownloadToComplete(matchId: string) {
    const validDownloadStates = ['checking', 'downloading', 'watch'];
    const completed = 'watch';
    do {
      const metaData = await this.getReplayMetaData(matchId);
      var downloadState = metaData.state;
      if (!validDownloadStates.includes(downloadState)) {
        throw new CustomError(
          `Failed to download replay for matchId : ${matchId}. Download state : ${downloadState}. The riot replay service may be down, please try again later.`,
        );
      }
    } while (downloadState !== completed);
  }

  async launchReplay(matchId: string) {
    await this.downloadReplay(matchId);
    await makeRequest('POST', `/lol-replays/v1/rofls/${matchId}/watch`, {}, 10);
  }

  /// MATCH REQUESTS ///
  async getEndOfMatchDataByMatchId(matchId: any) {
    return await makeRequest('GET', `/lol-match-history/v1/games/${matchId}`);
  }

  async getSummonersByRiotId(riotId: string) {
    return makeRequest('GET', `/lol-summoner/v1/summoners?name=${riotId}`);
  }

  async getMatchHistoryByPuuid(puuid: any, begIndex: any, endIndex: any) {
    const matchData = await makeRequest(
      'GET',
      `/lol-match-history/v1/products/lol/${puuid}/matches?begIndex=${begIndex}&endIndex=${endIndex}`,
    );
    const matchHistory = await matchData.games.games;
    return matchHistory;
  }

  async getMatchTimelineByMatchId(matchId: any) {
    const matchData = await makeRequest('GET', `/lol-match-history/v1/game-timelines/${matchId}`);
    const matchTimeline = await matchData.frames;
    return matchTimeline;
  }

  async getPatchVersion() {
    if (this.patch.length > 0) {
      return this.patch;
    }
    const rawPatchData = await makeRequest('GET', '/lol-patch/v1/game-version');
    return rawPatchData;
  }

  async getState({ options = { retry: 0 } }: { options?: { retry: number } }): Promise<{ action: 'Idle' }> {
    return await makeRequest('GET', '/lol-patch/v1/products/league_of_legends/state', null, options.retry);
  }

  async getQueues() {
    return makeRequest('GET', 'lol-game-queues/v1/queues');
  }

  async getCurrentSummoner(): Promise<Summoner> {
    const currentSummoner = await makeRequest('GET', '/lol-summoner/v1/current-summoner');
    return new Summoner(
      currentSummoner.displayName,
      currentSummoner.tagLine,
      currentSummoner.puuid,
    );
  }

  async getLockfilePath(): Promise<string> {
    const localAppData = process.env.LOCALAPPDATA;
    return path.join(localAppData, 'Riot Games', 'League of Legends', 'lockfile');
  }

  async removeLockfile() {
    try {
      const lockfilePath = await this.getLockfilePath();
      await promisify(fs.unlink)(lockfilePath);
    } catch (e) {
      // ignore
    }
  }
}
