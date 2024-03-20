import { exec } from 'child_process';
import { expect } from 'chai';

describe('testJob', () => {
  it('should run without errors', done => {
    exec('node --loader ts-node/esm ./src/jobs/testJob.ts', error => {
      expect(error).to.be.eq(null);
      done();
    });
  });
});
