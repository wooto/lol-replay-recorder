import { exit } from 'node:process';
import { LeagueClientExecution, ReplayClient } from '../src';
import { Locale } from '../src/model/Locale';

await new ReplayClient().focusBySummonerName('wooto');
