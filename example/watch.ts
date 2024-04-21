import { exit } from 'node:process';
import { LeagueClientUx } from '../src';
import { Locale } from '../src/model/Locale';


console.log('Hello, World!');
await new LeagueClientUx().startClient({ region: 'na1', locale: Locale.ko_KR });

exit(0);
