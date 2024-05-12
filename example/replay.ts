import { exit } from 'node:process';
import { LeagueClientExecution, LeagueClientUx, ReplayClient } from '../src';
import { Locale } from '../src/model/Locale';

// await new LeagueClientUx().launchReplay('6935070152');
await new ReplayClient().focusBySummonerName('Pawko');
