import { Container } from 'typedi';
import { PostgresqlClient } from '../src/infrastructure/config/PostgresqlClient';
import fs from 'fs';

export const createAllTables = async () => {
  const knex = Container.get(PostgresqlClient).getKnex();
  const sql = fs.readFileSync('init.sql').toString();
  await knex.schema.raw(sql);
};
export const dropAllTables = async () => {
  const knex = Container.get(PostgresqlClient).getKnex();
  // get all tables
  const tables = await knex('pg_tables').where('schemaname', 'public').select('tablename');
  for (const table of tables) {
    await knex.schema.dropTableIfExists(table.tablename);
  }
};
