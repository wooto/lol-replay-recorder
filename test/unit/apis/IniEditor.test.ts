import { expect } from "chai";
import { IniEditor } from "../../../src/apis/IniEditor";
import path, { dirname } from "path";
import { fileURLToPath } from "url";


const __filename = fileURLToPath(import.meta.url);

const __dirname = dirname(__filename);

describe('IniEditor', () => {

    it('load test.ini', async () => {
        const inieditor = new IniEditor(path.join(__dirname, 'test.ini'));

        expect(inieditor.data).to.be.not.null;
        expect(inieditor.data.test).to.eql("1")
        expect(inieditor.data['test.a']).to.eql("1-a")
        expect(inieditor.data['test.b']).to.eql("1-b")
    });

    it('update test.ini', async () => {
        const inieditor = new IniEditor(path.join(__dirname, 'test.ini'));
        inieditor.update('test', '2');
        inieditor.update('test.a', '2-a');
        inieditor.update('test.b', '2-b');
        inieditor.save();

        const inieditor2 = new IniEditor(path.join(__dirname, 'test.ini'));
        expect(inieditor2.data).to.be.not.null;
        expect(inieditor2.data.test).to.eql("2")
        expect(inieditor2.data['test.a']).to.eql("2-a")
        expect(inieditor2.data['test.b']).to.eql("2-b")
    });
});