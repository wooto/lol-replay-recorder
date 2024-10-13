import * as fs from "fs";
import * as ini from "ini";

export class IniEditor {
  filename: string;
  data: any;

  constructor(filename: string) {
    this.filename = filename;
    this.data = this.loadIni();
  }

  private loadIni(): any {
    try {
      return ini.parse(fs.readFileSync(this.filename, "utf8"));
    } catch (error) {
      throw new Error(`Failed to load INI file: ${error.message}`);
    }
  }

  save(): void {
    const iniData = ini.stringify(this.data, { whitespace: true })
      .replace(/"\[([^\]]+)\]"/g, '[$1]');

    fs.writeFileSync(this.filename, iniData, "utf8");
  }

  updateSection(section: string, key: string, value: any): void {
    if (!this.data[section]) {
      this.data[section] = {};
    }
    this.data[section][key] = value;
  }

}
