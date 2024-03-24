import { makeRequest } from '../models/lcu-requests';
import Summoner from '../models/summoner';
import CustomError from '../models/custom-error';

export class LeagueClientUx {
  patch: string;

  constructor() {
    this.patch = '';
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
    await makeRequest('POST', `/lol-replays/v1/rofls/${matchId}/download`);
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

  async launchReplay(matchId: any) {
    await this.downloadReplay(matchId);
    await makeRequest('POST', `/lol-replays/v1/rofls/${matchId}/watch`);
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
}

export default new LeagueClientUx();
