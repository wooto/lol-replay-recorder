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

  type Vector3 = {
    x: number;
    y: number;
    z: number;
  }

  type Color = {
    r: number;
    g: number;
    b: number;
    a: number;
  }

  type CameraSettings = {
    cameraAttached: boolean;
    cameraLookSpeed: number;
    cameraMode: string;
    cameraMoveSpeed: number;
    cameraPosition: Vector3;
    cameraRotation: Vector3;
  }

  type FogSettings = {
    depthFogColor: Color;
    depthFogEnabled: boolean;
    depthFogEnd: number;
    depthFogIntensity: number;
    depthFogStart: number;
    heightFogColor: Color;
    heightFogEnabled: boolean;
    heightFogEnd: number;
    heightFogIntensity: number;
    heightFogStart: number;
  }

  type DepthOfFieldSettings = {
    depthOfFieldCircle: number;
    depthOfFieldDebug: boolean;
    depthOfFieldEnabled: boolean;
    depthOfFieldFar: number;
    depthOfFieldMid: number;
    depthOfFieldNear: number;
    depthOfFieldWidth: number;
  }

  type InterfaceSettings = {
    interfaceAll: boolean;
    interfaceAnnounce: boolean;
    interfaceChat: boolean;
    interfaceFrames: boolean;
    interfaceKillCallouts: boolean;
    interfaceMinimap: boolean;
    interfaceNeutralTimers: boolean;
    interfaceQuests: null | boolean; // Assuming it can be null or a boolean
    interfaceReplay: boolean;
    interfaceScore: boolean;
    interfaceScoreboard: boolean;
    interfaceTarget: boolean;
    interfaceTimeline: boolean;
  }

  type RenderSettings = {
    banners: boolean;
    characters: boolean;
    environment: boolean;
    farClip: number;
    fieldOfView: number;
    floatingText: true;
    fogOfWar: boolean;
    healthBarChampions: boolean;
    healthBarMinions: boolean;
    healthBarPets: boolean;
    healthBarStructures: boolean;
    healthBarWards: boolean;
    navGridOffset: number;
    nearClip: number;
    outlineHover: boolean;
    outlineSelect: boolean;
    particles: boolean;
    selectionName: string;
    selectionOffset: Vector3;
    skyboxOffset: number;
    skyboxPath: string;
    skyboxRadius: number;
    skyboxRotation: number;
    sunDirection: Vector3;
  }

  export type RenderProperties = {
    cameraSettings: CameraSettings;
    fogSettings: FogSettings;
    depthOfFieldSettings: DepthOfFieldSettings;
    interfaceSettings: InterfaceSettings;
    renderSettings: RenderSettings;
  }


}
