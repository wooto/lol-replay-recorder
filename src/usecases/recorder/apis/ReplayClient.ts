import CustomError from '../models/custom-error';
import { makeRequest } from '../models/replay-request';
import { sleepInSeconds } from '../../../utils/utils';

process.env.NODE_TLS_REJECT_UNAUTHORIZED = '0';

class ReplayClient {
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

  public async getRecordingProperties(): Promise<{
    endTime: number;
    currentTime: number;
    recording: boolean;
    path: string;
  }> {
    return await makeRequest('GET', `${this.url}/replay/recording`);
  }

  public async postRecordingProperties(options: any) {
    return await makeRequest('POST', `${this.url}/replay/recording`, {}, options);
  }

  public async getRenderProperties() {
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
}

export default ReplayClient;
