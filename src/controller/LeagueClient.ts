import { exec } from "child_process";
import { promisify } from "util";
import { existsSync } from "node:fs";
import path from "node:path";
import { sleepInSeconds } from "../utils/utils";
import { RiotGameClient } from "./RiotGameClient";
import { LeagueClientUx } from "./LeagueClientUx";
import { Locale } from "../model/Locale";
import { RiotTypes } from "../model/RiotTypes";
import { YamlEditor } from "../apis/YamlEditor";
import { WindowHandler } from "./WindowHandler";
import { IniEditor } from "../apis/IniEditor";
import { getActiveWindow } from "@kirillvakalov/nut-tree__nut-js";

const execAsync = promisify(exec);

export class LeagueClient {
  async startRiotProcessesSafely(params: {
    region: RiotTypes.Region,
    locale: Locale,
    username: string,
    password: string,
  }) {
    await this.stopRiotProcesses();
    await this.setLocale(params.locale);
    for (let i = 0; i < 5; i++) {
      try {
        await new RiotGameClient().startRiotClient(params.region as any, params.locale);
        await new RiotGameClient().login(params.username, params.password, params.region);

        const { action } = await new LeagueClientUx().getState({ options: { retry: 60 } });
        if (action !== "Idle") {
          throw new Error("Client is not ready", action);
        }
        break;
      } catch (e) {
        console.error("Error starting Riot processes:", e);
        await sleepInSeconds(1);
        await this.stopRiotProcesses();
      }
    }

    const { locale } = await new LeagueClientUx().getRegionLocale(30);
    console.log("RCU response", await new RiotGameClient().getRegionLocale(30));
    console.log("LCU response", await new LeagueClientUx().getRegionLocale(30));

    if (locale !== params.locale) {
      throw new Error(`Locale is not correct: ${locale}`);
    }

    await sleepInSeconds(5);
  }

  async stopRiotProcesses() {
    const prcoesses = [
      "RiotClientUx.exe",
      "RiotClientServices.exe",
      "RiotClient.exe",
      "Riot Client.exe",
      "LeagueClient.exe",
      "League of Legends.exe",
      "LeagueClientUxRender.exe"
    ];
    for (const process of prcoesses) {
      try {
        await execAsync(`taskkill /F /IM "${process}" /T`);
      } catch (e) {
        // ignore
      }
    }

    for (const process of prcoesses) {
      for (let i = 0; i < 30; i++) {
        try {
          // process to check if the process is still running
          await sleepInSeconds(1);
          const { stdout } = await execAsync(`tasklist /FI "IMAGENAME eq ${process}"`);
          if (!stdout.includes(process)) {
            break;
          }
        } catch (e) {
          break;
        }
      }
    }
    await new RiotGameClient().removeLockfile();
    await new LeagueClientUx().removeLockfile();
  };

  async findWindowsInstalled(): Promise<string[]> {
    const paths = ["C:\\Riot Games\\League of Legends"];
    for (const path of paths) {
      if (existsSync(path)) {
        return [path];
      }
    }
    return [];
  }

  getProductSettingsPath(): string {
    return path.join("C:", "ProgramData", "Riot Games", "Metadata", "league_of_legends.live", "league_of_legends.live.product_settings.yaml");
  }

  async getInstalledPaths(): Promise<string[]> {
    return this.findWindowsInstalled();
  }

  async getConfigFilePaths(): Promise<string[]> {
    const installedPaths = await this.getInstalledPaths();
    return installedPaths.map(installedPath => {
      return this.getConfigFilePath(installedPath);
    });
  }

  getConfigFilePath(initialPath: string): string | null {
    let lolPath = path.resolve(initialPath);
    let configPaths = [
      path.join(lolPath, "DATA", "CFG", "game.cfg"),
      path.join(lolPath, "Config", "game.cfg"),
      path.join(lolPath, "Game", "Config", "game.cfg")
    ];
    for (const configPath of configPaths) {
      if (existsSync(configPath)) {
        return configPath;
      }
    }
    return null;
  }

  async isGameEnabled(path: string): Promise<boolean> {
    try {
      const editor = new IniEditor(path);
      const enableReplayApi = editor.data["config"]?.["General"]?.["EnableReplayApi"];
      return enableReplayApi === 1 || enableReplayApi === true || enableReplayApi === "1";
    } catch (error) {
      return false;
    }
  }

  async setGameEnabled(path: string, enabled: boolean): Promise<void> {
    try {
      const editor = new IniEditor(path);
      editor.updateSection("General", "EnableReplayApi", enabled);
      editor.save();
    } catch (error) {
      console.error(`Error writing config file: ${error}`);
    }
  }

  async getGameInputIniPath(): Promise<any> {
    return path.join("C:", "Riot Games", "League of Legends", "Config", "input.ini");
  }

  async setDefaultInputIni(): Promise<void> {
    try {
      const editor = new IniEditor(await this.getGameInputIniPath());
      editor.updateSection("GameEvents", "evtSelectOrderPlayer1", "[1]");
      editor.updateSection("GameEvents", "evtSelectOrderPlayer2", "[2]");
      editor.updateSection("GameEvents", "evtSelectOrderPlayer3", "[3]");
      editor.updateSection("GameEvents", "evtSelectOrderPlayer4", "[4]");
      editor.updateSection("GameEvents", "evtSelectOrderPlayer5", "[5]");
      editor.save();
    } catch (error) {
      console.error(`Error writing config file: ${error}`);
    }
  }

  async setLocale(locale: string) {
    const yamlEditor = new YamlEditor(this.getProductSettingsPath());

    const availableLocales: string[] = yamlEditor.data["locale_data"]["available_locales"];
    if (!availableLocales.includes(locale)) {
      throw new Error(`Invalid locale: ${locale}, available locales: ${availableLocales}`);
    }
    yamlEditor.update("locale_data.default_locale", locale);
    yamlEditor.update("settings.locale", locale);
    yamlEditor.saveChanges();
  }


  async focusClientWindow(): Promise<void> {
    const targetWindowTitle = "League of Legends (TM)";
    await WindowHandler.Handler.focusClientWindow(targetWindowTitle);
  }
}
