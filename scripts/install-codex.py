#!/usr/bin/env python3
"""Install the native Codex adapter from this checkout (Python 3.11+).

No downloads, shell commands, model credentials or DSH/Claude configuration writes.
The Codex CLI owns TOML registration. Previous runtime versions/backups are retained.
"""
import argparse
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import tomllib
import uuid

MARKER = '.agentflow-codex.json'


def unrelated_config(raw):
    parsed = tomllib.loads(raw.decode('utf-8')) if raw else {}
    servers = parsed.get('mcp_servers', {})
    servers.pop('agentflow', None)
    if not servers:
        parsed.pop('mcp_servers', None)
    return parsed


def install(source, codex_home, skill_dir, binary, codex_command, *, dry_run=False,
            replace_skill=False, replace_mcp=False, runner=subprocess.run):
    source, home, skill, binary = map(lambda p: Path(p).absolute(),
                                     (source, codex_home, skill_dir, binary))
    for p in (home, skill, binary):
        if any(x.is_symlink() for x in [p, *p.parents]):
            raise ValueError('symlink destination/source requires explicit physical path: ' + str(p))
    state = home / 'agentflow'
    for p in (state, state / 'runtime', state / 'backups', state / 'data',
              state / 'data/agentflow.db', home / 'config.toml'):
        if p.is_symlink() or (hasattr(p, 'is_junction') and p.is_junction()):
            raise ValueError('linked state path requires inspection: ' + str(p))
    if skill == home or skill in home.parents or home in skill.parents:
        raise ValueError('skill directory must be outside Codex home')
    if skill.exists() and not (skill / MARKER).is_file() and not replace_skill:
        raise ValueError('unmanaged skill already exists; use --replace-skill after reviewing it')
    config = home / 'config.toml'
    original = config.read_bytes() if config.exists() else None
    parsed = tomllib.loads(original.decode('utf-8')) if original else {}
    existing = parsed.get('mcp_servers', {}).get('agentflow')
    if existing and not replace_mcp:
        command = Path(existing.get('command', '')).absolute()
        if state / 'runtime' not in command.parents:
            raise ValueError('unmanaged agentflow MCP already exists; use --replace-mcp after reviewing it')
    if not binary.is_file():
        raise ValueError('engine binary missing: ' + str(binary))
    for p in ('codex-agentflow/skills/agentflow/SKILL.md', 'bt_service/__main__.py',
              'trees/leader-default.json'):
        if not (source / p).is_file():
            raise ValueError('source checkout missing: ' + p)
    ident = uuid.uuid4().hex
    runtime = state / 'runtime' / ident
    backup = state / 'backups' / ident
    skill_backup = skill.parent / ('.agentflow-backup-' + ident)
    db = state / 'data' / 'agentflow.db'
    executable = runtime / ('agentflow.exe' if os.name == 'nt' else 'agentflow')
    args = [str(codex_command), 'mcp', 'add', 'agentflow',
            '--env', 'AGENTFLOW_DB_PATH=' + str(db),
            '--env', 'AGENTFLOW_BT_DIR=' + str(runtime),
            '--env', 'AGENTFLOW_PYTHON=' + sys.executable,
            '--', str(executable), 'stdio']
    result = {'skill': str(skill), 'runtime': str(runtime), 'database': str(db),
              'backup': str(backup), 'skill_backup': str(skill_backup), 'command': args, 'dry_run': dry_run}
    if dry_run:
        return result
    state.mkdir(parents=True, exist_ok=True, mode=0o700)
    lock = state / '.install-lock'
    lock.mkdir()  # A stale lock is inspected, never silently removed.
    skill_installed = False
    registration_started = False
    try:
        # Refuse to overwrite configuration changed during preparation.
        if (config.read_bytes() if config.exists() else None) != original:
            raise ValueError('Codex config changed; retry installation')
        backup.mkdir(parents=True, mode=0o700)
        if original is not None:
            (backup / 'config.toml').write_bytes(original)
            (backup / 'config.toml').chmod(0o600)
        runtime.mkdir(parents=True)
        shutil.copy2(binary, executable)
        executable.chmod(0o755)
        for directory in ('bt_service', 'trees'):
            shutil.copytree(source / directory, runtime / directory,
                            ignore=shutil.ignore_patterns('__pycache__', '*.pyc'))
        db.parent.mkdir(parents=True, exist_ok=True)
        skill.parent.mkdir(parents=True, exist_ok=True)
        skill_backup.mkdir(mode=0o700)
        if skill.exists():
            skill.rename(skill_backup / 'skill')
        # No recursive deletion: failed copies are retained for inspection.
        skill_installed = True
        shutil.copytree(source / 'codex-agentflow/skills/agentflow', skill)
        (skill / MARKER).write_text(json.dumps({'runtime': str(runtime), 'database': str(db)}, indent=2), encoding='utf-8')
        if (config.read_bytes() if config.exists() else None) != original:
            raise ValueError('Codex config changed during preparation; retry installation')
        if original is None:
            config.write_text('', encoding='utf-8')
        registration_started = True
        runner(args, check=True, env={**os.environ, 'CODEX_HOME': str(home)},
               capture_output=True, text=True, timeout=60)
        (backup / 'install.json').write_text(json.dumps(result, indent=2), encoding='utf-8')
    except BaseException:
        if registration_started:
            current = config.read_bytes() if config.exists() else None
            try:
                safe_restore = unrelated_config(current) == unrelated_config(original)
            except (ValueError, UnicodeError):
                safe_restore = False
            if safe_restore:
                if original is None:
                    if config.exists():
                        config.rename(backup / 'failed-config.toml')
                else:
                    config.write_bytes(original)
            else:
                print('Registration failed; changed or invalid config retained to avoid losing concurrent edits. Review backup: ' + str(backup), file=sys.stderr)
        if skill_installed and skill.exists():
            skill.rename(skill_backup / 'failed-skill')
        if (skill_backup / 'skill').exists():
            (skill_backup / 'skill').rename(skill)
        raise
    finally:
        lock.rmdir()
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--codex-home', type=Path, default=Path(os.environ.get('CODEX_HOME', Path.home() / '.codex')))
    parser.add_argument('--skill-dir', type=Path, default=Path.home() / '.agents/skills/agentflow')
    parser.add_argument('--binary', type=Path, help='Use an already-built trusted engine; otherwise build from this checkout')
    parser.add_argument('--codex-command', default=shutil.which('codex.exe' if os.name == 'nt' else 'codex'))
    parser.add_argument('--dry-run', action='store_true')
    parser.add_argument('--replace-skill', action='store_true')
    parser.add_argument('--replace-mcp', action='store_true')
    args = parser.parse_args()
    if not args.codex_command:
        parser.error('Codex CLI not found; supply --codex-command with the native executable path')
    if os.name == 'nt' and Path(args.codex_command).suffix.lower() in ('.cmd', '.bat', '.ps1'):
        parser.error('Use codex.exe rather than a shell shim on Windows')
    source = Path(__file__).resolve().parents[1]
    if args.dry_run and args.binary is None:
        parser.error('dry-run requires --binary; it never builds or writes files')
    if args.dry_run:
        print(json.dumps(install(source, args.codex_home, args.skill_dir, args.binary,
                                 args.codex_command, dry_run=True, replace_skill=args.replace_skill,
                                 replace_mcp=args.replace_mcp), ensure_ascii=False, indent=2))
        return
    with tempfile.TemporaryDirectory(prefix='agentflow-codex-build-') as build:
        binary = args.binary
        if binary is None:
            binary = Path(build) / ('agentflow.exe' if os.name == 'nt' else 'agentflow')
            subprocess.run(['go', 'build', '-o', str(binary), './cmd/agentflow'], cwd=source, check=True)
        if not args.dry_run:
            info = subprocess.check_output([str(binary.resolve()), 'version-json'], text=True, timeout=15)
            json.loads(info)  # Do not install an unrelated/nonfunctional executable.
        result = install(source, args.codex_home, args.skill_dir, binary, args.codex_command,
                         dry_run=args.dry_run, replace_skill=args.replace_skill, replace_mcp=args.replace_mcp)
        print(json.dumps(result, ensure_ascii=False, indent=2))
        if not args.dry_run:
            print('Installed configuration and skill. Restart/reload Codex, then invoke $agentflow setup and call flow_ping in-session.')


if __name__ == '__main__':
    main()
