#!/usr/bin/env node

import { RecorderService } from './usecases/recorder/RecorderService';

const main = async () => {
  const recorder = new RecorderService();
  await recorder.record({
    gameId: 6994396521,
    summonerName: 'test',
    startTime: 30,
    interfaceScoreboard: true,
    interfaceTimeline: false,
  });
  await recorder.cleanUp();
};
main();
