import { exit } from 'node:process';
import { LeagueClientExecution, LeagueClientUx, ReplayClient } from '../src';
import { Locale } from '../src/model/Locale';
const sleepInSeconds = (seconds: number) =>
  new Promise(resolve => setTimeout(resolve, seconds * 1000));

await new LeagueClientUx().launchReplay('7070752623');
// await sleepInSeconds(20);

await new ReplayClient().focusBySummonerName('DRX Teddy');
await sleepInSeconds(3);
const replay = new ReplayClient();
  await replay.postPlaybackProperties({
    time: 40,
    paused: true,
    seeking: false,
    speed: 1.0,
  });

//   await replay.focusBySummonerName(params.summonerName);
//   await sleepInSeconds(3);
await sleepInSeconds(3);

await replay.postRecordingProperties({
  recording: true,
  // endTime: 100,
});
await new ReplayClient().focusBySummonerName('DRX Teddy');
