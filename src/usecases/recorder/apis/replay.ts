import CustomError from '../models/custom-error';
import { makeRequest } from '../models/replay-request';
import { sleepInSeconds } from '../../../utils/utils';

process.env.NODE_TLS_REJECT_UNAUTHORIZED = '0';

class Replay {
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

  async getRecordingProperties(): Promise<{
    endTime: number;
    currentTime: number;
    recording: boolean;
    path: string;
  }> {
    return await makeRequest('GET', `${this.url}/replay/recording`);
  }

  async postRecordingProperties(options: any) {
    return await makeRequest('POST', `${this.url}/replay/recording`, {}, options);
  }

  async getRenderProperties() {
    return await makeRequest('GET', `${this.url}/replay/render`);
  }

  async postRenderProperties(options: any) {
    return await makeRequest('POST', `${this.url}/replay/render`, {}, options);
  }

  async load(timeout: number, numRetries: number) {
    let responseReceived = false;
    do {
      try {
        await this.getPlaybackProperties();
        await this.getRecordingProperties();
        responseReceived = true;
      } catch (err) {
        console.log(err);
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
    // EventService.publish('clipProgress', `Replay loaded succcessfully...`);
  }

  async waitForAssetsToLoad() {
    let playbackState;
    let paused;
    let time;
    do {
      playbackState = await this.getPlaybackProperties();
      time = playbackState.time;
      paused = playbackState.paused;
    } while (time < 15 && !paused);
  }

  async waitForRecording() {
    let recording = false;
    let waitTime = 0;

    do {
      const recordingState = await this.getRecordingProperties();
      const playback = await this.getPlaybackProperties();
      recording = recordingState.recording;
      waitTime = playback.length - recordingState.currentTime - 1;
      // await sleepInSeconds(waitTime);
      console.log('recordingState:', playback);
      await sleepInSeconds(1);
      // if(recordingState.currentTime > 10) {
      //   break;
      // }
    } while (recording && waitTime > 0);
  }

  async waitForRecordingToFinish(time: number) {
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

export default Replay;
