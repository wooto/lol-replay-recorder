import * as tls from 'tls';

export const Default = {
  environment: 'default',
  redis: {
    username: 'default',
    password: undefined as string | undefined,
    host: 'localhost',
    port: 6379,
    tls: undefined as tls.ConnectionOptions,
  },
  postgres: {
    host: 'localhost',
    port: 5432,
    username: 'wooto',
    password: 'wooto',
    database: 'lol-replays',
    ssl: false,
  },
};
