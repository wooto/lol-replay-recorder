import CustomError from '../model/CustomError';
import { makeRequest } from '../model/RiotRequest';
import _ from 'lodash';
import { sleepInSeconds } from '../utils/utils';
import { ReplayType } from '../model/ReplayType';
import { LeagueClientExecution } from './LeagueClientExecution';
import { Key, keyboard } from '@kirillvakalov/nut-tree__nut-js';

process.env.NODE_TLS_REJECT_UNAUTHORIZED = '0';

export class ReplayClient {
  url: string;

  pid: any;

  constructor() {
    this.url = 'https://127.0.0.1:2999';
    this.pid = null;
  }

  async init() {
    try {
      const pid = await this.getProcessId();
      this.setProcessId(pid);
    } catch (error) {
      console.error(error);
    }
  }

  setProcessId(pid: any) {
    this.pid = pid;
  }

  async getProcessId() {
    if (this.pid) {
      return this.pid;
    }
    const replayData = await makeRequest('GET', `${this.url}/replay/game`);
    if (replayData?.processID) {
      this.pid = replayData.processID;
    }
    return this.pid;
  }

  async exit() {
    try {
      const pid = await this.getProcessId();
      process.kill(pid);
    } catch (err) {
      // shhhh...
    }
  }

  async getPlaybackProperties() {
    return await makeRequest('GET', `${this.url}/replay/playback`);
  }

  async postPlaybackProperties(options: any) {
    return await makeRequest('POST', `${this.url}/replay/playback`, {}, options);
  }

  public async getRecordingProperties(): Promise<ReplayType.RecordingProperties> {
    return await makeRequest('GET', `${this.url}/replay/recording`);
  }

  public async postRecordingProperties(options: any) {
    return await makeRequest('POST', `${this.url}/replay/recording`, {}, options);
  }

  public async getRenderProperties(): Promise<ReplayType.RenderProperties> {
    return await makeRequest('GET', `${this.url}/replay/render`);
  }

  public async postRenderProperties(options: any) {
    return await makeRequest('POST', `${this.url}/replay/render`, {}, options);
  }

  public async load(timeout: number, numRetries: number) {
    let responseReceived = false;
    do {
      try {
        await this.getPlaybackProperties();
        await this.getRecordingProperties();
        responseReceived = true;
      } catch (err) {
        numRetries--;
        // EventService.publish('clipProgress', `Loading replay...`);
        console.log(
          `Couldnt connect to replay API, waiting ${timeout} seconds then retrying (${numRetries} retries remaining).`,
        );
        await sleepInSeconds(timeout);
      }
    } while (!responseReceived && numRetries > 0);
    if (numRetries <= 0) {
      throw new CustomError(
        'Failed to launch replay. Please ensure the replay API is enabled and the client is running, then try again',
      );
    }
    await this.waitForAssetsToLoad();
  }

  public async waitForAssetsToLoad() {
    let playbackState;
    let paused;
    let time;
    do {
      playbackState = await this.getPlaybackProperties();
      time = playbackState.time;
      paused = playbackState.paused;
    } while (time < 15 && !paused);
  }

  public async waitForRecordingToFinish(time: number) {
    let waitTime = time;
    let recording;
    do {
      await sleepInSeconds(waitTime);
      const recordingState = await this.getRecordingProperties();
      recording = recordingState.recording;
      waitTime = recordingState.endTime - recordingState.currentTime;
    } while (recording && waitTime > 0);
  }

  async getAllGameData(): Promise<ReplayType.GameData> {
    return await makeRequest('GET', `${this.url}/liveclientdata/allgamedata`);
  }

  async getInGamePositionBySummonerName(summonerName: string): Promise<number> {
    const data = await this.getAllGameData();
    const orderTeam = data.allPlayers.filter(it => {
      return it.team === 'ORDER';
    });
    const chaosTeam = data.allPlayers.filter(it => {
      return it.team === 'CHAOS';
    });

    const orderIndex = _.findIndex(orderTeam, it => {
      return it.summonerName === summonerName;
    });

    if (orderIndex !== -1) {
      return orderIndex;
    }

    const chaosIndex = _.findIndex(chaosTeam, it => {
      return it.summonerName === summonerName;
    });

    if (chaosIndex !== -1) {
      return chaosIndex + 5;
    }

    throw new CustomError('Summoner not found in game');
  }

  async focusBySummonerName(targetSummonerName: string) {
    const position = await this.getInGamePositionBySummonerName(targetSummonerName);
    const keyboardKey = [
      ...[Key.Num1, Key.Num2, Key.Num3, Key.Num4, Key.Num5],
      ...[Key.Q, Key.W, Key.E, Key.R, Key.T],
    ][position];

    const execution = new LeagueClientExecution();
    for (let i = 0; i < 10; i++) {
      await execution.focusClientWindow();
      for (let j = 0; j < 50; j++) {
        await keyboard.type(keyboardKey);
        await sleepInSeconds(0.2);
      }
      await sleepInSeconds(10);
      const { selectionName } = await this.getRenderProperties();
      if (selectionName === targetSummonerName) {
        break;
      }
    }
  }
}
