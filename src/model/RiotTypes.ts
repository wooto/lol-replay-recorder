export namespace RiotTypes {
  export enum PlatformId {
    EUW1 = 'euw1',
    EUNE1 = 'eun1',
    NA1 = 'na1',
    LA1 = 'la1',
    LA2 = 'la2',
    KR = 'kr',
    JP1 = 'jp1',
    BR1 = 'br1',
    OC1 = 'oc1',
    RU = 'ru',
    TR1 = 'tr1',
    EUROPE = 'europe',
    ASIA = 'asia',
    SEA = 'sea',
    AMERICAS = 'americas',
    AP = 'ap',
    BR = 'br',
    EU = 'eu',
    NA = 'na',
    LATAM = 'latam',
    PH2 = 'ph2',
    SG2 = 'sg2',
    TH2 = 'th2',
    TW2 = 'tw2',
    VN2 = 'vn2',
    ESPORTS = 'esports',
    APAC = 'apac',
  }

  export type Cluster =
    | PlatformId.EUROPE
    | PlatformId.AMERICAS
    | PlatformId.ASIA
    | PlatformId.SEA
    | PlatformId.ESPORTS;

  export type Region =
    | PlatformId.BR1
    | PlatformId.EUNE1
    | PlatformId.EUW1
    | PlatformId.JP1
    | PlatformId.KR
    | PlatformId.LA1
    | PlatformId.LA2
    | PlatformId.NA1
    | PlatformId.OC1
    | PlatformId.RU
    | PlatformId.TR1
    | PlatformId.PH2
    | PlatformId.SG2
    | PlatformId.TH2
    | PlatformId.TW2
    | PlatformId.VN2;
}
