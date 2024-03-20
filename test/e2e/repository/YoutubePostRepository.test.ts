import 'reflect-metadata';

import { YoutubePostRepository } from '../../../src/infrastructure/YoutubePostRepository';
import { Container } from 'typedi';
import { expect } from 'chai';
import { createAllTables, dropAllTables } from '../../knex';

describe('YoutubePostRepository', () => {
  beforeEach(async () => {
    await dropAllTables();
    await createAllTables();
  });

  it('should be able to create a post', async () => {
    const youtubePostRepository = Container.get(YoutubePostRepository);
    const result = await youtubePostRepository.create({
      channel_id: 'channel',
      title: 'title',
      created_at: new Date(),
    });
    expect(result.title).to.equal('title');
  });

  it('should be able to count by channel', async () => {
    const youtubePostRepository = Container.get(YoutubePostRepository);
    await youtubePostRepository.create({
      channel_id: 'channel',
      title: 'title',
      created_at: new Date(),
    });
    const result = await youtubePostRepository.countByChannelId({
      created_at_gte: new Date(new Date().getTime() - 1000 * 60 * 60 * 24 * 7),
    });
    console.log(result);
    expect(result[0].count).to.equal('1');
  });
});
