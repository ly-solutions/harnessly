import {connect} from './index.js';
import {installBridge} from './bridge-shim.js';

let connection;
let trustedOrigin;

/** Connect a remote editor using its configured Harnessly origin. */
export async function connectEmbedded({hostOrigin}) {
  if (typeof window === 'undefined') return null;
  const origin = new URL(hostOrigin);
  if (origin.protocol !== 'https:' || origin.origin !== hostOrigin) {
    throw new TypeError('An exact HTTPS host origin is required');
  }
  if (connection) {
    if (trustedOrigin !== hostOrigin) throw new Error('Host origin changed');
    return connection;
  }
  if (window.parent === window || !/(?:^#|&)hl-nonce=[A-Za-z0-9_-]+(?:&|$)/.test(location.hash)) return null;
  trustedOrigin = hostOrigin;
  installBridge(hostOrigin);
  connection = connect();
  return connection;
}
