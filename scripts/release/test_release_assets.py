#!/usr/bin/env python3
"""Failure-mode regression coverage for the release-asset gate."""
import pathlib
import tempfile
import unittest
from unittest.mock import patch
import sys

import importlib.util

spec = importlib.util.spec_from_file_location("release_assets", pathlib.Path(__file__).with_name("release-assets.py"))
assert spec is not None and spec.loader is not None
gate = importlib.util.module_from_spec(spec)
spec.loader.exec_module(gate)


class ReleaseAssetTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.folder = pathlib.Path(self.temp.name)
        for name in gate.expected("v1.2.3"):
            (self.folder / name).write_bytes(b"sample")

    def run_gate(self, mode):
        with patch.object(sys, "argv", ["release-assets.py", mode, "v1.2.3", str(self.folder)]):
            gate.main()

    def test_exact_manifest_and_hashes(self):
        with self.assertRaises(ValueError):
            gate.expected("v1.2.3-test")
        self.run_gate("create")
        manifest = self.folder / "checksums.txt"
        (self.folder / "checksums.txt.sigstore.json").write_text("synthetic bundle", encoding="ascii")
        self.run_gate("verify")
        (self.folder / "momentum_1.2.3_amd64.deb").write_bytes(b"tampered")
        with self.assertRaisesRegex(ValueError, "checksum mismatch"):
            self.run_gate("verify")

    def test_missing_bundle_and_duplicate_entry_rejected(self):
        self.run_gate("create")
        with self.assertRaisesRegex(ValueError, "bundle missing"):
            self.run_gate("verify")
        (self.folder / "checksums.txt.sigstore.json").write_text("synthetic bundle", encoding="ascii")
        manifest = self.folder / "checksums.txt"
        first = manifest.read_text().splitlines()[0]
        with manifest.open("a") as stream:
            stream.write(first + "\n")
        with self.assertRaisesRegex(ValueError, "duplicate"):
            self.run_gate("verify")

    def test_missing_package_rejected(self):
        (self.folder / "momentum_1.2.3_arm64.deb").unlink()
        with self.assertRaisesRegex(ValueError, "missing required release assets"):
            self.run_gate("create")


if __name__ == "__main__":
    unittest.main()
