"""Offline source fixture bootstrap guard tests."""
import unittest
from unittest.mock import patch
from m0.bootstrap_source import ensure_fixture

SOURCE = "postgresql://postgres.hfvjpsniwcvkiulbdpbc:secret@aws-0-eu-west-1.pooler.supabase.com:5432/postgres"
TARGET = "postgresql://postgres.kikurisvmnqjpebqbtvx:secret@aws-0-eu-west-1.pooler.supabase.com:5432/postgres"

class BootstrapTests(unittest.TestCase):
    def test_same_project_ref_fails_before_sql(self):
        with patch("m0.bootstrap_source.run") as run:
            with self.assertRaisesRegex(ValueError, "match"):
                ensure_fixture(SOURCE, SOURCE)
            run.assert_not_called()

    def test_unmanaged_schema_never_modified(self):
        with patch("m0.bootstrap_source.run", side_effect=["1", "1", "<unmarked>"]) as run:
            with self.assertRaisesRegex(RuntimeError, "not managed"):
                ensure_fixture(SOURCE, TARGET)
            self.assertEqual(run.call_count, 3)

    def test_managed_fixture_reused_without_writes(self):
        with patch("m0.bootstrap_source.run", side_effect=["1", "1", "m0-source-fixture-v1", "2"]) as run:
            self.assertIn("verified", ensure_fixture(SOURCE, TARGET))
            self.assertEqual(run.call_count, 4)

    def test_missing_fixture_created_once(self):
        with patch("m0.bootstrap_source.run", side_effect=["1", "1", "<absent>", ""]) as run:
            self.assertIn("Created", ensure_fixture(SOURCE, TARGET))
            self.assertEqual(run.call_args.args[1], "-f")

if __name__ == "__main__":
    unittest.main()
