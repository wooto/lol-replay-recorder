// src/windowController.ts
import { Library, types } from 'ffi-napi';

const user32 = new Library('user32', {
  'FindWindowA': [types.int32, [types.CString, types.CString]],
  'SetForegroundWindow': [types.bool, [types.int32]],
});


export function focusWindow(title: string): boolean {
  const windowHandle = user32.FindWindowA(null, title);
  if (windowHandle !== 0) {
    return user32.SetForegroundWindow(windowHandle) as boolean;
  }
  return false;
}
