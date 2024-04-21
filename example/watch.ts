import { exit } from 'node:process';
import { LeagueClientExecution } from '../src';
import { Locale } from '../src/model/Locale';


const username = process.env.RIOT_USERNAME;
const password = process.env.RIOT_PASSWORD;
console.log('Hello, World!');

const locale_list = Object.values(Locale);
for (const locale of locale_list) {
  new LeagueClientExecution().startRiotProcessesSafely({
    region: 'na1' as any,
    locale,
    username,
    password,
  });
}
exit(0);
