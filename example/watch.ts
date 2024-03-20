import { LolRecorderService } from '../src';


console.log('Recording...');
await new LolRecorderService().record({
  gameId: 6994800465,
  startTime: 30,
  endTime: 40,
  summonerName: 'PlayerName',
  cameraMode: 'auto',
});
