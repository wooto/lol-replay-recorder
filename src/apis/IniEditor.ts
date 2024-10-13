import * as fs from "fs";

type RecursiveMap = Map<string, RecursiveMap>;

export class IniEditor {
  filename: string;
  data: RecursiveMap;

  constructor(filename: string) {
    this.filename = filename;
    this.data = this.loadIni();
  }

  private loadIni(): any {
    try {
      const fileContents = fs.readFileSync(this.filename, "utf8");
      return this.parseIni(fileContents);
    } catch (error) {
      throw new Error(`Failed to load INI file: ${error.message}`);
    }
  }

  private parseIni(fileContents: string): any {
    const lines = fileContents.split("\n");
    const data: any = {};
    let currentSection = "";

    for (const line of lines) {
      if (line.startsWith("[")) {
        currentSection = line.slice(1, line.length - 1);
        data[currentSection] = {};
      } else if (line.includes("=")) {
        const [key, value] = line.split("=");
        data[currentSection][key] = value;
      }
    }

    return data;
  }

  save(): void {
    const newIni = this.stringifyIni();
    fs.writeFileSync(this.filename, newIni, "utf8");
    console.log(`INI file saved to ${this.filename}`);
  }

  private stringifyIni(): string {
    let iniString = "";

    for (const section in this.data) {
      iniString += `[${section}]\n`;

      for (const key in this.data[section]) {
        iniString += `${key}=${this.data[section][key]}\n`;
      }

      iniString += "\n";
    }

    return iniString;
  }

  update(key: string, value: any): void {
    const keys = key.split(".");
    let current = this.data;

    for (let i = 0; i < keys.length - 1; i++) {
      if (!(keys[i] in current)) {
        current[keys[i]] = {};
      }
      current = current[keys[i]];
    }

    current[keys[keys.length - 1]] = value;
  }

}
