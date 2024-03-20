import 'reflect-metadata';
import { expect } from 'chai';
import { Container } from 'typedi';
import { describe } from 'mocha';
import { PostgresqlClient } from '../../../../src/infrastructure/config/PostgresqlClient';

describe('PostgresqlClient', () => {
  it('should be able to connect to postgres', async () => {
    const postgresqlClient = Container.get(PostgresqlClient);
    const result = await postgresqlClient.healthCheck();
    expect(result.rows[0].result).to.equal(2);
  });
});
