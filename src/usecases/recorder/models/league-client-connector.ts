// import { spliceString, isEmpty } from '../utils/utils';
// import { CustomError } from './custom-error';
// import find from 'find-process';

class LeagueClientConnector {
  credentials: {
    user: string;
    host: string;
    password: string;
    port: string;
  };
  processInformation: string;
  pid: string;

  constructor() {
    // @ts-ignore
    this.credentials = {};
    this.processInformation = '';
    this.pid = '';
  }

  async getClientCredentials(): Promise<{
    user: string;
    host: string;
    password: string;
    port: string;
  }> {
    // if(!isEmpty(this.credentials) && this.isClientRunning()) {
    //     return this.credentials;
    // }
    // const processInformation = await this.getClientProcessInformation();
    // this.setClientProcessInformation(processInformation);
    // return this.credentials;
    return {} as never;
  }

  async isClientRunning(): Promise<boolean> {
    // try {
    //     const pid = (this.pid.length > 0) ? this.pid : await this.getClientPid();
    //     let isRunning = process.kill(pid, 0);
    //     return isRunning;
    // } catch (error) {
    //     return false;
    // }
    return {} as never;
  }

  async getClientProcessInformation(): Promise<string> {
    // const response = await find('name', 'LeagueClientUx.exe');
    // if(Array.isArray(response) && response.length != 0){
    //     this.processInformation = response[0].cmd;
    //     return this.processInformation;
    // } else {
    //     throw new CustomError("Failed to find the league client process. Please make sure the client is running.");
    // }
    return {} as never;
  }

  async setClientProcessInformation(processInformation: string): Promise<void> {
    this.processInformation = processInformation;
    // const passwordFieldStartIndex = processInformation.search("--remoting-auth-token=") + ("--remoting-auth-token=".length);
    // const portFieldStartIndex = processInformation.search('--app-port=') + ('--app-port='.length);
    // this.credentials = {
    //     user        : 'riot',
    //     host        : 'https://127.0.0.1',
    //     password    : spliceString(processInformation, passwordFieldStartIndex, '"'),
    //     port        : spliceString(processInformation, portFieldStartIndex , '"')
    // };
    // this.pid = await this.getClientPid();
    return {} as never;
  }

  async getClientPid(): Promise<string> {
    // if(this.pid.length > 0) {
    //     return this.pid;
    // }
    // const processInformation = await this.getClientProcessInformation();
    // const pidStartIndex =  processInformation.search("--app-pid=") + "--app-pid=".length;
    // const pid = spliceString(processInformation, pidStartIndex, '"');
    // return pid;
    return {} as never;
  }
}

export default new LeagueClientConnector();

// const lcc = new LeagueClientConnector();
// lcc.getClientCredentials().then(res => console.log(res)).catch(err=>console.log(err));
