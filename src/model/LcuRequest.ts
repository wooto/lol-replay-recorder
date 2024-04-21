import { authenticate, createHttp1Request } from 'league-connect';
import CustomError from './CustomError';
import Bottleneck from 'bottleneck';

const MAX_REQUESTS_PER_SECOND = 10;

const limiter = new Bottleneck({
  minTime: 1000 / MAX_REQUESTS_PER_SECOND,
});

async function makeRequest(method: any, url: any, body: any = {}, retries: any = 3): Promise<any> {
  try {
    const response = await limiter.schedule(async () => {
      const credentials = await authenticate();
      console.log(`Making request to ${url} ${retries}`);
      return createHttp1Request(
        {
          method,
          url,
          body,
        },
        credentials,
      );
    });

    if (!response.ok) {
      await parseResponseForErrors(response, retries - 1);
      console.log(`retrying. retries = ${retries - 1}`);
      return makeRequest(method, url, body, retries - 1);
    }
    try {
      return await response.json();
    } catch (err) {
      return response;
    }
  } catch (e) {
    if (retries <= 0) {
      throw new Error(`Client Request Error: ${e.message}`);
    }
    return makeRequest(method, url, body, retries - 1);
  }
}

async function parseResponseForErrors(response: any, retries: any) {
  if (response.status === 404) {
    throw new CustomError('Failed to find the requested resource.');
  }

  if (retries <= 0) {
    throw new Error(
      `Client Request Error: ${response.status} ${response.statusText} - ${await response.text()}`,
    );
  }
}

export { makeRequest };
