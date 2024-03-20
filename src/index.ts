#!/usr/bin/env node

import { RecorderService } from './usecases/recorder/RecorderService';

const main = async () => {
  const recorder = new RecorderService();
  await recorder.record({ gameId: 1, summonerName: 'test' });
};
main();
