// window.electronAPI for the Harnessly web page: the same surface the Electron shell's preload gives, so the page
// needs no second code path. Calls from a page outside the origin the app was pointed at are refused by the ACL.
(function () {
  if (window.electronAPI) return;
  var invoke = function (cmd, args) {
    var ipc = window.__TAURI_INTERNALS__;
    return ipc ? ipc.invoke(cmd, args || {}) : Promise.reject(new Error('desktop bridge unavailable'));
  };
  var ua = navigator.userAgent;
  var platform = /Mac/.test(ua) ? 'darwin' : /Win/.test(ua) ? 'win32' : 'linux';
  var api = {
    isElectron: false,
    isDesktop: true,
    shell: 'tauri',
    platform: platform,
    openExternal: function (url) { return invoke('open_external', { url: String(url) }); },
    showNotification: function (title, body) {
      return invoke('show_notification', { title: String(title), body: String(body) });
    },
    openNewWindow: function () { return invoke('open_new_window'); },
    closeWindow: function () { return invoke('close_window'); },
    setDockBadge: function (count) { return invoke('set_dock_badge', { count: Number(count) || 0 }); },
    setLocale: function (locale) { return invoke('set_locale', { locale: String(locale) }); },
    setNativeTheme: function (theme) { return invoke('set_native_theme', { theme: String(theme) }); },
    checkForUpdates: function () { return Promise.resolve(null); },
    // Desktop notifications carry no click callback here; the subscription is kept so callers need no branch.
    onNotificationClick: function () { return function () {}; },
  };
  Object.defineProperty(window, 'electronAPI', { value: Object.freeze(api), configurable: false, writable: false });
})();
