"""Preserve mounted manager data and run the bundled game through the web UI."""
import hashlib
import json
import os
from pathlib import Path
import tempfile


def prepare(data=Path('/opt/fsm-data'), game=Path('/opt/factorio')):
    data.mkdir(parents=True, exist_ok=True)
    for name in ('saves', 'mods', 'config'):
        (game / name).mkdir(parents=True, exist_ok=True)
    path = data / 'conf.json'
    original = path.read_bytes() if path.exists() else None
    config = json.loads(original) if original else {}
    config.update({
        'factorio_dir': str(game), 'saves_dir': str(game / 'saves'),
        'mods_dir': str(game / 'mods'), 'basemod_dir': str(game / 'data/base'),
        'config_directory': str(game / 'config'), 'config_file': str(game / 'config/config.ini'),
        'factorio_binary': str(game / 'bin/x64/factorio'),
        'settings_file': str(game / 'config/server-settings.json'),
        'factorio_admin_file': str(game / 'config/server-adminlist.json'),
        'sq_lite_database_file': str(data / 'sqlite.db'),
        'factorio_credentials_file': str(data / 'factorio.auth'),
        'mod_pack_dir': str(data / 'mod-packs'),
        'logfile': str(game / 'factorio-current.log'),
        'log_file': str(data / 'factorio-server-manager.log'),
        'server_ip': '0.0.0.0', 'server_port': '80',
    })
    config.setdefault('secure', False)
    config.setdefault('rcon_port', 27015)
    config.setdefault('max_upload_size', 536870912)
    updated = (json.dumps(config, indent=2) + '\n').encode()
    if original != updated:
        if original:
            backup = data / ('conf-backup-' + hashlib.sha256(original).hexdigest() + '.json')
            if not backup.exists():
                with backup.open('xb') as stream:
                    stream.write(original)
        fd, temporary = tempfile.mkstemp(dir=data, prefix='.conf-')
        try:
            with os.fdopen(fd, 'wb') as stream:
                stream.write(updated)
                stream.flush()
                os.fsync(stream.fileno())
            os.replace(temporary, path)
        finally:
            if os.path.exists(temporary):
                os.unlink(temporary)
    ini = game / 'config/config.ini'
    if not ini.exists():
        ini.write_text('[path]\nread-data=' + str(game / 'data') + '\nwrite-data=' + str(game) + '\n')
    return path


if __name__ == '__main__':
    os.umask(0o077)
    config = prepare()
    os.chdir('/opt/fsm')
    os.execv('/opt/fsm/factorio-server-manager', ['factorio-server-manager', '--conf', str(config), '--autostart=false'])
