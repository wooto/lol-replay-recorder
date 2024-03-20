import { BaseConfigType } from './index';

const Config: BaseConfigType = {
  environment: 'test',
  redis: {
    username: 'default',
    password: undefined,
    host: 'localhost',
    port: 6379,
  },
};

export default Config;
