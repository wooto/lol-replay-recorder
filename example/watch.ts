import { exit } from 'node:process';
import { LeagueClientExecution, RiotGameClient } from '../src';
import { getActiveWindow, getWindows, mouse, keyboard, Key } from '@kirillvakalov/nut-tree__nut-js';
import { Locale } from '../src/model/Locale';



const username = process.env.RIOT_USERNAME;
const password = process.env.RIOT_PASSWORD;
console.log('Hello, World!');
console.log('username:', username);
console.log('password:', password);

const locale_list = Object.values(Locale);
for (const locale of locale_list) {
  console.log('start locale:', locale);
  await new LeagueClientExecution().startRiotProcessesSafely({
    region: 'na1' as any,
    locale,
    username,
    password,
  });
}
exit(0);
