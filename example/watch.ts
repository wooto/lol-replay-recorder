import { exit } from 'node:process';
import { RiotGameClient } from '../src';

await new RiotGameClient().login('wootokim', 'wlghwlgh#6');
exit(0);
