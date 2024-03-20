import Replay from './apis/replay';
import { sleepInSeconds } from '../../utils/utils';
import LeagueClient from './apis/league-client';
import * as fs from 'fs';
import path from 'node:path';

export class RecorderService {
  async cleanUp() {
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

  async record(params: {
    gameId: number;
    summonerName?: string;
    startTime?: number;
    endTime?: number;
    cameraMode?: 'centerScreen' | 'auto';
  }) {
    params.startTime = params.startTime || 0;
    params.endTime = params.endTime || Number.MIN_VALUE;
    params.cameraMode = params.cameraMode || 'auto';

    await this.cleanUp();

    const replay = new Replay();
    try {
      await LeagueClient.launchReplay(params.gameId);
      await replay.load(10, 10); // Add global vars
      await replay.init();
      await replay.waitForAssetsToLoad();
      await LeagueClient.enableWindowMode();

      if (params.cameraMode === 'centerScreen') {
        await this.setCenterScreen(replay);
      }
      await replay.postRenderProperties({
        selectionName: params.summonerName,
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
      await replay.waitForRecording();
      await replay.postRecordingProperties({
        recording: false,
      });
      await sleepInSeconds(5);
      await replay.exit();
      await sleepInSeconds(10);
      return recordingProperties;
    } finally {
      await replay.exit();
    }
    // return metadata
  }

  private async setCenterScreen(replay: Replay) {
    await replay.postRenderProperties({
      interfaceTimeline: false,
      cameraAttached: true, // cameraAttatched setting only works when cameraMode=fps
      cameraMode: 'fps',
      interfaceScoreboard: true,
      selectionOffset: {
        x: 0.0,
        y: 1911.85,
        z: -1350.0,
      },
    });
  }
}
