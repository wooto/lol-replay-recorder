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

  await new LeagueClientUx().startClient({ region, locale: locale });
  console.log('Client started');
  await new RiotGameClient().login(username, password);
  console.log('Logged in');
  await new RiotGameClient().getInstalls();
  await new LeagueClientUx().getState();
  console.log('Client is running');
  await sleepInSeconds(5);
  console.log('Waiting for 5 seconds');
}
exit(0);
