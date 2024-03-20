import _ from 'lodash';
import { Default } from './default';

type DeepPartial<T> = {
  [P in keyof T]?: T[P] extends (infer U)[]
    ? DeepPartial<U>[]
    : // tslint:disable-next-line:no-shadowed-variable
      T[P] extends readonly (infer U)[]
      ? readonly DeepPartial<U>[]
      : DeepPartial<T[P]>;
};

export type BaseConfigType = DeepPartial<typeof Default>;

const Config = _.cloneDeep(Default);

if (process.env.NODE_ENV) {
  try {
    const EnvConfig = (await import(`./${process.env.NODE_ENV}`)).default;
    _.merge(Config, EnvConfig);
  } catch (e) {
    console.log(`Cannot find configs for env=${process.env.NODE_ENV}`);
  }
}

console.log(`Config loaded for env=${Config.environment}`);

export { Config };
