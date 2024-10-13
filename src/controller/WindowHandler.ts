import { getActiveWindow as NutGetActiveWindow, getWindows, keyboard, mouse, Window as NutWindow } from '@kirillvakalov/nut-tree__nut-js';
import { sleep } from '../utils/utils';

export namespace WindowHandler {
  export enum Key {
    Escape = 0,
    F1 = 1,
    F2 = 2,
    F3 = 3,
    F4 = 4,
    F5 = 5,
    F6 = 6,
    F7 = 7,
    F8 = 8,
    F9 = 9,
    F10 = 10,
    F11 = 11,
    F12 = 12,
    F13 = 13,
    F14 = 14,
    F15 = 15,
    F16 = 16,
    F17 = 17,
    F18 = 18,
    F19 = 19,
    F20 = 20,
    F21 = 21,
    F22 = 22,
    F23 = 23,
    F24 = 24,
    Print = 25,
    ScrollLock = 26,
    Pause = 27,
    Grave = 28,
    Num1 = 29,
    Num2 = 30,
    Num3 = 31,
    Num4 = 32,
    Num5 = 33,
    Num6 = 34,
    Num7 = 35,
    Num8 = 36,
    Num9 = 37,
    Num0 = 38,
    Minus = 39,
    Equal = 40,
    Backspace = 41,
    Insert = 42,
    Home = 43,
    PageUp = 44,
    NumLock = 45,
    Divide = 46,
    Multiply = 47,
    Subtract = 48,
    Tab = 49,
    Q = 50,
    W = 51,
    E = 52,
    R = 53,
    T = 54,
    Y = 55,
    U = 56,
    I = 57,
    O = 58,
    P = 59,
    LeftBracket = 60,
    RightBracket = 61,
    Backslash = 62,
    Delete = 63,
    End = 64,
    PageDown = 65,
    NumPad7 = 66,
    NumPad8 = 67,
    NumPad9 = 68,
    Add = 69,
    CapsLock = 70,
    A = 71,
    S = 72,
    D = 73,
    F = 74,
    G = 75,
    H = 76,
    J = 77,
    K = 78,
    L = 79,
    Semicolon = 80,
    Quote = 81,
    Return = 82,
    NumPad4 = 83,
    NumPad5 = 84,
    NumPad6 = 85,
    LeftShift = 86,
    Z = 87,
    X = 88,
    C = 89,
    V = 90,
    B = 91,
    N = 92,
    M = 93,
    Comma = 94,
    Period = 95,
    Slash = 96,
    RightShift = 97,
    Up = 98,
    NumPad1 = 99,
    NumPad2 = 100,
    NumPad3 = 101,
    Enter = 102,
    LeftControl = 103,
    LeftSuper = 104,
    LeftWin = 105,
    LeftCmd = 106,
    LeftAlt = 107,
    Space = 108,
    RightAlt = 109,
    RightSuper = 110,
    RightWin = 111,
    RightCmd = 112,
    Menu = 113,
    RightControl = 114,
    Fn = 115,
    Left = 116,
    Down = 117,
    Right = 118,
    NumPad0 = 119,
    Decimal = 120,
    Clear = 121,
    AudioMute = 122,
    AudioVolDown = 123,
    AudioVolUp = 124,
    AudioPlay = 125,
    AudioStop = 126,
    AudioPause = 127,
    AudioPrev = 128,
    AudioNext = 129,
    AudioRewind = 130,
    AudioForward = 131,
    AudioRepeat = 132,
    AudioRandom = 133
  }

  export declare class Region {
    left: number;
    top: number;
    width: number;
    height: number;

    constructor(left: number, top: number, width: number, height: number);

    area(): number;

    toString(): string;
  }


  type Window = {
    getTitle: () => Promise<string>;
    focus: () => Promise<boolean>;
    getRegion(): Promise<Region>;
  }

  type Handler = {
    keyboardType: (key: string | Key) => Promise<void>;
    pressKey: (key: Key) => Promise<void>;
    getActiveWindow: () => Promise<Window>;
    getWindows: () => Promise<Window[]>;
    mouseMove: (x: number, y: number) => Promise<void>;
    mouseLeftClick: () => Promise<void>;
    focusClientWindow: (windowTitle: string) => Promise<void>;
  }

  class HandlerImpl implements Handler {
    async pressKey(key: Key): Promise<void> {
      await keyboard.pressKey(key);
    }

    async keyboardType(key: string | Key): Promise<void> {
      await keyboard.type(key as any);
    }

    async getActiveWindow(): Promise<Window> {
      const window = await NutGetActiveWindow();
      return HandlerImpl.mapWindow(window);
    }

    async getWindows(): Promise<Window[]> {
      const windows = await getWindows();

      return windows.map((window) => {
        return HandlerImpl.mapWindow(window);
      });
    }

    async mouseMove(x: number, y: number): Promise<void> {
      await mouse.move([{ x, y }]);
    }

    async mouseLeftClick(): Promise<void> {
      await mouse.leftClick();
    }

    async focusClientWindow(windowTitle: string, retry: number = 10): Promise<void> {
      const windows = await getWindows();
      for (const window of windows) {
        if ((await window.getTitle()).includes(windowTitle)) {
          for (let i = 0; i < retry; i++) {
            await window.focus();
            const region = await window.getRegion();
            await this.mouseMove((region.left + region.width) / 2, (region.top + region.height) / 2);
            await this.mouseLeftClick();
            if ((await (await this.getActiveWindow()).getTitle()) === (await window.getTitle())) {
              return;
            }
            await sleep(Math.min(50 * 2 ** i, 1000));
          }
        }
      }

      throw new Error(`Window with title ${windowTitle} not found`);
    }

    private static mapWindow(window: NutWindow) {
      return {
        getTitle: async () => {
          return await window.getTitle();
        },
        focus: async () => {
          return await window.focus();
        },
        getRegion: async () => {
          return await window.getRegion();
        },
      };
    }
  }

  export const Handler: Handler = new HandlerImpl();
}
