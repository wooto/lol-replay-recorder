import { exit } from 'node:process';
import { LeagueClientExecution } from '../src';
import { Locale } from '../src/model/Locale';


const username = process.env.RIOT_USERNAME;
const password = process.env.RIOT_PASSWORD;
console.log('Hello, World!');
console.log('username:', username);
console.log('password:', password);

// const locale_list = Object.values(Locale);
// for (const locale of locale_list) {
//   await new LeagueClientExecution().startRiotProcessesSafely({
//     region: 'na1' as any,
//     locale,
//     username,
//     password,
//   });
// }

await new LeagueClientExecution().startRiotProcessesSafely({
  region: 'kr' as any,
  locale: Locale.ko_KR,
  username,
  password,
});
exit(0);
