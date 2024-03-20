class Summoner {
  summonerName: string;

  tagline: string;

  riotId: string;

  puuid: string;

  constructor(summonerName: string, tagline: string, puuid: string = '') {
    this.summonerName = summonerName;
    this.tagline = tagline;
    this.riotId = `${encodeURIComponent(summonerName)}#${tagline}`;
    this.puuid = puuid;
  }
}

export default Summoner;
