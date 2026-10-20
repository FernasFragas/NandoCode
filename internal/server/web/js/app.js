// Entry point: imports and mount calls only. Each browser feature lives in
// features/<name>.js and exports mount(ctx); mount order matters only for the
// Escape chain (dialog, then permission, then composer) and for session, which
// goes last because it starts the first session.
import { createContext } from './ctx.js';
import { adoptToken } from './token.js';
import { mount as mountKeys } from './features/keys.js';
import { mount as mountDialog } from './features/dialog.js';
import { mount as mountSettings } from './features/settings.js';
import { mount as mountCredential } from './features/credential.js';
import { mount as mountShell } from './features/shell.js';
import { mount as mountHeader } from './features/header.js';
import { mount as mountNav } from './features/nav.js';
import { mount as mountSessions } from './features/sessions.js';
import { mount as mountRunControls } from './features/runcontrols.js';
import { mount as mountTree } from './features/tree.js';
import { mount as mountTranscript } from './features/transcript.js';
import { mount as mountPermission } from './features/permission.js';
import { mount as mountComposer } from './features/composer.js';
import { mount as mountSplit } from './features/split.js';
import { mount as mountTabs } from './features/tabs.js';
import { mount as mountCodeview } from './features/codeview.js';
import { mount as mountEditor } from './features/editor.js';
import { mount as mountAgents } from './features/agents.js';
import { mount as mountRail } from './features/rail.js';
import { mount as mountFooter } from './features/footer.js';
import { mount as mountSession } from './features/session.js';

const FEATURES = [
  mountKeys, mountDialog, mountSettings, mountCredential, mountShell, mountHeader, mountNav,
  mountSessions, mountRunControls, mountTree, mountTranscript, mountPermission, mountComposer, mountSplit, mountTabs, mountCodeview, mountEditor, mountAgents, mountRail, mountFooter,
  mountSession,
];

function sessionStorageOrNull(win) {
  try {
    return win.sessionStorage;
  } catch {
    return null;
  }
}

export function boot({ doc, win, fetchImpl, storage }) {
  const token = adoptToken(win, storage);
  const ctx = createContext({ doc, win, fetchImpl, storage, token });
  FEATURES.forEach((mount) => mount(ctx));
  return ctx;
}

if (typeof window !== 'undefined' && typeof document !== 'undefined' && document.getElementById('app')) {
  boot({ doc: document, win: window, fetchImpl: (...a) => window.fetch(...a), storage: sessionStorageOrNull(window) });
}
