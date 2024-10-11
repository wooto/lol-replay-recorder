import * as fs from 'fs';
import * as yaml from 'js-yaml';

export class YamlEditor {
    filename: string;
    data: any;

    constructor(filename: string) {
        this.filename = filename;
        this.data = this.loadYAML();
    }

    private loadYAML(): any {
        try {
            const fileContents = fs.readFileSync(this.filename, 'utf8');
            return yaml.load(fileContents);
        } catch (error) {
            throw new Error(`Failed to load YAML file: ${error.message}`);
        }
    }

    private saveYAML(): void {
        try {
            const newYaml = yaml.dump(this.data);
            fs.writeFileSync(this.filename, newYaml, 'utf8');
            console.log(`YAML file saved to ${this.filename}`);
        } catch (error) {
            throw new Error(`Failed to save YAML file: ${error.message}`);
        }
    }

    updateKey(path: string, value: any): void {
        const keys = path.split('.');
        let current = this.data;

        for (let i = 0; i < keys.length - 1; i++) {
            if (!(keys[i] in current)) {
                current[keys[i]] = {}; 
            }
            current = current[keys[i]];
        }

        current[keys[keys.length - 1]] = value;
    }

    saveChanges(): void {
        this.saveYAML();
    }
}
