import { exit } from 'node:process';
import { LeagueClientExecution, LeagueClientUx, RiotGameClient } from '../src';
import { Locale } from '../src/model/Locale';
import { sleepInSeconds } from '../src/utils/utils';


const username = process.env.RIOT_USERNAME;
const password = process.env.RIOT_PASSWORD;
console.log('Hello, World!');

const locale_list = Object.values(Locale);
for (const locale of locale_list) {
  console.log('Starting client with locale:', locale)
  const region = 'na1';
  await new LeagueClientExecution().stopRiotProcesses();
  console.log('Stopped Riot processes')
  await new RiotGameClient().startRiotClient(region as any, locale);
  await new LeagueClientExecution().stopRiotProcesses();
  console.log('Stopped Riot processes')

  for(let i = 0; i < 5; i++) {
    try{
      await new RiotGameClient().startRiotClient(region as any, locale);
      await new RiotGameClient().login(username, password);
      console.log('Started client');
      await new LeagueClientUx().startClient({ region, locale: locale });
      console.log('Waiting for client to be ready');
      await new LeagueClientUx().getState({ options: { retry: 10 } });
      break;
    } catch (e) {
      console.log('Failed to start client:', e);
      await sleepInSeconds(1);
    }
  }
  console.log('Client is running');
  await sleepInSeconds(5);
  console.log('Waiting for 5 seconds');
}
exit(0);
