import Replay from './apis/replay';
import { sleepInSeconds } from '../../utils/utils';
import LeagueClient from './apis/league-client';
import { Service } from 'typedi';

@Service()
export class RecorderService {
  async cleanUp() {
    const replay = new Replay();
    await replay.exit();
  }

  async launch() {
    const replay = new Replay();
    await replay.exit();
    await sleepInSeconds(5);
    await replay.exit();
  }

  async record(metadata: { gameId: number; summonerName: string }) {
    await LeagueClient.getHighlightsFolderPath();
    const replay = new Replay();
    await replay.exit();
    await sleepInSeconds(5);
    try {
      await LeagueClient.launchReplay(metadata.gameId);
      await replay.load(10, 10); // Add global vars
      await replay.init();
      await replay.waitForAssetsToLoad();
      await LeagueClient.enableWindowMode();
      await replay.postRenderProperties({
        interfaceTimeline: false,
        cameraAttached: true, // cameraAttatched setting only works when cameraMode=fps
        selectionName: metadata.summonerName,
        cameraMode: 'fps',
        interfaceScoreboard: true,
        selectionOffset: {
          x: 0.0,
          y: 1911.85,
          z: -1350.0,
        },
      });

      await replay.postPlaybackProperties({
        time: 30,
        paused: false,
        seeking: false,
        speed: 1.0,
        length: 0,
      });
      await sleepInSeconds(2);

      await replay.postRecordingProperties({
        startTime: 30,
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
}
