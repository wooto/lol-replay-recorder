// test/windowController.test.ts
import { expect } from 'chai';
import { focusWindow } from '../../src/controller/WindowController';
import { Library } from 'ffi-napi';
import sinon from 'sinon';

describe('focusWindow', () => {
  let findWindowAStub: sinon.SinonStub;
  let setForegroundWindowStub: sinon.SinonStub;

  beforeEach(() => {
    findWindowAStub = sinon.stub(Library.prototype, 'FindWindowA');
    setForegroundWindowStub = sinon.stub(Library.prototype, 'SetForegroundWindow');
  });

  afterEach(() => {
    findWindowAStub.restore();
    setForegroundWindowStub.restore();
  });

  it('should focus window if found', () => {
    findWindowAStub.returns(100);
    setForegroundWindowStub.returns(true);

    const result = focusWindow('My Window Title');
    expect(result).to.be.true;
  });
});
