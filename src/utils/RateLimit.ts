import 'reflect-metadata';

import Bottleneck from 'bottleneck';

export const bottleneckLimiter = new Bottleneck({
  maxConcurrent: 1, // 동시에 실행될 수 있는 작업의 최대 수
  minTime: 800,
});

export function RateLimit() {
  return function (target: any, propertyKey: string, descriptor: PropertyDescriptor) {
    const originalMethod = descriptor.value;

    descriptor.value = function (...args: any[]) {
      return bottleneckLimiter.schedule(() => {
        return originalMethod.apply(this, args);
      });
    };

    return descriptor;
  };
}
