import Replay from './apis/replay';
import { sleepInSeconds } from '../../utils/utils';
import * as fs from 'fs';
import path from 'node:path';
import LeagueClient from './apis/league-client';

export type RecordParams = {
  gameId: number;
  summonerName?: string;
  startTime?: number;
  endTime?: number;
  cameraMode?: 'centerScreen' | 'auto';
  windowMode?: boolean;
  interfaceTimeline?: boolean;
  interfaceScoreboard?: boolean;
}

export type RecordResult = {
  endTime: number;
  currentTime: number;
  recording: boolean;
  path: string;
};

export class LolRecorderService {
  public async cleanUp() {
    const recursiveDelete = (folderPath: string) => {
      fs.readdirSync(folderPath).forEach(file => {
        const filePath = path.join(folderPath, file);
        fs.unlinkSync(filePath);
      });
    };

    const highlightsFolderPath = await LeagueClient.getHighlightsFolderPath();
    recursiveDelete(highlightsFolderPath);

    const rotlsPath = await LeagueClient.getRoflsPath();
    recursiveDelete(rotlsPath);
  }

  public async record(params: RecordParams): Promise<RecordResult> {
    params.startTime = params.startTime || 0;
    params.cameraMode = params.cameraMode || 'auto';
    params.windowMode = params.windowMode || false;
    params.interfaceTimeline = params.interfaceTimeline || false;
    params.interfaceScoreboard = params.interfaceScoreboard || false;
    console.log(await LeagueClient.getPatchVersion());

    const replay = new Replay();
    try {
      await LeagueClient.launchReplay(params.gameId);
      await replay.load(10, 10); // Add global vars
      await replay.init();
      await replay.waitForAssetsToLoad();
      if (params.windowMode) {
        await LeagueClient.enableWindowMode();
      }
      params.endTime = params.endTime || (await replay.getPlaybackProperties()).length;

      await replay.postRenderProperties({
        ...(params.summonerName && { selectionName: params.summonerName }),
        ...(params.cameraMode === 'centerScreen' && {
          cameraAttached: true, // cameraAttatched setting only works when cameraMode=fps
          cameraMode: 'fps',
          selectionOffset: {
            x: 0.0,
            y: 1911.85,
            z: -1350.0,
          },
        }),
        interfaceTimeline: params.interfaceTimeline,
        interfaceScoreboard: params.interfaceScoreboard,
      });

      await replay.postPlaybackProperties({
        time: params.startTime,
        paused: false,
        seeking: false,
        speed: 1.0,
        length: 0,
      });
      await sleepInSeconds(2);

      await replay.postRecordingProperties({
        startTime: params.startTime,
        endTime: params.endTime,
        height: 1080,
        width: 1920,
        recording: true,
      });
      const recordingProperties = await replay.getRecordingProperties();
      await replay.waitForRecordingToFinish(params.endTime);
      await replay.postRecordingProperties({
        recording: false,
      });
      await replay.exit();
      return recordingProperties;
    } finally {
      await replay.exit();
    }

  }

  public async getHighlightsFolderPath() {
    return await LeagueClient.getHighlightsFolderPath();
  }
}
