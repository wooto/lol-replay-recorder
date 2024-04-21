import Bottleneck from 'bottleneck';
import https from 'https';
import CustomError from './CustomError';
import { sleepInSeconds } from '../utils/utils';

const MAX_REQUESTS_PER_SECOND = 1;

const limiter = new Bottleneck({
  minTime: 1000 / MAX_REQUESTS_PER_SECOND,
});

/* The replay API is different than the LCU API, so we can't use the League-Connect
package for making replay requests like we do for requests in league-client.js */
async function makeRequest(
  method: any,
  url: any,
  headers: any = null,
  body: any = null,
  retries: any = 3,
): Promise<any> {
  const newHeaders: any = { ...headers, 'Content-Type': 'application/json' };
  try {
    const response = await limiter.schedule(() => {
      console.log(`Making request to ${url} ${retries}`);
      return fetch(url, new RequestOptions(method, newHeaders, body));
    });

    if (!response.ok) {
      await parseResponseForErrors(response, retries - 1);
      return await makeRequest(method, url, headers, body, retries - 1);
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
    return await makeRequest(method, url, headers, body, retries - 1);
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

class RequestOptions {
  body: any;

  method: any;

  agent: any;

  headers: any;

  constructor(method: any, headers: any, body: any) {
    this.method = method; // GET or POST
    this.headers = headers;
    this.agent = new https.Agent({
      rejectUnauthorized: false,
      requestCert: true,
    });
    this.body = method.toLowerCase() === 'get' ? undefined : JSON.stringify(body);
  }
}

export { makeRequest };
