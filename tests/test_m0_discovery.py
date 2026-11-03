"""Offline checks for M0 documentation structure. Not a recovery test."""
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
COMPONENTS = ("PostgreSQL", "Storage", "Auth", "Edge Functions", "Settings", "Secrets")

class M0DocumentationTests(unittest.TestCase):
    def test_matrix_and_gate_are_present(self):
        text = (ROOT / "docs/discovery/M0_DISCOVERY.md").read_text()
        for name in COMPONENTS:
            self.assertIn(name, text)
        self.assertIn("NO-GO", text)
        self.assertIn("self-hosted", text)
        self.assertIn("Cloud", text)

    def test_protocol_requires_isolation(self):
        text = (ROOT / "docs/discovery/M0_TEST_PROTOCOL.md").read_text()
        for phrase in ("separate empty target", "synthetic", "SHA-256", "without original Mac"):
            self.assertIn(phrase, text)

if __name__ == "__main__":
    unittest.main()
