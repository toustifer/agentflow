#!/usr/bin/env python3
"""Protocol diagnostic only, never a substitute for in-session MCP/task execution.

Loads only agentflow's registered command; overrides its DB with a temporary DB.
Exercises initialize, tools/list and flow_ping, then closes the subprocess.
"""
import argparse
import json
import os
from pathlib import Path
import subprocess
import tempfile
import tomllib


def check(home):
    config = tomllib.loads((Path(home) / 'config.toml').read_text(encoding='utf-8'))
    server = config['mcp_servers']['agentflow']
    with tempfile.TemporaryDirectory(prefix='agentflow-codex-probe-') as directory:
        env = {**os.environ, **server.get('env', {}),
               'AGENTFLOW_DB_PATH': str(Path(directory) / 'probe.db')}
        messages = [
            {'jsonrpc': '2.0', 'id': 1, 'method': 'initialize', 'params': {
                'protocolVersion': '2024-11-05', 'capabilities': {},
                'clientInfo': {'name': 'agentflow-codex-diagnostic', 'version': '1.0.0'}}},
            {'jsonrpc': '2.0', 'method': 'notifications/initialized'},
            {'jsonrpc': '2.0', 'id': 2, 'method': 'tools/list', 'params': {}},
            {'jsonrpc': '2.0', 'id': 3, 'method': 'tools/call', 'params': {
                'name': 'flow_ping', 'arguments': {}}},
        ]
        result = subprocess.run([server['command'], *server.get('args', [])], env=env,
                                input=''.join(json.dumps(m) + '\n' for m in messages),
                                capture_output=True, text=True, timeout=45, check=True)
        replies = {m['id']: m for line in result.stdout.splitlines()
                   if (m := json.loads(line)).get('id') is not None}
        for ident in (1, 2, 3):
            if ident not in replies or 'error' in replies[ident]:
                raise RuntimeError('MCP request failed: ' + str(ident))
        names = {t['name'] for t in replies[2]['result']['tools']}
        if not {'flow_ping', 'task_prepare_start', 'task_transition'}.issubset(names):
            raise RuntimeError('required native lifecycle tools are missing')
        if replies[3]['result'].get('isError'):
            raise RuntimeError('flow_ping returned tool error')
        return {'protocol': 'PASS', 'tools': len(names), 'flow_ping': 'PASS',
                'database': 'temporary (configured user DB untouched)',
                'session_tools_verified': False}


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--codex-home', required=True)
    print(json.dumps(check(parser.parse_args().codex_home), indent=2))
