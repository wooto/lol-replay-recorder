import Replay from './apis/replay';
import { sleepInSeconds } from '../../utils/utils';
import LeagueClient from './apis/league-client';
import * as fs from 'fs';
import path from 'node:path';

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

  public async record(params: {
    gameId: number;
    summonerName?: string;
    startTime?: number;
    endTime?: number;
    cameraMode?: 'centerScreen' | 'auto';
    interfaceTimeline?: boolean;
    interfaceScoreboard?: boolean;
  }) {
    params.startTime = params.startTime || 0;
    params.cameraMode = params.cameraMode || 'auto';
    console.log(await LeagueClient.getPatchVersion());

    const replay = new Replay();
    try {
      await LeagueClient.launchReplay(params.gameId);
      await replay.load(10, 10); // Add global vars
      await replay.init();
      await replay.waitForAssetsToLoad();
      await LeagueClient.enableWindowMode();
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
        ...(params.interfaceTimeline && { interfaceTimeline: true }),
        ...(params.interfaceScoreboard && { interfaceScoreboard: true }),
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
    // return metadata
  }
}
