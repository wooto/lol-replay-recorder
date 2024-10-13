import * as fs from "fs";
import * as ini from "ini";

export class IniEditor {
  filename: string;
  data: any

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
    fs.writeFileSync(this.filename, ini.stringify(this.data));
  }

  update(key: string, value: any): void {
    this.data[key] = value;
  }

}
