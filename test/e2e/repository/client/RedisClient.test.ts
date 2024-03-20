import 'reflect-metadata';
import { RedisClient } from '../../../../src/infrastructure/config/RedisClient';
import { expect } from 'chai';
import { Container } from 'typedi';
import { describe } from 'mocha';

describe('RedisClient', () => {
  it('should set value', async () => {
    const redisClient = Container.get(RedisClient);
    const result = await redisClient.set('test', 'test', 'EX', 60);
    expect(result).to.be.equal('OK');
  });
  //
  it('should get value', async () => {
    const redisClient = Container.get(RedisClient);
    const result = await redisClient.get('test');
    expect(result).to.be.equal('test');
  });
});
