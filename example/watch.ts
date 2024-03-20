import { LolRecorderService } from '../src';
// import { LeagueClient } from 'league-connect';


console.log('Recording...');
await new LolRecorderService().record({
  gameId: 6994800465,
  startTime: 30,
  endTime: 40,
  summonerName: 'PlayerName',
  cameraMode: 'auto',
});
