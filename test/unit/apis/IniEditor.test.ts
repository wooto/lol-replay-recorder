import { expect } from "chai";
import { IniEditor } from "../../../src/apis/IniEditor";
import path, { dirname } from "path";
import { fileURLToPath } from "url";
import fs from "fs";
import { tmpdir } from "os";
import exp from "constants";


const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);


describe('IniEditor', () => {
    beforeEach(() => {
        fs.copyFileSync(path.join(__dirname, 'test.ini'), path.join(tmpdir(), 'test.ini'));
    })

    it('load test.ini', async () => {
        const inieditor = new IniEditor(path.join(tmpdir(), 'test.ini'));

        expect(inieditor.data).to.be.not.null;
        expect(inieditor.data.GameEvents).to.be.not.null;
        expect(inieditor.data.GameEvents.evtSelectOrderPlayer1).to.eql('[1]');
    });

    it('update test.ini', async () => {
        const inieditor = new IniEditor(path.join(tmpdir(), 'test.ini'));
        inieditor.updateSection("GameEvents", "evtSelectOrderPlayer1", '[2]');
        inieditor.save();

        const inieditor2 = new IniEditor(path.join(tmpdir(), 'test.ini'));
        expect(inieditor2.data).to.be.not.null;
        expect(inieditor2.data.GameEvents.evtSelectOrderPlayer1).to.eql('[2]');
    });
});