// A block on the start page: what waits for the person today, from the workspace the settings name.
harnessly.connect().then(async (hl) => {
  const root = document.getElementById('root');
  const ws = (await hl.storage.install.get('workspace')) || 'team.example.com';
  root.innerHTML = '<p class="today"></p>';
  root.querySelector('.today').textContent = 'Nothing waits for you in ' + ws + ' today.';
});
