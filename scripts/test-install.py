#!/usr/bin/env python3
"""Installer contract tests. No root, network, services or real route mutations."""
import contextlib
import hashlib
import io
import json
import os
import re
import subprocess
import sys
import tarfile
import tempfile
import unittest
import urllib.error
import urllib.request
from pathlib import Path
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[1]
SCRIPT = ROOT / 'install.sh'
TEXT = SCRIPT.read_text()
PY_BLOCKS = re.findall(r"<<'PY'\n(.*?)\nPY", TEXT, re.S)
DOWNLOAD = re.search(r"<<'DOWNLOAD_PY'\n(.*?)\nDOWNLOAD_PY", TEXT, re.S).group(1)

class InstallerTests(unittest.TestCase):
    def test_syntax_and_arguments(self):
        subprocess.run(['bash', '-n', str(SCRIPT)], check=True)
        for marker, block in re.findall(r"<<'(\w*PY)'\n(.*?)\n\1", TEXT, re.S):
            compile(block, marker, 'exec')
        help_result = subprocess.run(['bash', str(SCRIPT), '--help'], capture_output=True)
        self.assertEqual(help_result.returncode, 0)
        for args in [['--unknown'], ['--version'], ['--beta', '--version', '1.0.1'], ['install', 'update']]:
            self.assertEqual(subprocess.run(['bash', str(SCRIPT)] + args, capture_output=True).returncode, 2)

    def test_archive_checksum_and_member(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            archive = root / 'tcp-brutal-manager-1.0.1-linux-amd64.tar.gz'
            with tarfile.open(archive, 'w:gz') as tar:
                info = tarfile.TarInfo('tcp-brutal-manager-1.0.1-linux-amd64/tcp-brutal-manager')
                info.size = 4
                tar.addfile(info, io.BytesIO(b'test'))
            (root / 'SHA256SUMS').write_text(hashlib.sha256(archive.read_bytes()).hexdigest() + '  ' + archive.name + '\n')
            with patch.object(sys, 'argv', ['test', str(archive), str(root / 'binary'), '1.0.1', 'amd64']):
                exec(PY_BLOCKS[0], {})
                self.assertEqual((root / 'binary').read_bytes(), b'test')
                archive.write_bytes(b'tampered')
                with self.assertRaises(SystemExit):
                    exec(PY_BLOCKS[0], {})

    def test_config_and_state_migration(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp).resolve()
            old = root / 'old-config/config.json'
            old.parent.mkdir()
            oldstate = root / 'old-state'
            oldstate.mkdir()
            cfg = {'rules': [{'ip': '192.0.2.1', 'rate': 200}], 'settings': {}}
            old.write_text(json.dumps(cfg))
            source = str(old) + '|ip|192.0.2.1|1'
            data = {'version': 2, 'records': [{'rule': {'sources': [source]}}], 'attributes': [{'sources': [source]}]}
            compact = json.dumps(data, separators=(',', ':')).encode()
            (oldstate / 'state.json').write_text(json.dumps({'data': data, 'checksum': hashlib.sha256(compact).hexdigest()}))
            (oldstate / 'initialized').write_text('1')
            def adapt(block):
                for before, after in [('/etc/tcp-brutal-manager', 'new-config'), ('/etc/tcp-brutal-config', 'old-config'), ('/var/lib/tcp-brutal-manager', 'new-state'), ('/var/lib/tcp-brutal-config', 'old-state')]:
                    block = block.replace(before, str(root / after))
                return block
            with patch.object(sys, 'argv', ['test', '']):
                exec(adapt(PY_BLOCKS[2]), {})
                exec(adapt(PY_BLOCKS[4]), {})
                new = root / 'new-config/config.json'
                self.assertEqual(json.loads(new.read_text()), cfg)
                env = json.loads((root / 'new-state/state.json').read_text())
                self.assertTrue(env['data']['records'][0]['rule']['sources'][0].startswith(str(new) + '|'))
                self.assertTrue(env['data']['attributes'][0]['sources'][0].startswith(str(new) + '|'))
                compact = json.dumps(env['data'], separators=(',', ':'), ensure_ascii=False).encode()
                self.assertEqual(hashlib.sha256(compact).hexdigest(), env['checksum'])
                self.assertTrue(list((root / 'new-state/deploy-backups').glob('*/state.json')))
                (oldstate / 'state.json').unlink()
                (oldstate / 'initialized').unlink()
                (root / 'new-state/state.json').write_text('broken')
                with self.assertRaises((SystemExit, ValueError)):
                    exec(adapt(PY_BLOCKS[2]), {})

    def run_download(self, response, version='', beta='0', failure=False):
        class Opener:
            def __init__(self):
                self.requests = []
                self.fail = failure
            def open(self, req, timeout):
                self.requests.append(req)
                if self.fail:
                    self.fail = False
                    raise urllib.error.HTTPError(req.full_url, 503, '', {}, None)
                return io.BytesIO(json.dumps(response).encode())
        opener = Opener()
        namespace = {}
        with tempfile.TemporaryDirectory() as temp, patch.dict(os.environ, {'GH_TOKEN': 'test-token-not-a-credential', 'PATH': ''}, clear=True), patch.object(sys, 'argv', ['test', 'owner/repo', version, beta, 'amd64', temp, 'check']), patch('urllib.request.build_opener', return_value=opener), patch('time.sleep'), contextlib.redirect_stdout(io.StringIO()):
            exec(DOWNLOAD, namespace)
            selected = Path(temp, 'selected-version').read_text()
        return namespace, opener, selected

    @staticmethod
    def release(tag='v1.0.1', prerelease=False):
        return {'tag_name': tag, 'draft': False, 'prerelease': prerelease, 'assets': [{'name': 'tcp-brutal-manager-1.0.1-linux-amd64.tar.gz', 'url': 'https://api.github.com/repos/owner/repo/releases/assets/1'}, {'name': 'SHA256SUMS', 'url': 'https://api.github.com/repos/owner/repo/releases/assets/2'}]}

    def test_version_channels_and_retry(self):
        _, opener, selected = self.run_download(self.release(), failure=True)
        self.assertEqual(selected, '1.0.1')
        self.assertEqual(len(opener.requests), 2)
        self.assertTrue(opener.requests[-1].full_url.endswith('/releases/latest'))
        _, opener, _ = self.run_download(self.release(prerelease=True), version='1.0.1')
        self.assertTrue(opener.requests[-1].full_url.endswith('/releases/tags/v1.0.1'))
        _, _, selected = self.run_download([self.release(prerelease=True)], beta='1')
        self.assertEqual(selected, '1.0.1')
        with self.assertRaises(SystemExit):
            self.run_download(self.release(tag='v1.0.0'))

    def test_redirect_drops_token(self):
        namespace, _, _ = self.run_download(self.release())
        redirect = namespace['SafeRedirect']()
        req = urllib.request.Request('https://api.github.com/file', headers={'Authorization': 'Bearer test-token-not-a-credential'})
        new = redirect.redirect_request(req, None, 302, '', {}, 'https://release-assets.githubusercontent.com/file')
        self.assertIsNone(new.get_header('Authorization'))
        with self.assertRaises(ValueError):
            redirect.redirect_request(req, None, 302, '', {}, 'http://example.com/file')

    def test_flush_plan_rejected(self):
        with tempfile.TemporaryDirectory() as temp:
            plan = Path(temp) / 'plan.json'
            plan.write_text(json.dumps({'would_flush': True, 'issues': []}))
            with patch.object(sys, 'argv', ['test', str(plan), '3']), self.assertRaises(SystemExit):
                exec(PY_BLOCKS[5], {})

if __name__ == '__main__':
    unittest.main()
