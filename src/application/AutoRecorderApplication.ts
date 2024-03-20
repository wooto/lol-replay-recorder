import { RecorderService } from '../usecases/recorder/RecorderService';
import { Service } from 'typedi';

@Service()
export class AutoRecorderApplication {
  constructor(public recorderService: RecorderService) {}

  async record(input: { summonerName: string; gameId: number }) {
    await this.recorderService.record({
      summonerName: input.summonerName,
      gameId: input.gameId,
    });
  }
}
