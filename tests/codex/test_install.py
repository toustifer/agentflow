import importlib.util
import json
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location('install_codex', ROOT / 'scripts/install-codex.py')
installer = importlib.util.module_from_spec(spec)
spec.loader.exec_module(installer)


class InstallTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix='agentflow space ')
        self.base = Path(self.tmp.name)
        self.home = self.base / 'codex home'
        self.skill = self.base / 'skills/agentflow'
        self.binary = self.base / 'engine.exe'
        self.binary.write_bytes(b'test engine')
        self.calls = []

    def tearDown(self):
        self.tmp.cleanup()

    def run_cli(self, args, **kwargs):
        self.calls.append((args, kwargs))
        config = self.home / 'config.toml'
        config.write_text(config.read_text() + '\n[mcp_servers.agentflow]\ncommand="registered"\n')
        return subprocess.CompletedProcess(args, 0)

    def install(self, **kwargs):
        return installer.install(ROOT, self.home, self.skill, self.binary,
                                 'codex.exe', runner=self.run_cli, **kwargs)

    def test_dry_run_no_writes(self):
        self.install(dry_run=True)
        self.assertFalse(self.home.exists())
        self.assertFalse(self.skill.exists())
        self.assertEqual(self.calls, [])

    def test_install_preserves_config_and_uses_persistent_db(self):
        self.home.mkdir()
        config = self.home / 'config.toml'
        config.write_text('model = "keep-me"\n')
        result = self.install()
        args, options = self.calls[0]
        self.assertIn('model = "keep-me"', config.read_text())
        self.assertEqual(options['env']['CODEX_HOME'], str(self.home))
        self.assertIn('AGENTFLOW_DB_PATH=' + str(self.home / 'agentflow/data/agentflow.db'), args)
        self.assertFalse(options.get('shell', False))
        self.assertTrue((self.skill / 'SKILL.md').exists())
        self.assertTrue((Path(result['runtime']) / 'bt_service/__main__.py').exists())
        self.assertTrue((Path(result['backup']) / 'config.toml').exists())

    def test_unknown_skill_not_overwritten(self):
        self.skill.mkdir(parents=True)
        (self.skill / 'SKILL.md').write_text('foreign skill')
        with self.assertRaisesRegex(ValueError, 'unmanaged'):
            self.install()
        self.assertEqual((self.skill / 'SKILL.md').read_text(), 'foreign skill')
        self.assertFalse(self.home.exists())

    def test_reinstall_preserves_db_and_backs_up_skill(self):
        self.install()
        db = self.home / 'agentflow/data/agentflow.db'
        db.write_bytes(b'existing database')
        # Fake runner preserves unrelated config without requiring a TOML parser.
        self.home.joinpath('config.toml').write_text('model="keep"\n')
        result = self.install()
        self.assertEqual(db.read_bytes(), b'existing database')
        self.assertTrue((Path(result['skill_backup']) / 'skill/SKILL.md').exists())

    def test_registration_failure_restores_previous_files(self):
        self.install()
        previous = (self.skill / 'SKILL.md').read_bytes()
        config = self.home / 'config.toml'
        config.write_text('model="before"\n')
        def fail(args, **kwargs):
            config.write_text('model="before"\n[mcp_servers.agentflow]\ncommand="partial"\n')
            raise subprocess.CalledProcessError(1, args)
        with self.assertRaises(subprocess.CalledProcessError):
            installer.install(ROOT, self.home, self.skill, self.binary, 'codex.exe', runner=fail)
        self.assertEqual(config.read_text(), 'model="before"\n')
        self.assertEqual((self.skill / 'SKILL.md').read_bytes(), previous)

    def test_failure_keeps_concurrent_config_edit(self):
        self.home.mkdir()
        config = self.home / 'config.toml'
        config.write_text('model="before"\n')
        def fail(args, **kwargs):
            config.write_text('model="concurrent-edit"\n')
            raise subprocess.CalledProcessError(1, args)
        with self.assertRaises(subprocess.CalledProcessError):
            installer.install(ROOT, self.home, self.skill, self.binary, 'codex.exe', runner=fail)
        self.assertEqual(config.read_text(), 'model="concurrent-edit"\n')
        self.assertFalse(self.skill.exists())

    def test_foreign_mcp_not_overwritten(self):
        self.home.mkdir()
        config = self.home / 'config.toml'
        original = '[mcp_servers.agentflow]\ncommand="foreign-engine"\n'
        config.write_text(original)
        with self.assertRaisesRegex(ValueError, 'unmanaged agentflow MCP'):
            self.install()
        self.assertEqual(config.read_text(), original)

    def test_cross_disk_skill_update_and_rollback(self):
        if ROOT.anchor.lower() == self.base.anchor.lower():
            self.skipTest('requires checkout and temporary home on different volumes')
        with tempfile.TemporaryDirectory(prefix='.tmp-codex-skill-', dir=ROOT) as target:
            self.skill = Path(target) / 'agentflow'
            self.install()
            self.home.joinpath('config.toml').write_text('model="keep"\n')
            result = self.install()
            self.assertEqual(Path(result['skill_backup']).anchor, self.skill.anchor)
            self.home.joinpath('config.toml').write_text('model="keep"\n')
            def fail(args, **kwargs):
                raise subprocess.CalledProcessError(1, args)
            with self.assertRaises(subprocess.CalledProcessError):
                installer.install(ROOT, self.home, self.skill, self.binary, 'codex.exe', runner=fail)
            self.assertTrue((self.skill / 'SKILL.md').is_file())

    def test_skill_has_native_invariants(self):
        self.install()
        body = (self.skill / 'SKILL.md').read_text(encoding='utf-8')
        workflow = (self.skill / 'references/workflow.md').read_text(encoding='utf-8')
        self.assertIn('$agentflow', body)
        self.assertIn('task_prepare_start', workflow)
        self.assertIn('runtime.provider', workflow)
        self.assertIn('codex', workflow)
        self.assertNotIn('runtime.provider = "claude_code"', workflow)
