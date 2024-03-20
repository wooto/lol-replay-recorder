export type PlayerInfo = {
  game_id: string;
  summoner: Summoner;
};

type Summoner = {
  id: number;
  summoner_id: string;
  tagline: string;
  puuid: string;
  name: string;
  position: string;
  tier: string;
  game_name: string;
  internal_name: string;
  team_info: {
    nickname: string;
    team: {
      name: string;
    };
  };
};

export type SelectorData = {
  game_id: string;
};
