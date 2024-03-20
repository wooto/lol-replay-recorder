#!/usr/bin/env node
import 'reflect-metadata';

import { AutoRecorderApplication } from './application/AutoRecorderApplication';
import { sleep } from './utils/utils';
import { Container } from 'typedi';

const main = async () => {
  const serviceInstance = Container.get(AutoRecorderApplication);

  // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition,no-constant-condition
  while (true) {
    try {
      await serviceInstance.record();
    } catch (e) {
      console.error(e);
    }
    await sleep(60 * 1000);
  }
};
main();
