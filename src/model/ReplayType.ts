export namespace ReplayType {
  export type GameData = {
    activePlayer: ActivePlayer;
    allPlayers: Player[];
    events: Events;
    gameData: GameDetails;
  };

  type ActivePlayer = {
    error: string;
  };

  type Player = {
    championName: string;
    isBot: boolean;
    isDead: boolean;
    items: Item[];
    level: number;
    position: string;
    rawChampionName: string;
    rawSkinName: string;
    respawnTimer: number;
    runes: Runes;
    scores: Scores;
    screenPositionBottom: string;
    screenPositionCenter: string;
    skinID: number;
    skinName: string;
    summonerName: string;
    summonerSpells: SummonerSpells;
    team: string;
  };

  type Item = {
    canUse: boolean;
    consumable: boolean;
    count: number;
    displayName: string;
    itemID: number;
    price: number;
    rawDescription: string;
    rawDisplayName: string;
    slot: number;
  };

  type Runes = {
    keystone: Keystone;
    primaryRuneTree: RuneTree;
    secondaryRuneTree: RuneTree;
  };

  type Keystone = {
    displayName: string;
    id: number;
    rawDescription: string;
    rawDisplayName: string;
  };

  type RuneTree = {
    displayName: string;
    id: number;
    rawDescription: string;
    rawDisplayName: string;
  };

  type Scores = {
    assists: number;
    creepScore: number;
    deaths: number;
    kills: number;
    wardScore: number;
  };

  type SummonerSpells = {
    summonerSpellOne: SummonerSpell;
    summonerSpellTwo: SummonerSpell;
  };

  type SummonerSpell = {
    displayName: string;
    rawDescription: string;
    rawDisplayName: string;
  };

  type Events = {
    Events: Event[];
  };

  type Event = {
    EventID: number;
    EventName: string;
    EventTime: number;
    Assisters?: string[];
    KillerName?: string;
    VictimName?: string;
    KillStreak?: number;
  };

  type GameDetails = {
    gameMode: string;
    gameTime: number;
    mapName: string;
    mapNumber: number;
    mapTerrain: string;
  };

  export type RecordingProperties = {
    codec: string;
    currentTime: number;
    endTime: number;
    enforceFrameRate: boolean;
    framesPerSecond: number;
    height: number;
    lossless: boolean;
    path: string;
    recording: boolean;
    replaySpeed: number;
    startTime: number;
    width: number;
  }

}
