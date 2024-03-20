#!/usr/bin/env node

import { RecorderService } from './usecases/recorder/RecorderService';

const main = async () => {
  const recorder = new RecorderService();
  await recorder.record({ gameId: 6994393466, summonerName: 'test', startTime: 30, endTime: 60 });
};
main();
